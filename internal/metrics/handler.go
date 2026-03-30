package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Handler returns an http.Handler that serves Prometheus metrics at /metrics.
func Handler() http.Handler {
	return promhttp.Handler()
}

// RegisterRoutes adds the /metrics endpoint to the given ServeMux.
func RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /metrics", Handler())
}
