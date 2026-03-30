// Package pipeline provides an asynchronous write pipeline for memory storage.
package pipeline

import (
	"os"
	"runtime"
	"strconv"
	"time"
)

// Config holds async write pipeline settings.
type Config struct {
	Enabled        bool
	Workers        int
	QueueSize      int
	FlushInterval  time.Duration
	BatchMaxSize   int
}

// ConfigFromEnv reads pipeline config from environment variables.
func ConfigFromEnv() Config {
	c := Config{
		Enabled:       os.Getenv("MAGI_ASYNC_WRITES") == "true",
		Workers:       runtime.NumCPU(),
		QueueSize:     1000,
		FlushInterval: 100 * time.Millisecond,
		BatchMaxSize:  50,
	}

	if v := os.Getenv("MAGI_WRITE_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.Workers = n
		}
	}
	if v := os.Getenv("MAGI_WRITE_QUEUE_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.QueueSize = n
		}
	}
	if v := os.Getenv("MAGI_BATCH_FLUSH_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			c.FlushInterval = d
		}
	}
	if v := os.Getenv("MAGI_BATCH_MAX_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.BatchMaxSize = n
		}
	}

	return c
}
