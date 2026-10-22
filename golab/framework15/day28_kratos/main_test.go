package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccount(t *testing.T) {
	rec := request(NewServer(), http.MethodGet, "/v1/accounts/a-1", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "alice")
	require.Equal(t, "kratos-demo", rec.Header().Get("X-Request-Id"))
}

func TestNotFound(t *testing.T) {
	rec := request(NewServer(), http.MethodGet, "/v1/accounts/nope", "")
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "account_not_found")
}

func TestTransferValidation(t *testing.T) {
	server := NewServer()
	rec := request(server, http.MethodPost, "/v1/transfers", `{"from":"a-1","to":"b-1","amount":10}`)
	require.Equal(t, http.StatusAccepted, rec.Code)
	rec = request(server, http.MethodPost, "/v1/transfers", `{"from":"","to":"b-1","amount":0}`)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, rec.Body.String(), "validation_failed")
}
