package main

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T) (*ghttp.Server, string) {
	t.Helper()
	server, baseURL, err := StartServer(fmt.Sprintf("test-%s-%d", t.Name(), time.Now().UnixNano()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Shutdown() })
	return server, baseURL
}

func TestAccount(t *testing.T) {
	_, baseURL := newTestServer(t)
	resp, body := request(baseURL, http.MethodGet, "/v1/accounts/a-1", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "alice")
	require.Equal(t, "gf-demo", resp.Header.Get("X-Request-Id"))
}

func TestNotFound(t *testing.T) {
	_, baseURL := newTestServer(t)
	resp, body := request(baseURL, http.MethodGet, "/v1/accounts/nope", "")
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.Contains(t, body, "account_not_found")
}

func TestTransferValidation(t *testing.T) {
	_, baseURL := newTestServer(t)
	resp, _ := request(baseURL, http.MethodPost, "/v1/transfers", `{"from":"a-1","to":"b-1","amount":10}`)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	resp, body := request(baseURL, http.MethodPost, "/v1/transfers", `{"from":"","to":"b-1","amount":0}`)
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	require.Contains(t, body, "validation_failed")
}
