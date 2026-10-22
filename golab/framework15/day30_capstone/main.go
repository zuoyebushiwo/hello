package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"frameworklab/internal/labkit"

	"github.com/glebarez/sqlite"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-playground/validator/v10"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	ErrInsufficient    = errors.New("insufficient balance")
	ErrVersionConflict = errors.New("account version conflict")
	errDuplicate       = errors.New("duplicate idempotency key")
)

type Account struct {
	ID      uint   `gorm:"primaryKey"`
	Owner   string `gorm:"uniqueIndex;not null"`
	Balance int64  `gorm:"not null"`
	Version int64  `gorm:"not null;default:0"`
}

type LedgerEntry struct {
	ID             uint   `gorm:"primaryKey"`
	IdempotencyKey string `gorm:"uniqueIndex;not null"`
	FromAccountID  uint   `gorm:"not null"`
	ToAccountID    uint   `gorm:"not null"`
	Amount         int64  `gorm:"not null"`
}

type TransferRequest struct {
	IdempotencyKey string `json:"idempotency_key" validate:"required"`
	FromOwner      string `json:"from_owner" validate:"required"`
	ToOwner        string `json:"to_owner" validate:"required"`
	Amount         int64  `json:"amount" validate:"required,gt=0"`
}

type Metrics struct {
	Requests *prometheus.CounterVec
	Transfer *prometheus.CounterVec
	Latency  *prometheus.HistogramVec
	Registry *prometheus.Registry
}

type App struct {
	DB       *gorm.DB
	Router   http.Handler
	Metrics  *Metrics
	Tracer   oteltrace.Tracer
	Logger   *slog.Logger
	Exporter *tracetest.InMemoryExporter
	validate *validator.Validate
}

func OpenDB(name string) (*gorm.DB, error) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", name)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&Account{}, &LedgerEntry{}); err != nil {
		return nil, err
	}
	return db, nil
}

func NewApp(db *gorm.DB, logger *slog.Logger) *App {
	registry := prometheus.NewRegistry()
	metrics := &Metrics{
		Requests: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "http_requests_total", Help: "HTTP requests."}, []string{"method", "route", "status"}),
		Transfer: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "transfers_total", Help: "Transfer outcomes."}, []string{"result"}),
		Latency:  prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "http_request_duration_seconds", Help: "HTTP latency.", Buckets: []float64{0.005, 0.01, 0.05, 0.1, 0.5}}, []string{"route"}),
		Registry: registry,
	}
	registry.MustRegister(metrics.Requests, metrics.Transfer, metrics.Latency)
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))

	app := &App{
		DB:       db,
		Metrics:  metrics,
		Tracer:   provider.Tracer("frameworklab/day30"),
		Logger:   logger,
		Exporter: exporter,
		validate: validator.New(),
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Request-Id", middleware.GetReqID(r.Context()))
			next.ServeHTTP(w, r)
		})
	})
	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	router.Post("/v1/transfers", app.handleTransfer)
	router.Get("/v1/accounts/{owner}", app.handleBalance)
	router.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	app.Router = app.observe(router)
	return app
}

