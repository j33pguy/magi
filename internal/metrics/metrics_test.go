package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsRegistered(t *testing.T) {
	// Initialize CounterVec labels so they appear in Gather output.
	CacheHits.WithLabelValues("memory")
	CacheMisses.WithLabelValues("memory")

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	want := map[string]bool{
		"magi_write_latency_seconds":      false,
		"magi_search_latency_seconds":     false,
		"magi_queue_depth":                false,
		"magi_cache_hits_total":           false,
		"magi_cache_misses_total":         false,
		"magi_embedding_duration_seconds": false,
		"magi_git_commits_total":          false,
		"magi_memory_count":               false,
		"magi_active_sessions":            false,
	}

	for _, f := range families {
		if _, ok := want[f.GetName()]; ok {
			want[f.GetName()] = true
		}
	}

	for name, found := range want {
		if !found {
			t.Errorf("metric %q not found in gathered families", name)
		}
	}
}

func TestMetricsHandler(t *testing.T) {
	// Increment some counters to ensure they appear in output.
	WriteLatency.Observe(0.05)
	SearchLatency.Observe(0.02)
	CacheHits.WithLabelValues("memory").Inc()
	CacheMisses.WithLabelValues("query").Inc()
	MemoryCount.Set(42)

	mux := http.NewServeMux()
	RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body, _ := io.ReadAll(w.Body)
	text := string(body)

	checks := []string{
		"magi_write_latency_seconds",
		"magi_search_latency_seconds",
		"magi_cache_hits_total",
		"magi_cache_misses_total",
		"magi_memory_count",
	}
	for _, c := range checks {
		if !strings.Contains(text, c) {
			t.Errorf("response body missing %q", c)
		}
	}
}
