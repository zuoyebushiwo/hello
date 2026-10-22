package main

import (
	"io"
	"net"
	"net/http"
	"strings"

	"frameworklab/internal/labkit"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

type TransferRequest struct {
	From   string `json:"from" v:"required"`
	To     string `json:"to" v:"required"`
	Amount int64  `json:"amount" v:"required|min:1"`
}

func NewServer(name string) *ghttp.Server {
	server := ghttp.GetServer(name)
	server.SetDumpRouterMap(false)
	server.SetLogStdout(false)
	server.Use(func(r *ghttp.Request) {
		r.Response.Header().Set("X-Request-Id", "gf-demo")
		r.Middleware.Next()
	})

	server.BindHandler("GET:/v1/accounts/{id}", func(r *ghttp.Request) {
		id := r.Get("id").String()
		if id != "a-1" {
			r.Response.WriteHeader(http.StatusNotFound)
			r.Response.WriteJson(g.Map{"error": "account_not_found", "id": id})
			return
		}
		r.Response.WriteJson(g.Map{"id": id, "owner": "alice", "balance": 700})
	})

	server.BindHandler("POST:/v1/transfers", func(r *ghttp.Request) {
		var request TransferRequest
		if err := r.Parse(&request); err != nil {
			r.Response.WriteHeader(http.StatusUnprocessableEntity)
			r.Response.WriteJson(g.Map{"error": "validation_failed", "detail": err.Error()})
			return
		}
		r.Response.WriteHeader(http.StatusAccepted)
		r.Response.WriteJson(g.Map{"status": "accepted", "from": request.From, "to": request.To, "amount": request.Amount})
	})

	server.BindHandler("GET:/healthz", func(r *ghttp.Request) {
		r.Response.WriteJson(g.Map{"status": "ok"})
	})
	return server
}

func StartServer(name string) (*ghttp.Server, string, error) {
	server := NewServer(name)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", err
	}
	if err := server.SetListener(listener); err != nil {
		listener.Close()
		return nil, "", err
	}
	if err := server.Start(); err != nil {
		listener.Close()
		return nil, "", err
	}
	return server, "http://" + listener.Addr().String(), nil
}

func request(baseURL, method, path, body string) (*http.Response, string) {
	req, _ := http.NewRequest(method, baseURL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &http.Response{StatusCode: 0}, err.Error()
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	return resp, string(payload)
}

func main() {
	lab := labkit.New("Day 27", "GoFrame：最接近 Spring Boot 的 Go 全家桶")
	server, baseURL, err := StartServer("day27-demo")
	if err != nil {
		lab.Check(false, "start server: %v", err)
		lab.Done()
		return
	}
	defer server.Shutdown()

	lab.Section("场景 A：路由、中间件与统一响应")
	resp, body := request(baseURL, http.MethodGet, "/v1/accounts/a-1", "")
	lab.Infof("status=%d request_id=%s body=%s", resp.StatusCode, resp.Header.Get("X-Request-Id"), strings.TrimSpace(body))
	lab.Check(resp.StatusCode == http.StatusOK && strings.Contains(body, "alice"), "控制器直接输出 JSON")
	lab.Check(resp.Header.Get("X-Request-Id") == "gf-demo", "中间件在框架上下文里统一加请求 ID")

	lab.Section("场景 B：业务 404")
	resp, body = request(baseURL, http.MethodGet, "/v1/accounts/missing", "")
	lab.Infof("status=%d body=%s", resp.StatusCode, strings.TrimSpace(body))
	lab.Check(resp.StatusCode == http.StatusNotFound, "显式状态码不会被框架覆盖")

	lab.Section("场景 C：请求结构体校验")
	resp, body = request(baseURL, http.MethodPost, "/v1/transfers", `{"from":"a-1","to":"b-1","amount":100}`)
	lab.Infof("status=%d body=%s", resp.StatusCode, strings.TrimSpace(body))
	lab.Check(resp.StatusCode == http.StatusAccepted, "合法请求被接收")
	resp, body = request(baseURL, http.MethodPost, "/v1/transfers", `{"from":"","to":"b-1","amount":0}`)
	lab.Infof("status=%d body=%s", resp.StatusCode, strings.TrimSpace(body))
	lab.Check(resp.StatusCode == http.StatusUnprocessableEntity, "v tag 校验失败返回 422")

	lab.Section("场景 D：框架边界")
	lab.Infof("GoFrame 同时提供 ghttp、gdb、gcfg、gvalid、glog、gcmd，体验最像 Spring Boot")
	lab.Infof("代价是框架绑定较深；团队要接受它的生命周期、配置和代码生成约定")
	lab.Infof("全球主流仍是 Chi/Gin + 自选库；国内 GoFrame 的整包体验更有吸引力")
	lab.Done()
}
