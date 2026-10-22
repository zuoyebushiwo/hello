package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateOrder(t *testing.T) {
	rec := request(NewRouter(), http.MethodPost, "/v1/orders", `{"customer_id":"c-1","amount":199,"items":["book"]}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Contains(t, rec.Body.String(), `"ord-1001"`)
	require.NotEmpty(t, rec.Header().Get("X-Request-Id"))
}

func TestValidationErrors(t *testing.T) {
	rec := request(NewRouter(), http.MethodPost, "/v1/orders", `{"customer_id":"","amount":0,"items":[]}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "validation_failed")
	require.Contains(t, rec.Body.String(), "CustomerID")
	require.Contains(t, rec.Body.String(), "Amount")
}

func TestMalformedJSON(t *testing.T) {
	rec := request(NewRouter(), http.MethodPost, "/v1/orders", `{"customer_id":`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid_json")
}

func TestRecovery(t *testing.T) {
	rec := request(NewRouter(), http.MethodGet, "/v1/panic", "")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}
