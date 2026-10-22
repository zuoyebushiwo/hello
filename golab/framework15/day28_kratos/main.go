package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"

	"frameworklab/internal/labkit"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

type TransferRequest struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Amount int64  `json:"amount"`
}

func (r TransferRequest) Validate() error {
	if r.From == "" || r.To == "" {
		return errors.New("from and to are required")
	}
	if r.Amount <= 0 {
		return errors.New("amount must be positive")
	}
	return nil
}

func NewServer() *khttp.Server {
	server := khttp.NewServer(
		khttp.Address("127.0.0.1:0"),
		khttp.Filter(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Request-Id", "kratos-demo")
				next.ServeHTTP(w, r)
			})
		}),
	)

	router := server.Route("/v1")
	router.GET("/accounts/{id}", func(ctx khttp.Context) error {
		id := ctx.Vars().Get("id")
		if id != "a-1" {
			return ctx.JSON(http.StatusNotFound, map[string]any{"error": "account_not_found", "id": id})
		}
		return ctx.JSON(http.StatusOK, map[string]any{"id": id, "owner": "alice", "balance": 700})
	})

	router.POST("/transfers", func(ctx khttp.Context) error {
		var request TransferRequest
		if err := json.NewDecoder(ctx.Request().Body).Decode(&request); err != nil {
			return ctx.JSON(http.StatusBadRequest, map[string]any{"error": "invalid_json"})
		}
		if err := request.Validate(); err != nil {
			return ctx.JSON(http.StatusUnprocessableEntity, map[string]any{"error": "validation_failed", "detail": err.Error()})
		}
		return ctx.JSON(http.StatusAccepted, map[string]any{"status": "accepted", "from": request.From, "to": request.To, "amount": request.Amount})
	})

	router.GET("/healthz", func(ctx khttp.Context) error {
		return ctx.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	return server
}

func request(server http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	return rec
}

func main() {
	lab := labkit.New("Day 28", "Kratos：Proto-first 微服务底盘")
	server := NewServer()

	lab.Section("场景 A：Kratos 路由与滤镜")
	rec := request(server, http.MethodGet, "/v1/accounts/a-1", "")
	lab.Infof("status=%d request_id=%s body=%s", rec.Code, rec.Header().Get("X-Request-Id"), strings.TrimSpace(rec.Body.String()))
	lab.Check(rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "alice"), "Kratos Context 返回 JSON")
	lab.Check(rec.Header().Get("X-Request-Id") == "kratos-demo", "Filter 在 transport 层统一加请求 ID")

	lab.Section("场景 B：业务错误")
	rec = request(server, http.MethodGet, "/v1/accounts/missing", "")
	lab.Infof("status=%d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	lab.Check(rec.Code == http.StatusNotFound, "handler 返回 error 或显式 JSON 状态码")

	lab.Section("场景 C：请求校验")
	rec = request(server, http.MethodPost, "/v1/transfers", `{"from":"a-1","to":"b-1","amount":10}`)
	lab.Check(rec.Code == http.StatusAccepted, "合法转账请求被接收")
	rec = request(server, http.MethodPost, "/v1/transfers", `{"from":"","to":"b-1","amount":0}`)
	lab.Infof("status=%d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	lab.Check(rec.Code == http.StatusUnprocessableEntity, "领域校验失败返回 422")

	lab.Section("场景 D：Kratos 的能力边界")
	lab.Infof("Kratos 提供 transport/http、transport/grpc、registry、config、middleware、log")
	lab.Infof("典型工作流是 proto -> 生成 Go 代码 -> 实现 service -> 接入注册中心和治理中间件")
	lab.Infof("它不是 Spring Boot 式注解容器；依赖注入和生命周期仍要显式写")
	lab.Done()
}
