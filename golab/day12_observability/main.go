package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golab/internal/labkit"
)

func newMux(logger *slog.Logger, reg *labkit.Registry) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/work", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		nodeType := r.URL.Query().Get("type")
		if nodeType == "" {
			nodeType = "sql"
		}
		reqLog := logger.With(
			"request_id", r.Header.Get("X-Request-ID"),
			"node_type", nodeType,
			"path", r.URL.Path,
		)
		time.Sleep(3 * time.Millisecond)
		status := "success"
		code := http.StatusOK
		if nodeType == "fail" {
			status = "failed"
			code = http.StatusInternalServerError
		}
		reg.Inc("dataflow_node_total", map[string]string{"type": nodeType, "status": status})
		reg.Observe("dataflow_node_duration_seconds", time.Since(start).Seconds(), map[string]string{"type": nodeType})
		reqLog.Info("node finished", "status", status, "duration_ms", time.Since(start).Milliseconds())
		w.WriteHeader(code)
		_, _ = io.WriteString(w, status)
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = io.WriteString(w, reg.Render())
	})
	return mux
}

func main() {
	lab := labkit.New("Day 12", "可观测性：结构化日志 + Prometheus 指标 + 优雅关闭")
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	reg := labkit.NewRegistry()
	mux := newMux(logger, reg)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		lab.Failf("listen: %v", err)
		lab.Done()
		return
	}
	srv := &http.Server{Handler: mux}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	base := "http://" + ln.Addr().String()
	client := &http.Client{Timeout: 2 * time.Second}

	// Wait until the listener actually accepts connections.
	for i := 0; i < 50; i++ {
		resp, err := client.Get(base + "/metrics")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	lab.Section("场景 A：三次成功、一次失败，每个请求带 request_id")
	requests := []struct {
		typ string
		id  string
	}{
		{"sql", "req-001"},
		{"sync", "req-002"},
		{"sql", "req-003"},
		{"fail", "req-004"},
	}
	for _, r := range requests {
		req, _ := http.NewRequest(http.MethodGet, base+"/work?type="+r.typ, nil)
		req.Header.Set("X-Request-ID", r.id)
		resp, err := client.Do(req)
		if err != nil {
			lab.Failf("request %s: %v", r.typ, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		lab.Infof("type=%-5s request_id=%-8s status=%d body=%s", r.typ, r.id, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	lab.Section("场景 B：拉取 Prometheus 文本格式")
	resp, err := client.Get(base + "/metrics")
	if err != nil {
		lab.Failf("scrape metrics: %v", err)
		lab.Done()
		return
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	for i, line := range lines {
		if i >= 12 {
			break
		}
		lab.Infof("%s", line)
	}
	metrics := string(body)
	lab.Check(strings.Contains(metrics, `dataflow_node_total{status="success",type="sql"} 2`), "counter 按 type/status 正确聚合")
	lab.Check(strings.Contains(metrics, `dataflow_node_total{status="failed",type="fail"} 1`), "失败请求单独计数")
	lab.Check(strings.Contains(metrics, "dataflow_node_duration_seconds_bucket"), "histogram 输出 _bucket/_sum/_count")

	lab.Section("场景 C：优雅关闭")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		lab.Failf("shutdown: %v", err)
	}
	select {
	case err := <-serveErr:
		lab.Check(err == nil || errors.Is(err, http.ErrServerClosed), "Serve 返回 ErrServerClosed，而不是进程退出")
	case <-time.After(time.Second):
		lab.Failf("server did not stop")
	}

	lab.Section("Java → Go 对照")
	lab.Infof("SLF4J + MDC                    ≈  slog.With(request_id, node_type, ...)；字段是显式 key/value")
	lab.Infof("Micrometer Counter/Timer       ≈  labkit.Registry 的 counter/histogram")
	lab.Infof("Actuator /prometheus           ≈  独立 /metrics handler")
	lab.Infof("Spring graceful shutdown       ≈  srv.Shutdown(ctx) + signal.NotifyContext")
	lab.Done()
}
