package node

import (
	"os"
	"strconv"
)

// Mode controls how nodes communicate.
type Mode string

const (
	// ModeEmbedded runs all nodes as goroutine pools in-process (Phase 1).
	ModeEmbedded Mode = "embedded"
)

// Config holds node mesh configuration.
type Config struct {
	Mode               Mode
	WriterPoolSize     int
	ReaderPoolSize     int
	CoordinatorEnabled bool
}

// DefaultConfig returns sensible defaults for embedded mode.
func DefaultConfig() *Config {
	return &Config{
		Mode:               ModeEmbedded,
		WriterPoolSize:     4,
		ReaderPoolSize:     8,
		CoordinatorEnabled: true,
	}
}

// ConfigFromEnv reads node configuration from environment variables.
func ConfigFromEnv() *Config {
	cfg := DefaultConfig()

	if v := os.Getenv("MAGI_NODE_MODE"); v != "" {
		cfg.Mode = Mode(v)
	}
	if v := os.Getenv("MAGI_WRITER_POOL_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.WriterPoolSize = n
		}
	}
	if v := os.Getenv("MAGI_READER_POOL_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.ReaderPoolSize = n
		}
	}
	if v := os.Getenv("MAGI_COORDINATOR_ENABLED"); v != "" {
		cfg.CoordinatorEnabled = v == "true" || v == "1"
	}

	return cfg
}
