package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

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
// Configures WAL mode and connection pool settings for concurrent access.
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

	// Enable WAL mode for concurrent read/write access.
	// Some PRAGMAs return rows (journal_mode), others don't (synchronous).
	// Use Query+Close to handle both cases and avoid "Execute returned rows"
	// errors with drivers that reject Exec on row-returning statements.
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA cache_size=-2000", // 2MB cache
	}
	for _, pragma := range pragmas {
		rows, err := db.Query(pragma)
		if err != nil {
			logger.Warn("PRAGMA failed (non-fatal)", "pragma", pragma, "error", err)
			continue
		}
		rows.Close()
	}

	// Connection pool: allow more concurrent readers with WAL mode.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(10 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

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
