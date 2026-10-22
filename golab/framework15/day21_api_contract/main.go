package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"

	"frameworklab/internal/labkit"

	"github.com/go-playground/validator/v10"
)

type Problem struct {
	Type   string            `json:"type"`
	Title  string            `json:"title"`
	Status int               `json:"status"`
	Detail string            `json:"detail"`
	Fields map[string]string `json:"fields,omitempty"`
}

type CreateOrderRequest struct {
	CustomerID string `json:"customer_id" validate:"required"`
	Amount     int    `json:"amount" validate:"required,gt=0"`
	Currency   string `json:"currency" validate:"required,oneof=CNY USD"`
}

type OrderV1 struct {
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
	Amount     int    `json:"amount"`
}

type OrderV2 struct {
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
	Amount     int    `json:"amount"`
	Currency   string `json:"currency"`
}

type API struct {
	validate *validator.Validate
}

func NewHandler() http.Handler {
	api := &API{validate: validator.New()}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/orders", api.createOrder)
	mux.HandleFunc("GET /v1/orders/{id}", api.getOrderV1)
	mux.HandleFunc("GET /v2/orders/{id}", api.getOrderV2)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return withRequestID(mux)
}

func (a *API) createOrder(w http.ResponseWriter, r *http.Request) {
	if contentType := r.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		writeProblem(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json", nil)
		return
	}
	var request CreateOrderRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if err := a.validate.Struct(request); err != nil {
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			fields := make(map[string]string, len(validationErrors))
			requestType := reflect.TypeOf(request)
			for _, fieldError := range validationErrors {
				fields[jsonFieldName(requestType, fieldError.StructField())] = fieldError.Tag()
			}
			writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "one or more fields are invalid", fields)
			return
		}
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, OrderV2{ID: "o-1001", CustomerID: request.CustomerID, Amount: request.Amount, Currency: request.Currency})
}

func jsonFieldName(structType reflect.Type, goName string) string {
	field, ok := structType.FieldByName(goName)
	if !ok {
		return goName
	}
	name := strings.Split(field.Tag.Get("json"), ",")[0]
	if name == "" || name == "-" {
		return goName
	}
	return name
}

func (a *API) getOrderV1(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("id") != "o-1001" {
		writeProblem(w, http.StatusNotFound, "order_not_found", "order does not exist", nil)
		return
	}
	writeJSON(w, http.StatusOK, OrderV1{ID: "o-1001", CustomerID: "c-1", Amount: 199})
}

func (a *API) getOrderV2(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("id") != "o-1001" {
		writeProblem(w, http.StatusNotFound, "order_not_found", "order does not exist", nil)
		return
	}
	writeJSON(w, http.StatusOK, OrderV2{ID: "o-1001", CustomerID: "c-1", Amount: 199, Currency: "CNY"})
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = "req-local"
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r)
	})
}

func writeProblem(w http.ResponseWriter, status int, code, detail string, fields map[string]string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Problem{Type: "about:blank", Title: code, Status: status, Detail: detail, Fields: fields})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func request(handler http.Handler, method, path, body, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Request-Id", "req-demo")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func main() {
	lab := labkit.New("Day 21", "API 契约：错误模型、严格解码与版本化 DTO")
	lab.Section("框架简介：net/http + validator")
	lab.Infof("类型：标准库 + API 契约组合")
	lab.Infof("是什么：用标准库设计稳定的 HTTP 契约，并用 validator 做入口字段校验")
	lab.Infof("解决什么问题：错误码、JSON 解码、未知字段和版本化 DTO 如果没有统一约定，客户端会依赖偶然行为")
	lab.Infof("核心抽象：problem+json、HTTP 状态码、DisallowUnknownFields、DTO 版本化、validator 标签")
	lab.Infof("采用情况：标准库无采用度问题；go-playground/validator 约 20k star，是 Go 生态的事实标准校验库")
	lab.Infof("适合：所有对外 API，尤其是多客户端、需要长期兼容和可观测错误的服务")
	lab.Infof("慎用：严格拒绝未知字段可能破坏旧客户端；不要把内部 err.Error() 原样暴露；版本化不只是改 URL")
	lab.Infof("对照：OpenAPI 描述接口，JSON Schema 验证响应；gRPC/Protobuf 提供更强契约但工具链更重")
	handler := NewHandler()

	lab.Section("场景 A：成功响应")
	rec := request(handler, http.MethodPost, "/v1/orders", `{"customer_id":"c-1","amount":199,"currency":"CNY"}`, "application/json")
	lab.Infof("status=%d request_id=%s body=%s", rec.Code, rec.Header().Get("X-Request-Id"), strings.TrimSpace(rec.Body.String()))
	lab.Check(rec.Code == http.StatusCreated && rec.Header().Get("X-Request-Id") == "req-demo", "响应带关联 ID，业务 DTO 与错误模型分离")

	lab.Section("场景 B：未知字段不再被静默忽略")
	rec = request(handler, http.MethodPost, "/v1/orders", `{"customer_id":"c-1","amount":199,"currency":"CNY","admin":true}`, "application/json")
	lab.Infof("status=%d content_type=%s body=%s", rec.Code, rec.Header().Get("Content-Type"), strings.TrimSpace(rec.Body.String()))
	lab.Check(rec.Code == http.StatusBadRequest && rec.Header().Get("Content-Type") == "application/problem+json", "未知字段返回标准 problem+json")

	lab.Section("场景 C：字段错误一次返回")
	rec = request(handler, http.MethodPost, "/v1/orders", `{"customer_id":"","amount":0,"currency":"EUR"}`, "application/json")
	lab.Infof("status=%d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	lab.Check(rec.Code == http.StatusUnprocessableEntity && strings.Contains(rec.Body.String(), "validation_failed"), "业务校验与 JSON 语法错误使用不同状态码")

	lab.Section("场景 D：显式版本化，不原地改字段语义")
	v1 := request(handler, http.MethodGet, "/v1/orders/o-1001", "", "application/json")
	v2 := request(handler, http.MethodGet, "/v2/orders/o-1001", "", "application/json")
	lab.Infof("v1=%s", strings.TrimSpace(v1.Body.String()))
	lab.Infof("v2=%s", strings.TrimSpace(v2.Body.String()))
	lab.Check(v1.Code == http.StatusOK && !strings.Contains(v1.Body.String(), "currency"), "v1 DTO 保持旧契约")
	lab.Check(v2.Code == http.StatusOK && strings.Contains(v2.Body.String(), "currency"), "v2 通过新增 DTO 暴露新字段")

	lab.Section("设计要点")
	lab.Infof("错误响应用 application/problem+json；不要把 err.Error() 直接暴露成生产详情")
	lab.Infof("DisallowUnknownFields 适合内部 API；对外 API 要评估客户端灰度与兼容策略")
	lab.Infof("版本化优先新增端点或 DTO，避免复用同名字段改变含义")
	lab.Done()
}
