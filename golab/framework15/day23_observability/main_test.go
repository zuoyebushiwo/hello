package main

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func setup() (http.Handler, *Metrics, *tracetest.InMemoryExporter) {
	exporter := tracetest.NewInMemoryExporter()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	metrics := NewMetrics()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(logger, provider.Tracer("test"), metrics), metrics, exporter
}

func TestMetricsAndTrace(t *testing.T) {
	handler, metrics, exporter := setup()
	rec := request(handler, "/v1/echo/alice")
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, rec.Header().Get("X-Trace-Id"))
	require.Equal(t, float64(1), testutil.ToFloat64(metrics.Requests.WithLabelValues(http.MethodGet, "GET /v1/echo/{name}", "200")))
	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	require.Equal(t, "http.request", spans[0].Name)
}

func TestUnmatchedRouteUsesBoundedCardinality(t *testing.T) {
	handler, metrics, _ := setup()
	rec := request(handler, "/v1/missing")
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, float64(1), testutil.ToFloat64(metrics.Requests.WithLabelValues(http.MethodGet, "unmatched", "404")))
}
