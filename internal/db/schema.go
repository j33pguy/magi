package db

import (
	"fmt"
	"strings"
)

// Schema handles database migrations.
type Schema struct {
	client *Client
}

// Migrate runs all database migrations. Safe to call on every startup.
func (c *Client) Migrate() error {
	s := &Schema{client: c}
	if err := s.run(); err != nil {
		return err
	}
	// Apply orchestration tables (agent registry + task management)
	return c.RunOrchestrationMigrations()
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
		{4, migrationV4},
		{5, migrationV5},
		{6, migrationV6},
		{7, migrationV7},
	}

	for _, m := range migrations {
		applied, err := s.isApplied(m.version)
		if err != nil {
			return fmt.Errorf("checking migration %d: %w", m.version, err)
		}
		if applied {
			continue
		}

		if err := s.execMulti(m.sql); err != nil {
			return fmt.Errorf("running migration %d: %w", m.version, err)
		}

		if err := s.markApplied(m.version); err != nil {
			return fmt.Errorf("marking migration %d: %w", m.version, err)
		}

		s.client.logger.Info("Applied migration", "version", m.version)
	}

	return nil
}

// execMulti splits a SQL string into individual statements and executes each
// one separately. Turso (libSQL over Hrana) rejects multi-statement strings.
//
// Splitting is semicolon-based but aware of BEGIN...END blocks (triggers),
// so trigger bodies are not split mid-statement.
func (s *Schema) execMulti(sql string) error {
	statements := splitSQL(sql)
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := s.client.DB.Exec(stmt); err != nil {
			return fmt.Errorf("executing statement: %w\nSQL: %s", err, stmt)
		}
	}
	return nil
}

// splitSQL splits a SQL string into individual statements on semicolons,
// but treats BEGIN...END blocks as atomic (for triggers/procedures).
func splitSQL(sql string) []string {
	var stmts []string
	var current strings.Builder
	depth := 0
	upper := strings.ToUpper(sql)

	for i := 0; i < len(sql); i++ {
		ch := sql[i]

		// Track BEGIN...END depth (case-insensitive)
		if i+5 <= len(upper) && upper[i:i+5] == "BEGIN" {
			depth++
		}
		if i+3 <= len(upper) && upper[i:i+3] == "END" {
			if depth > 0 {
				depth--
			}
		}

		if ch == ';' && depth == 0 {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				stmts = append(stmts, stmt)
			}
			current.Reset()
			continue
		}
		current.WriteByte(ch)
	}
	if stmt := strings.TrimSpace(current.String()); stmt != "" {
		stmts = append(stmts, stmt)
	}
	return stmts
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

// migrationV4 introduces rich memory types (Issue #8).
// Renames the old default 'note' to 'memory' and adds an index on (type, created_at)
// to speed up type-filtered queries used by recall_incidents / recall_lessons.
const migrationV4 = `
UPDATE memories SET type = 'memory' WHERE type = 'note';
CREATE INDEX IF NOT EXISTS idx_memories_type_created ON memories(type, created_at DESC);
`

// migrationV5 adds structured taxonomy fields for categorized recall.
// speaker: who said/wrote this (user, assistant, agent, system)
// area: top-level domain (work, home, family, homelab, project, meta)
// sub_area: sub-domain, free-form (power-platform, proxmox, magi, etc.)
const migrationV5 = `
ALTER TABLE memories ADD COLUMN speaker TEXT NOT NULL DEFAULT '';
ALTER TABLE memories ADD COLUMN area TEXT NOT NULL DEFAULT '';
ALTER TABLE memories ADD COLUMN sub_area TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_memories_speaker ON memories(speaker) WHERE speaker != '';
CREATE INDEX IF NOT EXISTS idx_memories_area ON memories(area) WHERE area != '';
CREATE INDEX IF NOT EXISTS idx_memories_area_sub ON memories(area, sub_area) WHERE area != '';
`

// migrationV6 adds a dedicated index on created_at for temporal queries
// (after/before time filtering).
const migrationV6 = `
CREATE INDEX IF NOT EXISTS idx_memories_created_at ON memories(created_at);
`

// migrationV7 adds memory_links table for explicit memory-to-memory relationships.
// Enables graph traversal: "this decision caused that outcome", "this supersedes that".
const migrationV7 = `
CREATE TABLE IF NOT EXISTS memory_links (
	id          TEXT NOT NULL PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
	from_id     TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
	to_id       TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
	relation    TEXT NOT NULL CHECK(relation IN ('caused_by','led_to','related_to','supersedes','part_of','contradicts')),
	weight      REAL NOT NULL DEFAULT 1.0,
	auto        INTEGER NOT NULL DEFAULT 0,
	created_at  TEXT NOT NULL DEFAULT (datetime('now')),
	UNIQUE(from_id, to_id, relation)
);
CREATE INDEX IF NOT EXISTS idx_memory_links_from ON memory_links(from_id);
CREATE INDEX IF NOT EXISTS idx_memory_links_to ON memory_links(to_id);
`

// migrationV2 adds visibility field for access control.
// Values: "private" (owner only, never via HTTP API), "internal" (default, accessible within the system), "public" (no restrictions)
// Private memories: MEMORY.md, USER.md, credentials, family info — never exposed via HTTP
// Internal memories: code context, decisions, runbooks — accessible to all connected agents
// Public memories: shareable context, project docs — safe for any consumer
const migrationV2 = `
ALTER TABLE memories ADD COLUMN visibility TEXT NOT NULL DEFAULT 'internal'
	CHECK(visibility IN ('private', 'internal', 'public'));

CREATE INDEX IF NOT EXISTS idx_memories_visibility ON memories(visibility, archived_at);
`
