package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateOrder(t *testing.T) {
	rec := request(NewHandler(), http.MethodPost, "/v1/orders", `{"customer_id":"c-1","amount":199,"currency":"CNY"}`, "application/json")
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Contains(t, rec.Body.String(), "o-1001")
	require.Equal(t, "req-demo", rec.Header().Get("X-Request-Id"))
}

func TestUnknownFieldIsRejected(t *testing.T) {
	rec := request(NewHandler(), http.MethodPost, "/v1/orders", `{"customer_id":"c-1","amount":1,"currency":"CNY","admin":true}`, "application/json")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	require.Contains(t, rec.Body.String(), "invalid_json")
}

func TestValidationProblem(t *testing.T) {
	rec := request(NewHandler(), http.MethodPost, "/v1/orders", `{"customer_id":"","amount":0,"currency":"EUR"}`, "application/json")
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, rec.Body.String(), `"currency":"oneof"`)
}

func TestVersionedDTOs(t *testing.T) {
	handler := NewHandler()
	v1 := request(handler, http.MethodGet, "/v1/orders/o-1001", "", "application/json")
	v2 := request(handler, http.MethodGet, "/v2/orders/o-1001", "", "application/json")
	require.NotContains(t, v1.Body.String(), "currency")
	require.Contains(t, v2.Body.String(), "currency")
}

func TestNotFoundProblem(t *testing.T) {
	rec := request(NewHandler(), http.MethodGet, "/v1/orders/missing", "", "application/json")
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "order_not_found")
}
