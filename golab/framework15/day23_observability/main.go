package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"frameworklab/internal/labkit"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type Metrics struct {
	Requests *prometheus.CounterVec
	Latency  *prometheus.HistogramVec
	Registry *prometheus.Registry
}

func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests by route and status.",
	}, []string{"method", "route", "status"})
	latency := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency by route.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25},
	}, []string{"route"})
	registry.MustRegister(requests, latency)
	return &Metrics{Requests: requests, Latency: latency, Registry: registry}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func NewHandler(logger *slog.Logger, tracer oteltrace.Tracer, metrics *Metrics) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/echo/{name}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"name": r.PathValue("name")})
	})
	return instrument(logger, tracer, metrics, mux)
}

func instrument(logger *slog.Logger, tracer oteltrace.Tracer, metrics *Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ctx, span := tracer.Start(r.Context(), "http.request")
		defer span.End()
		span.SetAttributes(attribute.String("http.method", r.Method), attribute.String("url.path", r.URL.Path))
		r = r.WithContext(ctx)
		w.Header().Set("X-Trace-Id", span.SpanContext().TraceID().String())

		writer := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(writer, r)

		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		metrics.Requests.WithLabelValues(r.Method, route, strconv.Itoa(writer.status)).Inc()
		metrics.Latency.WithLabelValues(route).Observe(time.Since(start).Seconds())
		span.SetAttributes(attribute.Int("http.status_code", writer.status), attribute.String("http.route", route))
		if writer.status >= 500 {
			span.RecordError(http.ErrAbortHandler)
		}

		traceID := span.SpanContext().TraceID().String()
		logger.Info("request completed",
			"method", r.Method,
			"route", route,
			"status", writer.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"trace_id", traceID,
		)
	})
}

func request(handler http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func main() {
	lab := labkit.New("Day 23", "可观测性：slog、Prometheus 与 OpenTelemetry")
	lab.Section("框架简介：slog + Prometheus + OpenTelemetry")
	lab.Infof("类型：可观测性组合")
	lab.Infof("是什么：slog 输出结构化日志，Prometheus 采集指标，OpenTelemetry 生成并传播 trace")
	lab.Infof("解决什么问题：线上问题需要同时回答“发生了什么、量有多大、慢在哪一段”，单一日志或单一指标都不够")
	lab.Infof("核心抽象：结构化 key-value、Counter/Histogram、Span/Context 传播、route 标签、trace ID 关联")
	lab.Infof("采用情况：slog 是标准库；Prometheus client_golang 是事实标准；OpenTelemetry Go 是 CNCF 标准，实际采用度高于 star 数")
	lab.Infof("适合：所有生产服务，尤其是跨服务调用、需要 SLO 和故障定位的系统")
	lab.Infof("慎用：不要把 user_id、request_id 等高基数标签放进 Prometheus label；日志、指标、trace 不能互相替代")
	lab.Infof("对照：zap/zerolog 追求更低分配；Prometheus 负责聚合指标，OTel 负责跨服务 trace")
	exporter := tracetest.NewInMemoryExporter()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	defer provider.Shutdown(context.Background())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metrics := NewMetrics()
	handler := NewHandler(logger, provider.Tracer("frameworklab/day23"), metrics)

	lab.Section("场景 A：请求日志、指标和 span 一次采集")
	for i := 0; i < 2; i++ {
		rec := request(handler, "/v1/echo/alice")
		lab.Infof("status=%d trace_id=%s body=%s", rec.Code, rec.Header().Get("X-Trace-Id"), strings.TrimSpace(rec.Body.String()))
	}
	counter := testutil.ToFloat64(metrics.Requests.WithLabelValues(http.MethodGet, "GET /v1/echo/{name}", "200"))
	lab.Infof("requests_total=%v", counter)
	lab.Check(counter == 2, "相同 route 标签聚合两次请求")
	lab.Check(len(exporter.GetSpans()) == 2, "每个请求生成一个 span")

	lab.Section("场景 B：状态码与错误 span")
	rec := request(handler, "/v1/missing")
	lab.Infof("status=%d", rec.Code)
	lab.Check(rec.Code == http.StatusNotFound, "未匹配路由仍被中间件观测")
	notFound := testutil.ToFloat64(metrics.Requests.WithLabelValues(http.MethodGet, "unmatched", "404"))
	lab.Check(notFound == 1, "未匹配路由不会被错误标成成功")

	lab.Section("场景 C：指标基数")
	lab.Infof("route 使用模板 GET /v1/echo/{name}，而不是 /v1/echo/alice、/v1/echo/bob")
	lab.Infof("高基数标签（user_id、request_id）会打爆 Prometheus 时间序列")

	lab.Section("生态位置")
	lab.Infof("slog 是标准库结构化日志；zap/zerolog 用于更极致分配性能")
	lab.Infof("Prometheus 负责聚合指标；OpenTelemetry 负责跨服务 trace")
	lab.Infof("生产接入：OTLP exporter + Collector，本地实验用内存 exporter 便于断言")
	lab.Done()
}
