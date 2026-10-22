package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"

	"frameworklab/internal/labkit"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

type CreateOrderRequest struct {
	CustomerID string   `json:"customer_id" binding:"required"`
	Amount     int      `json:"amount" binding:"required,gt=0"`
	Items      []string `json:"items" binding:"required,min=1,dive,required"`
}

type OrderResponse struct {
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
	Amount     int    `json:"amount"`
}

var requestSeq atomic.Uint64

func NewRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.RecoveryWithWriter(io.Discard))
	router.Use(func(c *gin.Context) {
		c.Header("X-Request-Id", fmt.Sprintf("req-%d", requestSeq.Add(1)))
		c.Next()
	})

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	router.POST("/v1/orders", func(c *gin.Context) {
		var request CreateOrderRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			var validationErrors validator.ValidationErrors
			if errors.As(err, &validationErrors) {
				fields := make(map[string]string, len(validationErrors))
				for _, fieldError := range validationErrors {
					fields[fieldError.Field()] = fieldError.Tag()
				}
				c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "fields": fields})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json"})
			return
		}
		c.JSON(http.StatusCreated, OrderResponse{ID: "ord-1001", CustomerID: request.CustomerID, Amount: request.Amount})
	})

	router.GET("/v1/panic", func(c *gin.Context) {
		panic("simulated handler panic")
	})
	return router
}

func request(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func main() {
	lab := labkit.New("Day 18", "Gin：JSON 绑定、校验与中间件上下文")
	lab.Section("框架简介：Gin")
	lab.Infof("类型：Web 框架")
	lab.Infof("是什么：使用最广的 Go Web 框架之一，提供路由、Context、JSON 绑定、校验和 Recovery")
	lab.Infof("解决什么问题：快速构建 JSON API，减少请求解析、参数校验和统一响应的样板代码")
	lab.Infof("核心抽象：Engine、Context、ShouldBindJSON、路由组、中间件、Recovery")
	lab.Infof("采用情况：约 89k star，全球使用最广的 Go Web 框架之一，生态和教程非常丰富")
	lab.Infof("适合：业务 API、团队希望开箱即用地处理绑定和校验、能接受 Gin Context 的生命周期模型")
	lab.Infof("慎用：Gin Context 不是 net/http 标准中间件模型；handler 与框架耦合较深，默认校验只覆盖入口字段")
	lab.Infof("对照：Chi 更贴近标准库；Echo 能力相近；Fiber 基于 fasthttp，不能直接复用 net/http 中间件")
	router := NewRouter()

	lab.Section("场景 A：合法订单")
	rec := request(router, http.MethodPost, "/v1/orders", `{"customer_id":"c-1","amount":199,"items":["book","pen"]}`)
	lab.Infof("status=%d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	lab.Check(rec.Code == http.StatusCreated && strings.Contains(rec.Body.String(), "ord-1001"), "绑定 JSON 并返回业务结果")
	lab.Check(rec.Header().Get("X-Request-Id") != "", "中间件上下文可统一注入请求 ID")

	lab.Section("场景 B：多个字段校验失败")
	rec = request(router, http.MethodPost, "/v1/orders", `{"customer_id":"","amount":0,"items":[]}`)
	lab.Infof("status=%d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	lab.Check(rec.Code == http.StatusBadRequest && strings.Contains(rec.Body.String(), "validation_failed"), "校验失败返回 400 和结构化字段错误")
	lab.Check(strings.Contains(rec.Body.String(), "CustomerID") && strings.Contains(rec.Body.String(), "Amount"), "错误能够定位到具体字段")

	lab.Section("场景 C：非法 JSON")
	rec = request(router, http.MethodPost, "/v1/orders", `{"customer_id":`)
	lab.Infof("status=%d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	lab.Check(rec.Code == http.StatusBadRequest && strings.Contains(rec.Body.String(), "invalid_json"), "语法错误与业务校验错误分开表达")

	lab.Section("场景 D：Recovery 中间件")
	rec = request(router, http.MethodGet, "/v1/panic", "")
	lab.Infof("status=%d", rec.Code)
	lab.Check(rec.Code == http.StatusInternalServerError, "panic 被 Recovery 转成 500")

	lab.Section("生态位置")
	lab.Infof("Gin 的上下文模型不是 net/http 中间件标准，但生产使用最广、生态最大")
	lab.Infof("适合业务 API；如果团队大量依赖标准库中间件，优先 Chi")
	lab.Infof("校验在入口层完成，领域层仍需维护自己的不变量")
	lab.Done()
}
