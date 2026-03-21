package db

import "fmt"

// Schema handles database migrations.
type Schema struct {
	client *Client
}

// Migrate runs all database migrations. Safe to call on every startup.
func (c *Client) Migrate() error {
	s := &Schema{client: c}
	return s.run()
}

func (s *Schema) run() error {
	if err := s.createMetaTable(); err != nil {
		return fmt.Errorf("creating meta table: %w", err)
	}

	migrations := []struct {
		version int
		sql     string
	}{
		{1, migrationV1},
		{2, migrationV2},
		{3, migrationV3},
	}

	for _, m := range migrations {
		applied, err := s.isApplied(m.version)
		if err != nil {
			return fmt.Errorf("checking migration %d: %w", m.version, err)
		}
		if applied {
			continue
		}

		if _, err := s.client.DB.Exec(m.sql); err != nil {
			return fmt.Errorf("running migration %d: %w", m.version, err)
		}

		if err := s.markApplied(m.version); err != nil {
			return fmt.Errorf("marking migration %d: %w", m.version, err)
		}

		s.client.logger.Info("Applied migration", "version", m.version)
	}

	return nil
}

func (s *Schema) createMetaTable() error {
	_, err := s.client.DB.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`)
	return err
}

func (s *Schema) isApplied(version int) (bool, error) {
	var count int
	err := s.client.DB.QueryRow(
		"SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version,
	).Scan(&count)
	return count > 0, err
}

func (s *Schema) markApplied(version int) error {
	_, err := s.client.DB.Exec(
		"INSERT INTO schema_migrations (version) VALUES (?)", version,
	)
	return err
}

const migrationV1 = `
CREATE TABLE IF NOT EXISTS memories (
	id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
	content TEXT NOT NULL,
	summary TEXT,
	embedding F32_BLOB(384),

	project TEXT NOT NULL,
	type TEXT NOT NULL DEFAULT 'note',

	source TEXT,
	source_file TEXT,
	parent_id TEXT,
	chunk_index INTEGER DEFAULT 0,

	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	updated_at TEXT NOT NULL DEFAULT (datetime('now')),
	archived_at TEXT,
	token_count INTEGER,

	FOREIGN KEY (parent_id) REFERENCES memories(id)
);

CREATE TABLE IF NOT EXISTS memory_tags (
	memory_id TEXT NOT NULL,
	tag TEXT NOT NULL,
	PRIMARY KEY (memory_id, tag),
	FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_memories_embedding ON memories(
	libsql_vector_idx(embedding, 'metric=cosine', 'compress_neighbors=float8', 'max_neighbors=20')
);
CREATE INDEX IF NOT EXISTS idx_memories_project ON memories(project, archived_at);
CREATE INDEX IF NOT EXISTS idx_memories_type ON memories(type, archived_at);
CREATE INDEX IF NOT EXISTS idx_memories_created ON memories(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_memories_parent ON memories(parent_id);
CREATE INDEX IF NOT EXISTS idx_tags_tag ON memory_tags(tag);
`

// migrationV3 adds FTS5 full-text search over memory content.
// Used for BM25 keyword search in hybrid retrieval (vector + keyword via RRF fusion).
// Triggers keep the FTS index in sync with the memories table automatically.
const migrationV3 = `
CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
	content,
	content='memories',
	content_rowid='rowid'
);

-- Populate FTS from existing rows
INSERT INTO memories_fts(rowid, content) SELECT rowid, content FROM memories;

CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
	INSERT INTO memories_fts(rowid, content) VALUES (new.rowid, new.content);
END;

CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
	INSERT INTO memories_fts(memories_fts, rowid, content) VALUES('delete', old.rowid, old.content);
END;

CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
	INSERT INTO memories_fts(memories_fts, rowid, content) VALUES('delete', old.rowid, old.content);
	INSERT INTO memories_fts(rowid, content) VALUES (new.rowid, new.content);
END;
`

// migrationV2 adds visibility field for access control.
// Values: "private" (owner only, never via HTTP API), "internal" (default, accessible within the system), "public" (no restrictions)
// Private memories: MEMORY.md, USER.md, credentials, family info — never exposed via HTTP
// Internal memories: code context, decisions, runbooks — accessible to all Claude instances
// Public memories: shareable context, project docs — safe for any consumer
const migrationV2 = `
ALTER TABLE memories ADD COLUMN visibility TEXT NOT NULL DEFAULT 'internal'
	CHECK(visibility IN ('private', 'internal', 'public'));

CREATE INDEX IF NOT EXISTS idx_memories_visibility ON memories(visibility, archived_at);
`
