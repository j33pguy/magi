package db

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// Config holds storage backend configuration.
type Config struct {
	Backend string // "turso" (default) | "sqlite"

	// Turso
	TursoURL       string
	TursoAuthToken string
	ReplicaPath    string
	SyncInterval   time.Duration

	// SQLite
	SQLitePath string
}

// ConfigFromEnv reads storage configuration from environment variables.
func ConfigFromEnv() *Config {
	home, _ := os.UserHomeDir()

	replicaPath := os.Getenv("MAGI_REPLICA_PATH")
	if replicaPath == "" {
		replicaPath = filepath.Join(home, ".claude", "memory.db")
	}

	syncInterval := 60 * time.Second
	if v := os.Getenv("MAGI_SYNC_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v + "s"); err == nil {
			syncInterval = d
		}
	}

	sqlitePath := os.Getenv("SQLITE_PATH")
	if sqlitePath == "" {
		sqlitePath = filepath.Join(home, ".claude", "memory-local.db")
	}

	return &Config{
		Backend:        os.Getenv("MEMORY_BACKEND"),
		TursoURL:       os.Getenv("TURSO_URL"),
		TursoAuthToken: os.Getenv("TURSO_AUTH_TOKEN"),
		ReplicaPath:    replicaPath,
		SyncInterval:   syncInterval,
		SQLitePath:     sqlitePath,
	}
}

// NewStore creates a Store based on config.
func NewStore(cfg *Config, logger *slog.Logger) (*Client, error) {
	switch cfg.Backend {
	case "turso", "":
		return NewTursoClient(&TursoConfig{
			URL:          cfg.TursoURL,
			AuthToken:    cfg.TursoAuthToken,
			ReplicaPath:  cfg.ReplicaPath,
			SyncInterval: cfg.SyncInterval,
		}, logger)
	case "sqlite":
		c, err := NewSQLiteClient(cfg.SQLitePath, logger)
		if err != nil {
			return nil, err
		}
		return c.TursoClient, nil
	default:
		return nil, fmt.Errorf("unknown storage backend: %s", cfg.Backend)
	}
}
