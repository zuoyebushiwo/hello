package main

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setup(t *testing.T) (*App, *gorm.DB) {
	t.Helper()
	db, err := OpenDB(t.Name())
	require.NoError(t, err)
	require.NoError(t, db.Create(&Account{Owner: "alice", Balance: 1000}).Error)
	require.NoError(t, db.Create(&Account{Owner: "bob", Balance: 0}).Error)
	app := NewApp(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return app, db
}

func TestEndToEnd(t *testing.T) {
	app, _ := setup(t)
	rec := request(app.Router, http.MethodPost, "/v1/transfers", `{"idempotency_key":"tr-1","from_owner":"alice","to_owner":"bob","amount":300}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, float64(1), testutil.ToFloat64(app.Metrics.Transfer.WithLabelValues("applied")))

	rec = request(app.Router, http.MethodPost, "/v1/transfers", `{"idempotency_key":"tr-1","from_owner":"alice","to_owner":"bob","amount":300}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"applied":false`)
	require.Equal(t, float64(1), testutil.ToFloat64(app.Metrics.Transfer.WithLabelValues("duplicate")))

	rec = request(app.Router, http.MethodGet, "/v1/accounts/alice", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"balance":700`)
	require.GreaterOrEqual(t, len(app.Exporter.GetSpans()), 3)
}

func TestInsufficientIsAtomic(t *testing.T) {
	app, _ := setup(t)
	rec := request(app.Router, http.MethodPost, "/v1/transfers", `{"idempotency_key":"tr-x","from_owner":"alice","to_owner":"bob","amount":9999}`)
	require.Equal(t, http.StatusConflict, rec.Code)
	rec = request(app.Router, http.MethodGet, "/v1/accounts/alice", "")
	require.Contains(t, rec.Body.String(), `"balance":1000`)

	var entries int64
	require.NoError(t, app.DB.Model(&LedgerEntry{}).Count(&entries).Error)
	require.Zero(t, entries)
}

func TestValidationAndNotFound(t *testing.T) {
	app, _ := setup(t)
	rec := request(app.Router, http.MethodPost, "/v1/transfers", `{"idempotency_key":"","from_owner":"alice","to_owner":"bob","amount":0}`)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	rec = request(app.Router, http.MethodPost, "/v1/transfers", `{"idempotency_key":"k","from_owner":"nobody","to_owner":"bob","amount":1}`)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