func (a *App) handleTransfer(w http.ResponseWriter, r *http.Request) {
	var request TransferRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		a.Metrics.Transfer.WithLabelValues("invalid").Inc()
		writeProblem(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := a.validate.Struct(request); err != nil {
		a.Metrics.Transfer.WithLabelValues("invalid").Inc()
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", err.Error())
		return
	}

	var from, to Account
	if err := a.DB.Where("owner = ?", request.FromOwner).First(&from).Error; err != nil {
		a.Metrics.Transfer.WithLabelValues("not_found").Inc()
		writeProblem(w, http.StatusNotFound, "account_not_found", request.FromOwner)
		return
	}
	if err := a.DB.Where("owner = ?", request.ToOwner).First(&to).Error; err != nil {
		a.Metrics.Transfer.WithLabelValues("not_found").Inc()
		writeProblem(w, http.StatusNotFound, "account_not_found", request.ToOwner)
		return
	}

	applied, err := a.transfer(request.IdempotencyKey, from.ID, to.ID, request.Amount)
	if err != nil {
		switch {
		case errors.Is(err, ErrInsufficient):
			a.Metrics.Transfer.WithLabelValues("insufficient").Inc()
			writeProblem(w, http.StatusConflict, "insufficient_balance", "source account has insufficient balance")
		case errors.Is(err, ErrVersionConflict):
			a.Metrics.Transfer.WithLabelValues("conflict").Inc()
			writeProblem(w, http.StatusConflict, "version_conflict", "retry the transfer")
		default:
			a.Metrics.Transfer.WithLabelValues("error").Inc()
			writeProblem(w, http.StatusInternalServerError, "internal_error", "transfer failed")
		}
		return
	}

	status := http.StatusCreated
	result := "applied"
	if !applied {
		status = http.StatusOK
		result = "duplicate"
	}
	a.Metrics.Transfer.WithLabelValues(result).Inc()
	fromBalance, _ := balance(a.DB, request.FromOwner)
	writeJSON(w, status, map[string]any{"applied": applied, "from_balance": fromBalance, "idempotency_key": request.IdempotencyKey})
}

func (a *App) handleBalance(w http.ResponseWriter, r *http.Request) {
	value, err := balance(a.DB, chi.URLParam(r, "owner"))
	if err != nil {
		writeProblem(w, http.StatusNotFound, "account_not_found", chi.URLParam(r, "owner"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"owner": chi.URLParam(r, "owner"), "balance": value})
}

func (a *App) transfer(key string, fromID, toID uint, amount int64) (bool, error) {
	applied := false
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		entry := LedgerEntry{IdempotencyKey: key, FromAccountID: fromID, ToAccountID: toID, Amount: amount}
		if err := tx.Create(&entry).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
				return errDuplicate
			}
			return err
		}
		var from, to Account
		if err := tx.First(&from, fromID).Error; err != nil {
			return err
		}
		if from.Balance < amount {
			return ErrInsufficient
		}
		if err := tx.First(&to, toID).Error; err != nil {
			return err
		}
		debit := tx.Model(&Account{}).Where("id = ? AND version = ?", from.ID, from.Version).
			Updates(map[string]any{"balance": gorm.Expr("balance - ?", amount), "version": gorm.Expr("version + 1")})
		if debit.Error != nil {
			return debit.Error
		}
		if debit.RowsAffected != 1 {
			return ErrVersionConflict
		}
		credit := tx.Model(&Account{}).Where("id = ? AND version = ?", to.ID, to.Version).
			Updates(map[string]any{"balance": gorm.Expr("balance + ?", amount), "version": gorm.Expr("version + 1")})
		if credit.Error != nil {
			return credit.Error
		}
		if credit.RowsAffected != 1 {
			return ErrVersionConflict
		}
		applied = true
		return nil
	})
	if errors.Is(err, errDuplicate) {
		return false, nil
	}
	return applied, err
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (a *App) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ctx, span := a.Tracer.Start(r.Context(), "http.request")
		defer span.End()
		r = r.WithContext(ctx)
		w.Header().Set("X-Trace-Id", span.SpanContext().TraceID().String())
		writer := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(writer, r)

		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		a.Metrics.Requests.WithLabelValues(r.Method, route, strconv.Itoa(writer.status)).Inc()
		a.Metrics.Latency.WithLabelValues(route).Observe(time.Since(start).Seconds())
		span.SetAttributes(attribute.String("http.route", route), attribute.Int("http.status_code", writer.status))
		if a.Logger != nil {
			a.Logger.Info("request", "method", r.Method, "route", route, "status", writer.status, "trace_id", span.SpanContext().TraceID().String())
		}
	})
}

func balance(db *gorm.DB, owner string) (int64, error) {
	var account Account
	if err := db.Where("owner = ?", owner).First(&account).Error; err != nil {
		return 0, err
	}
	return account.Balance, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"title": code, "status": status, "detail": detail})
}

