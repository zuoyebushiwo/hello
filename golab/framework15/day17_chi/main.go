package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"frameworklab/internal/labkit"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func NewRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(recoverer)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-Request-Id", middleware.GetReqID(req.Context()))
			next.ServeHTTP(w, req)
		})
	})

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/v1", func(r chi.Router) {
		r.Get("/items", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, []Item{{ID: "41", Name: "keyboard"}, {ID: "42", Name: "monitor"}})
		})
		r.Get("/items/{id}", func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "id")
			if id != "41" && id != "42" {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "item not found"})
				return
			}
			writeJSON(w, http.StatusOK, Item{ID: id, Name: map[string]string{"41": "keyboard", "42": "monitor"}[id]})
		})
		r.With(middleware.Timeout(30*time.Millisecond)).Get("/slow", func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(200 * time.Millisecond):
				writeJSON(w, http.StatusOK, map[string]string{"status": "slow"})
			case <-r.Context().Done():
				return
			}
		})
		r.Get("/panic", func(w http.ResponseWriter, r *http.Request) {
			panic("simulated handler panic")
		})
	})
	return r
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func request(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func main() {
	lab := labkit.New("Day 17", "Chi：贴近 net/http 的生产级路由与中间件")
	router := NewRouter()

	lab.Section("场景 A：健康检查与请求 ID")
	rec := request(router, http.MethodGet, "/healthz")
	lab.Infof("status=%d body=%s request_id=%s", rec.Code, strings.TrimSpace(rec.Body.String()), rec.Header().Get("X-Request-Id"))
	lab.Check(rec.Code == http.StatusOK, "健康检查返回 200")
	lab.Check(rec.Header().Get("X-Request-Id") != "", "RequestID 中间件把关联 ID 注入响应头")

	lab.Section("场景 B：路径参数与 JSON")
	rec = request(router, http.MethodGet, "/v1/items/42")
	lab.Infof("status=%d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	lab.Check(rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), `"monitor"`), "URL 参数与 JSON 编码正常")
	rec = request(router, http.MethodGet, "/v1/items/99")
	lab.Check(rec.Code == http.StatusNotFound, "业务 404 不被框架吞掉")

	lab.Section("场景 C：超时必须由 handler 配合 context")
	start := time.Now()
	rec = request(router, http.MethodGet, "/v1/slow")
	elapsed := time.Since(start)
	lab.Infof("status=%d elapsed=%s", rec.Code, elapsed.Round(time.Millisecond))
	lab.Check(rec.Code == http.StatusGatewayTimeout, "middleware.Timeout 在 deadline 后返回 504")
	lab.Check(elapsed < 150*time.Millisecond, "handler 监听 context 后能提前退出")

	lab.Section("场景 D：panic 在中间件边界被隔离")
	rec = request(router, http.MethodGet, "/v1/panic")
	lab.Infof("status=%d", rec.Code)
	lab.Check(rec.Code == http.StatusInternalServerError, "Recoverer 把 panic 转成 500，进程继续服务")

	lab.Section("生态位置")
	lab.Infof("Chi 直接实现 http.Handler，中间件生态和 net/http 完全兼容")
	lab.Infof("适合作为“自拼 Spring Boot”的 HTTP 内核；Gin/Echo/Fiber 则是另一套上下文模型")
	lab.Infof("生产关注点：RequestID、访问日志、panic、超时、鉴权、限流、CORS 顺序")
	lab.Done()
}
