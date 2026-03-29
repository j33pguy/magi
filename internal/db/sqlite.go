package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	_ "github.com/tursodatabase/go-libsql" // registers "libsql" driver
)

// SQLiteClient implements Store using a local libSQL/SQLite file.
// Embeds TursoClient to reuse all query methods — the only difference
// is construction (local file, no remote sync) and teardown.
type SQLiteClient struct {
	*TursoClient
	path string
}

// NewSQLiteClient opens a local libSQL database at path.
// Uses the go-libsql driver so vector search and FTS5 are available.
func NewSQLiteClient(path string, logger *slog.Logger) (*SQLiteClient, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating directory: %w", err)
	}

	db, err := sql.Open("libsql", "file:"+path)
	if err != nil {
		return nil, fmt.Errorf("opening SQLite database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging SQLite database: %w", err)
	}

	return &SQLiteClient{
		TursoClient: &TursoClient{
			DB:     db,
			logger: logger,
		},
		path: path,
	}, nil
}

// Close shuts down the local database connection.
func (c *SQLiteClient) Close() error {
	return c.DB.Close()
}

// Sync is a no-op for local SQLite — there is no remote to sync with.
func (c *SQLiteClient) Sync() error {
	c.logger.Debug("Sync called on SQLite backend (no-op)")
	return nil
}
