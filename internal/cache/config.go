// Package cache provides caching layers for the magi memory server.
package cache

import (
	"os"
	"strconv"
	"time"
)

// Config holds cache settings.
type Config struct {
	Enabled       bool
	QueryTTL      time.Duration
	MemorySize    int
	EmbeddingSize int
}

// ConfigFromEnv reads cache config from environment variables.
func ConfigFromEnv() Config {
	c := Config{
		Enabled:       os.Getenv("MAGI_CACHE_ENABLED") == "true",
		QueryTTL:      60 * time.Second,
		MemorySize:    1000,
		EmbeddingSize: 5000,
	}

	if v := os.Getenv("MAGI_CACHE_QUERY_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			c.QueryTTL = d
		}
	}
	if v := os.Getenv("MAGI_CACHE_MEMORY_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.MemorySize = n
		}
	}
	if v := os.Getenv("MAGI_CACHE_EMBEDDING_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.EmbeddingSize = n
		}
	}

	return c
}