func request(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func main() {
	lab := labkit.New("Day 30", "综合服务：HTTP + 事务 + 幂等 + 指标 + trace")
	lab.Section("框架简介：Chi + GORM + OpenTelemetry + Prometheus")
	lab.Infof("类型：组合架构")
	lab.Infof("是什么：把路由、事务、幂等、指标和 trace 组合成一个可运行的转账服务")
	lab.Infof("解决什么问题：单章学会 API 不等于会设计边界；真正困难的是失败时的原子性、重复请求和可诊断性")
	lab.Infof("核心抽象：HTTP 入口、DTO 校验、事务、幂等键、乐观锁、problem+json、Counter/Histogram、span")
	lab.Infof("采用情况：各组件本身都是主流选择；组合方式代表“自拼 Spring Boot”的一种参考实现")
	lab.Infof("适合：需要理解一个 Go 服务从入口到数据库再到观测面的完整链路")
	lab.Infof("慎用：示例省略了鉴权、生产迁移、Outbox、TLS、限流和优雅关闭，不能直接当生产模板")
	lab.Infof("对照：GoFrame 提供全家桶；Kratos 提供微服务底盘；本组合强调可控依赖和清晰边界")
	db, err := OpenDB("day30")
	if err != nil {
		lab.Check(false, "open db: %v", err)
		lab.Done()
		return
	}
	if err := db.Create(&Account{Owner: "alice", Balance: 1000}).Error; err != nil {
		lab.Check(false, "seed alice: %v", err)
	}
	if err := db.Create(&Account{Owner: "bob", Balance: 0}).Error; err != nil {
		lab.Check(false, "seed bob: %v", err)
	}
	app := NewApp(db, slog.New(slog.NewTextHandler(io.Discard, nil)))

	lab.Section("场景 A：创建转账")
	rec := request(app.Router, http.MethodPost, "/v1/transfers", `{"idempotency_key":"tr-1","from_owner":"alice","to_owner":"bob","amount":300}`)
	lab.Infof("status=%d trace_id=%s body=%s", rec.Code, rec.Header().Get("X-Trace-Id"), strings.TrimSpace(rec.Body.String()))
	lab.Check(rec.Code == http.StatusCreated, "合法请求返回 201")

	lab.Section("场景 B：重复幂等键")
	rec = request(app.Router, http.MethodPost, "/v1/transfers", `{"idempotency_key":"tr-1","from_owner":"alice","to_owner":"bob","amount":300}`)
	lab.Infof("status=%d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	lab.Check(rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), `"applied":false`), "重复请求返回 200 且不重复扣款")

	lab.Section("场景 C：余额不足")
	rec = request(app.Router, http.MethodPost, "/v1/transfers", `{"idempotency_key":"tr-2","from_owner":"alice","to_owner":"bob","amount":9999}`)
	lab.Infof("status=%d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	lab.Check(rec.Code == http.StatusConflict, "余额不足返回 409")

	lab.Section("场景 D：查询余额与可观测性")
	rec = request(app.Router, http.MethodGet, "/v1/accounts/alice", "")
	lab.Infof("status=%d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	lab.Check(rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "700"), "余额仍为 700")
	rec = request(app.Router, http.MethodGet, "/metrics", "")
	lab.Check(strings.Contains(rec.Body.String(), "transfers_total{result=\"applied\"} 1"), "Prometheus 指标记录了成功转账")
	lab.Check(len(app.Exporter.GetSpans()) >= 4, "每个请求都有 trace span")

	lab.Section("组合结论")
	lab.Infof("HTTP 只是入口；幂等键和事务才是资金安全的边界")
	lab.Infof("指标、trace、请求 ID 必须从入口贯穿到领域错误")
	lab.Infof("生产替换：SQLite -> PostgreSQL/pgx，内存 exporter -> OTLP，日志 -> 集中式采集")
	lab.Done()
}
