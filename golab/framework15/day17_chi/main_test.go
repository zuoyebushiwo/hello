package main

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHealthAndRequestID(t *testing.T) {
	rec := request(NewRouter(), http.MethodGet, "/healthz")
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, rec.Header().Get("X-Request-Id"))
}

func TestItemRoutes(t *testing.T) {
	router := NewRouter()
	rec := request(router, http.MethodGet, "/v1/items/42")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"monitor"`)
	require.Equal(t, http.StatusNotFound, request(router, http.MethodGet, "/v1/items/99").Code)
}

func TestTimeout(t *testing.T) {
	start := time.Now()
	rec := request(NewRouter(), http.MethodGet, "/v1/slow")
	require.Equal(t, http.StatusGatewayTimeout, rec.Code)
	require.Less(t, time.Since(start), 150*time.Millisecond)
}

func TestPanicIsolated(t *testing.T) {
	rec := request(NewRouter(), http.MethodGet, "/v1/panic")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NotContains(t, strings.ToLower(rec.Body.String()), "goroutine")
}
