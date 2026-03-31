// Package db provides database access for the magi server using Turso
// with embedded replicas for fast local reads and cloud sync.
package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tursodatabase/go-libsql"
)

// TursoClient wraps a Turso database connection with embedded replica support.
type TursoClient struct {
	DB        *sql.DB
	connector *libsql.Connector
	logger    *slog.Logger
}

// Client is an alias for TursoClient, preserving backward compatibility.
type Client = TursoClient

// TursoConfig holds Turso database connection settings.
type TursoConfig struct {
	URL          string
	AuthToken    string
	ReplicaPath  string
	SyncInterval time.Duration
}

// tursoConfigFromEnv reads Turso configuration from environment variables.
func tursoConfigFromEnv() *TursoConfig {
	replicaPath := os.Getenv("MAGI_REPLICA_PATH")
	if replicaPath == "" {
		home, _ := os.UserHomeDir()
		replicaPath = filepath.Join(home, ".magi", "memory.db")
	}

	syncInterval := 60 * time.Second
	if v := os.Getenv("MAGI_SYNC_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v + "s"); err == nil {
			syncInterval = d
		}
	}

	return &TursoConfig{
		URL:          os.Getenv("TURSO_URL"),
		AuthToken:    os.Getenv("TURSO_AUTH_TOKEN"),
		ReplicaPath:  replicaPath,
		SyncInterval: syncInterval,
	}
}

// NewTursoClient creates a new database client connected to Turso with an embedded replica.
func NewTursoClient(cfg *TursoConfig, logger *slog.Logger) (*TursoClient, error) {
	// Ensure the replica directory exists
	if err := os.MkdirAll(filepath.Dir(cfg.ReplicaPath), 0o755); err != nil {
		return nil, fmt.Errorf("creating replica directory: %w", err)
	}

	connector, err := libsql.NewEmbeddedReplicaConnector(
		cfg.ReplicaPath,
		cfg.URL,
		libsql.WithAuthToken(cfg.AuthToken),
		libsql.WithSyncInterval(cfg.SyncInterval),
	)
	if err != nil {
		return nil, fmt.Errorf("creating embedded replica connector: %w", err)
	}

	db := sql.OpenDB(connector)

	// Keep pool small for embedded replica. Turso Hrana streams expire
	// server-side after idle periods; short lifetimes force the pool to
	// discard stale connections before reuse.
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(2 * time.Minute)
	db.SetConnMaxIdleTime(15 * time.Second)

	// Verify connectivity
	if err := db.Ping(); err != nil {
		connector.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return &TursoClient{
		DB:        db,
		connector: connector,
		logger:    logger,
	}, nil
}

// Sync manually triggers a sync with the remote Turso database.
// No-op when running without a remote (e.g. SQLite backend).
func (c *TursoClient) Sync() error {
	if c.connector == nil {
		return nil
	}
	rep, err := c.connector.Sync()
	if err != nil {
		return fmt.Errorf("syncing database: %w", err)
	}
	c.logger.Debug("Database synced", slog.Int("framesSynced", rep.FramesSynced))
	return nil
}

// isStreamExpired reports whether the error is a Turso Hrana "stream not found"
// error, indicating the server-side stream expired during idle.
func isStreamExpired(err error) bool {
	return err != nil && strings.Contains(err.Error(), "stream not found")
}

// Close shuts down the database connection and connector.
func (c *TursoClient) Close() error {
	if err := c.DB.Close(); err != nil {
		return fmt.Errorf("closing database: %w", err)
	}
	if c.connector != nil {
		if err := c.connector.Close(); err != nil {
			return fmt.Errorf("closing connector: %w", err)
		}
	}
	return nil
}
