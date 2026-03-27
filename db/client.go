// Package db provides database access for the claude-memory server using Turso
// with embedded replicas for fast local reads and cloud sync.
package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/tursodatabase/go-libsql"
)

// Client wraps a Turso database connection with embedded replica support.
type Client struct {
	DB        *sql.DB
	connector *libsql.Connector
	logger    *slog.Logger
}

// Config holds database connection settings.
type Config struct {
	URL          string
	AuthToken    string
	ReplicaPath  string
	SyncInterval time.Duration
}

// ConfigFromEnv reads database configuration from environment variables.
func ConfigFromEnv() *Config {
	replicaPath := os.Getenv("CLAUDE_MEMORY_REPLICA_PATH")
	if replicaPath == "" {
		home, _ := os.UserHomeDir()
		replicaPath = filepath.Join(home, ".claude", "memory.db")
	}

	syncInterval := 60 * time.Second
	if v := os.Getenv("CLAUDE_MEMORY_SYNC_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v + "s"); err == nil {
			syncInterval = d
		}
	}

	return &Config{
		URL:          os.Getenv("TURSO_URL"),
		AuthToken:    os.Getenv("TURSO_AUTH_TOKEN"),
		ReplicaPath:  replicaPath,
		SyncInterval: syncInterval,
	}
}

// NewClient creates a new database client connected to Turso with an embedded replica.
func NewClient(cfg *Config, logger *slog.Logger) (*Client, error) {
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

	// Keep pool small for embedded replica. Turso Hrana streams can expire on
	// long-idle connections, so cap idle time. Tag writes use batched INSERTs
	// to minimize round-trips and stream expiry exposure.
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(30 * time.Second)

	// Verify connectivity
	if err := db.Ping(); err != nil {
		connector.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return &Client{
		DB:        db,
		connector: connector,
		logger:    logger,
	}, nil
}

// Sync manually triggers a sync with the remote Turso database.
func (c *Client) Sync() error {
	rep, err := c.connector.Sync()
	if err != nil {
		return fmt.Errorf("syncing database: %w", err)
	}
	c.logger.Debug("Database synced", slog.Int("framesSynced", rep.FramesSynced))
	return nil
}

// Close shuts down the database connection and connector.
func (c *Client) Close() error {
	if err := c.DB.Close(); err != nil {
		return fmt.Errorf("closing database: %w", err)
	}
	if err := c.connector.Close(); err != nil {
		return fmt.Errorf("closing connector: %w", err)
	}
	return nil
}
