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
	Backend string // "turso" (default) | "sqlite" | "sqlserver"

	// Turso
	TursoURL       string
	TursoAuthToken string
	ReplicaPath    string
	SyncInterval   time.Duration

	// SQLite
	SQLitePath string

	// SQL Server
	SQLServerURL string
}

// ConfigFromEnv reads storage configuration from environment variables.
func ConfigFromEnv() *Config {
	home, _ := os.UserHomeDir()

	replicaPath := os.Getenv("MAGI_REPLICA_PATH")
	if replicaPath == "" {
		replicaPath = filepath.Join(home, ".magi", "memory.db")
	}

	syncInterval := 60 * time.Second
	if v := os.Getenv("MAGI_SYNC_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v + "s"); err == nil {
			syncInterval = d
		}
	}

	sqlitePath := os.Getenv("SQLITE_PATH")
	if sqlitePath == "" {
		sqlitePath = filepath.Join(home, ".magi", "memory-local.db")
	}

	// SQL Server DSN: SQLSERVER_URL takes precedence; if absent, build from parts.
	sqlServerURL := os.Getenv("SQLSERVER_URL")
	if sqlServerURL == "" {
		host := os.Getenv("SQLSERVER_HOST")
		if host != "" {
			port := os.Getenv("SQLSERVER_PORT")
			if port == "" {
				port = "1433"
			}
			database := os.Getenv("SQLSERVER_DATABASE")
			user := os.Getenv("SQLSERVER_USER")
			pass := os.Getenv("SQLSERVER_PASSWORD")
			sqlServerURL = fmt.Sprintf("sqlserver://%s:%s@%s:%s?database=%s",
				user, pass, host, port, database)
		}
	}

	return &Config{
		Backend:        os.Getenv("MEMORY_BACKEND"),
		TursoURL:       os.Getenv("TURSO_URL"),
		TursoAuthToken: os.Getenv("TURSO_AUTH_TOKEN"),
		ReplicaPath:    replicaPath,
		SyncInterval:   syncInterval,
		SQLitePath:     sqlitePath,
		SQLServerURL:   sqlServerURL,
	}
}

// NewStore creates a Store based on config.
// Returns the Store interface; callers that need the concrete *Client (e.g. for
// VCS wrapping) should use NewTursoStore.
func NewStore(cfg *Config, logger *slog.Logger) (Store, error) {
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
	case "sqlserver", "mssql":
		return NewSQLServerClient(cfg.SQLServerURL, logger)
	default:
		return nil, fmt.Errorf("unknown storage backend: %s", cfg.Backend)
	}
}

// NewTursoStore is a convenience wrapper that returns the concrete *Client type.
// Used by callers that need the concrete type (e.g. VCS VersionedStore wrapping).
// Only supports turso and sqlite backends.
func NewTursoStore(cfg *Config, logger *slog.Logger) (*Client, error) {
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
		return nil, fmt.Errorf("backend %q does not support concrete *Client return — use NewStore", cfg.Backend)
	}
}
