// Package metrics exposes Prometheus metrics for MAGI observability.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// WriteLatency tracks the duration of write (remember) operations.
	WriteLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "magi",
		Name:      "write_latency_seconds",
		Help:      "Latency of memory write operations in seconds.",
		Buckets:   prometheus.DefBuckets,
	})

	// SearchLatency tracks the duration of search (recall) operations.
	SearchLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "magi",
		Name:      "search_latency_seconds",
		Help:      "Latency of memory search operations in seconds.",
		Buckets:   prometheus.DefBuckets,
	})

	// QueueDepth tracks the current depth of the async write pipeline.
	QueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "magi",
		Name:      "queue_depth",
		Help:      "Current depth of the async write pipeline.",
	})

	// CacheHits tracks cache hit events, labeled by cache type.
	CacheHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "magi",
		Name:      "cache_hits_total",
		Help:      "Total cache hit count by cache type.",
	}, []string{"cache"})

	// CacheMisses tracks cache miss events, labeled by cache type.
	CacheMisses = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "magi",
		Name:      "cache_misses_total",
		Help:      "Total cache miss count by cache type.",
	}, []string{"cache"})

	// EmbeddingDuration tracks how long embedding generation takes.
	EmbeddingDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "magi",
		Name:      "embedding_duration_seconds",
		Help:      "Duration of embedding generation in seconds.",
		Buckets:   prometheus.DefBuckets,
	})

	// GitCommitCount tracks the total number of git commits made.
	GitCommitCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "magi",
		Name:      "git_commits_total",
		Help:      "Total number of git commits made.",
	})

	// MemoryCount tracks the current number of memories in the database.
	MemoryCount = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "magi",
		Name:      "memory_count",
		Help:      "Current number of memories in the database.",
	})

	// ActiveSessions tracks the number of active MCP sessions.
	ActiveSessions = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "magi",
		Name:      "active_sessions",
		Help:      "Number of active MCP sessions.",
	})
)
