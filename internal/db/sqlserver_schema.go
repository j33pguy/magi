package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// SQLServerSchema handles SQL Server database migrations.
type SQLServerSchema struct {
	db *sql.DB
}

// runSQLServerMigrations runs all T-SQL migrations. Safe to call on every startup.
func runSQLServerMigrations(db *sql.DB) error {
	s := &SQLServerSchema{db: db}
	return s.run()
}

func (s *SQLServerSchema) run() error {
	if err := s.createMetaTable(); err != nil {
		return fmt.Errorf("creating meta table: %w", err)
	}

	migrations := []struct {
		version int
		fn      func() error
	}{
		{1, s.migrationV1},
		{2, s.migrationV2},
		{3, s.migrationV3},
		{4, s.migrationV4},
		{5, s.migrationV5},
		{6, s.migrationV6},
		{7, s.migrationV7},
		{8, s.migrationV8},
		{9, s.migrationV9},
	}

	for _, m := range migrations {
		applied, err := s.isApplied(m.version)
		if err != nil {
			return fmt.Errorf("checking migration %d: %w", m.version, err)
		}
		if applied {
			continue
		}

		if err := m.fn(); err != nil {
			return fmt.Errorf("running migration %d: %w", m.version, err)
		}

		if err := s.markApplied(m.version); err != nil {
			return fmt.Errorf("marking migration %d: %w", m.version, err)
		}
	}

	return nil
}

func (s *SQLServerSchema) createMetaTable() error {
	_, err := s.db.Exec(`
		IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'schema_migrations')
		CREATE TABLE schema_migrations (
			version INT PRIMARY KEY,
			applied_at DATETIME2 NOT NULL DEFAULT GETUTCDATE()
		)
	`)
	return err
}

func (s *SQLServerSchema) isApplied(version int) (bool, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM schema_migrations WHERE version = @p1", version,
	).Scan(&count)
	return count > 0, err
}

func (s *SQLServerSchema) markApplied(version int) error {
	_, err := s.db.Exec(
		"INSERT INTO schema_migrations (version) VALUES (@p1)", version,
	)
	return err
}

// execStatements splits on GO delimiters and executes each batch.
func (s *SQLServerSchema) execStatements(stmts []string) error {
	for _, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("executing statement: %w\nSQL: %s", err, stmt)
		}
	}
	return nil
}

// migrationV1: core tables (memories, memory_tags) and indexes.
func (s *SQLServerSchema) migrationV1() error {
	return s.execStatements([]string{
		// memories table
		`IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'memories')
		CREATE TABLE memories (
			id NVARCHAR(32) NOT NULL PRIMARY KEY DEFAULT LOWER(REPLACE(NEWID(), '-', '')),
			content NVARCHAR(MAX) NOT NULL,
			summary NVARCHAR(MAX) NULL,
			embedding VARBINARY(MAX) NULL,

			project NVARCHAR(4000) NOT NULL,
			type NVARCHAR(100) NOT NULL DEFAULT 'note',

			source NVARCHAR(4000) NULL,
			source_file NVARCHAR(4000) NULL,
			parent_id NVARCHAR(32) NULL,
			chunk_index INT DEFAULT 0,

			created_at DATETIME2 NOT NULL DEFAULT GETUTCDATE(),
			updated_at DATETIME2 NOT NULL DEFAULT GETUTCDATE(),
			archived_at DATETIME2 NULL,
			token_count INT NULL,

			FOREIGN KEY (parent_id) REFERENCES memories(id)
		)`,

		// memory_tags table
		`IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'memory_tags')
		CREATE TABLE memory_tags (
			memory_id NVARCHAR(32) NOT NULL,
			tag NVARCHAR(4000) NOT NULL,
			PRIMARY KEY (memory_id, tag),
			FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE
		)`,

		// Indexes (skip vector index — done in Go-side cosine similarity)
		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_memories_project')
		CREATE INDEX idx_memories_project ON memories(project, archived_at)`,

		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_memories_type')
		CREATE INDEX idx_memories_type ON memories(type, archived_at)`,

		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_memories_created')
		CREATE INDEX idx_memories_created ON memories(created_at DESC)`,

		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_memories_parent')
		CREATE INDEX idx_memories_parent ON memories(parent_id)`,

		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_tags_tag')
		CREATE INDEX idx_tags_tag ON memory_tags(tag)`,
	})
}

// migrationV2: visibility field for access control.
func (s *SQLServerSchema) migrationV2() error {
	return s.execStatements([]string{
		`IF NOT EXISTS (SELECT * FROM sys.columns WHERE object_id = OBJECT_ID('memories') AND name = 'visibility')
		ALTER TABLE memories ADD visibility NVARCHAR(20) NOT NULL DEFAULT 'internal'`,

		`IF NOT EXISTS (SELECT * FROM sys.check_constraints WHERE name = 'CK_memories_visibility')
		ALTER TABLE memories ADD CONSTRAINT CK_memories_visibility
			CHECK (visibility IN ('private', 'internal', 'team', 'shared', 'public'))`,

		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_memories_visibility')
		CREATE INDEX idx_memories_visibility ON memories(visibility, archived_at)`,
	})
}

// migrationV3: Full-Text Search catalog and index.
// SQL Server FTS replaces SQLite FTS5. Requires a unique single-column index.
func (s *SQLServerSchema) migrationV3() error {
	return s.execStatements([]string{
		// Full-Text Catalog
		`IF NOT EXISTS (SELECT * FROM sys.fulltext_catalogs WHERE name = 'magi_fts_catalog')
		CREATE FULLTEXT CATALOG magi_fts_catalog AS DEFAULT`,

		// Full-Text Index on content column (requires unique index on the key)
		// The PK (id) already provides the unique index for the FT key.
		`IF NOT EXISTS (SELECT * FROM sys.fulltext_indexes WHERE object_id = OBJECT_ID('memories'))
		CREATE FULLTEXT INDEX ON memories(content)
			KEY INDEX PK__memories__id
			ON magi_fts_catalog
			WITH CHANGE_TRACKING AUTO`,
	})
}

// migrationV4: rename old default type 'note' to 'memory'.
func (s *SQLServerSchema) migrationV4() error {
	return s.execStatements([]string{
		`UPDATE memories SET type = 'memory' WHERE type = 'note'`,

		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_memories_type_created')
		CREATE INDEX idx_memories_type_created ON memories(type, created_at DESC)`,
	})
}

// migrationV5: taxonomy fields (speaker, area, sub_area).
func (s *SQLServerSchema) migrationV5() error {
	return s.execStatements([]string{
		`IF NOT EXISTS (SELECT * FROM sys.columns WHERE object_id = OBJECT_ID('memories') AND name = 'speaker')
		ALTER TABLE memories ADD speaker NVARCHAR(100) NOT NULL DEFAULT ''`,

		`IF NOT EXISTS (SELECT * FROM sys.columns WHERE object_id = OBJECT_ID('memories') AND name = 'area')
		ALTER TABLE memories ADD area NVARCHAR(100) NOT NULL DEFAULT ''`,

		`IF NOT EXISTS (SELECT * FROM sys.columns WHERE object_id = OBJECT_ID('memories') AND name = 'sub_area')
		ALTER TABLE memories ADD sub_area NVARCHAR(100) NOT NULL DEFAULT ''`,

		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_memories_speaker')
		CREATE INDEX idx_memories_speaker ON memories(speaker) WHERE speaker != ''`,

		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_memories_area')
		CREATE INDEX idx_memories_area ON memories(area) WHERE area != ''`,

		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_memories_area_sub')
		CREATE INDEX idx_memories_area_sub ON memories(area, sub_area) WHERE area != ''`,
	})
}

// migrationV6: dedicated index on created_at for temporal queries.
func (s *SQLServerSchema) migrationV6() error {
	return s.execStatements([]string{
		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_memories_created_at')
		CREATE INDEX idx_memories_created_at ON memories(created_at)`,
	})
}

// migrationV7: memory_links table for graph relationships.
func (s *SQLServerSchema) migrationV7() error {
	return s.execStatements([]string{
		`IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'memory_links')
		CREATE TABLE memory_links (
			id NVARCHAR(32) NOT NULL PRIMARY KEY DEFAULT LOWER(REPLACE(NEWID(), '-', '')),
			from_id NVARCHAR(32) NOT NULL REFERENCES memories(id),
			to_id NVARCHAR(32) NOT NULL REFERENCES memories(id),
			relation NVARCHAR(50) NOT NULL,
			weight FLOAT NOT NULL DEFAULT 1.0,
			auto BIT NOT NULL DEFAULT 0,
			created_at DATETIME2 NOT NULL DEFAULT GETUTCDATE(),
			UNIQUE(from_id, to_id, relation)
		)`,

		`IF NOT EXISTS (SELECT * FROM sys.check_constraints WHERE name = 'CK_memory_links_relation')
		ALTER TABLE memory_links ADD CONSTRAINT CK_memory_links_relation
			CHECK (relation IN ('caused_by','led_to','related_to','supersedes','part_of','contradicts'))`,

		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_memory_links_from')
		CREATE INDEX idx_memory_links_from ON memory_links(from_id)`,

		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_memory_links_to')
		CREATE INDEX idx_memory_links_to ON memory_links(to_id)`,
	})
}

func (s *SQLServerSchema) migrationV8() error {
	return s.execStatements([]string{
		`IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'machine_credentials')
		CREATE TABLE machine_credentials (
			id NVARCHAR(32) NOT NULL PRIMARY KEY DEFAULT LOWER(REPLACE(NEWID(), '-', '')),
			token_hash NVARCHAR(128) NOT NULL UNIQUE,
			user_name NVARCHAR(255) NOT NULL,
			machine_id NVARCHAR(255) NOT NULL,
			agent_name NVARCHAR(255) NOT NULL DEFAULT '',
			agent_type NVARCHAR(255) NOT NULL DEFAULT '',
			groups_json NVARCHAR(MAX) NOT NULL DEFAULT '[]',
			display_name NVARCHAR(255) NOT NULL DEFAULT '',
			description NVARCHAR(MAX) NOT NULL DEFAULT '',
			created_at DATETIME2 NOT NULL DEFAULT GETUTCDATE(),
			last_seen_at DATETIME2 NULL,
			revoked_at DATETIME2 NULL
		)`,
		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_machine_credentials_machine')
		CREATE INDEX idx_machine_credentials_machine ON machine_credentials(machine_id)`,
		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_machine_credentials_user')
		CREATE INDEX idx_machine_credentials_user ON machine_credentials(user_name)`,
		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_machine_credentials_revoked')
		CREATE INDEX idx_machine_credentials_revoked ON machine_credentials(revoked_at)`,
	})
}

func (s *SQLServerSchema) migrationV9() error {
	return s.execStatements([]string{
		`IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'tasks')
		CREATE TABLE tasks (
			id NVARCHAR(32) NOT NULL PRIMARY KEY DEFAULT LOWER(REPLACE(NEWID(), '-', '')),
			project NVARCHAR(255) NOT NULL DEFAULT '',
			queue_name NVARCHAR(255) NOT NULL DEFAULT 'default',
			title NVARCHAR(255) NOT NULL,
			summary NVARCHAR(MAX) NOT NULL DEFAULT '',
			description NVARCHAR(MAX) NOT NULL DEFAULT '',
			status NVARCHAR(32) NOT NULL DEFAULT 'queued',
			priority NVARCHAR(32) NOT NULL DEFAULT 'normal',
			created_by NVARCHAR(255) NOT NULL DEFAULT '',
			orchestrator NVARCHAR(255) NOT NULL DEFAULT '',
			worker NVARCHAR(255) NOT NULL DEFAULT '',
			parent_task_id NVARCHAR(32) NULL REFERENCES tasks(id),
			metadata_json NVARCHAR(MAX) NOT NULL DEFAULT '{}',
			created_at DATETIME2 NOT NULL DEFAULT GETUTCDATE(),
			updated_at DATETIME2 NOT NULL DEFAULT GETUTCDATE(),
			started_at DATETIME2 NULL,
			completed_at DATETIME2 NULL,
			failed_at DATETIME2 NULL,
			blocked_at DATETIME2 NULL
		)`,
		`IF NOT EXISTS (SELECT * FROM sys.check_constraints WHERE name = 'CK_tasks_status')
		ALTER TABLE tasks ADD CONSTRAINT CK_tasks_status
			CHECK (status IN ('queued','started','done','failed','blocked','canceled'))`,
		`IF NOT EXISTS (SELECT * FROM sys.check_constraints WHERE name = 'CK_tasks_priority')
		ALTER TABLE tasks ADD CONSTRAINT CK_tasks_priority
			CHECK (priority IN ('low','normal','high','urgent'))`,
		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_tasks_status_created')
		CREATE INDEX idx_tasks_status_created ON tasks(status, created_at)`,
		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_tasks_project_status')
		CREATE INDEX idx_tasks_project_status ON tasks(project, status)`,
		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_tasks_queue_status')
		CREATE INDEX idx_tasks_queue_status ON tasks(queue_name, status)`,
		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_tasks_worker_status')
		CREATE INDEX idx_tasks_worker_status ON tasks(worker, status)`,
		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_tasks_orchestrator_status')
		CREATE INDEX idx_tasks_orchestrator_status ON tasks(orchestrator, status)`,

		`IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'task_events')
		CREATE TABLE task_events (
			id NVARCHAR(32) NOT NULL PRIMARY KEY DEFAULT LOWER(REPLACE(NEWID(), '-', '')),
			task_id NVARCHAR(32) NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			event_type NVARCHAR(32) NOT NULL,
			actor_role NVARCHAR(64) NOT NULL DEFAULT '',
			actor_name NVARCHAR(255) NOT NULL DEFAULT '',
			actor_user NVARCHAR(255) NOT NULL DEFAULT '',
			actor_machine NVARCHAR(255) NOT NULL DEFAULT '',
			actor_agent NVARCHAR(255) NOT NULL DEFAULT '',
			summary NVARCHAR(MAX) NOT NULL DEFAULT '',
			content NVARCHAR(MAX) NOT NULL DEFAULT '',
			status NVARCHAR(32) NOT NULL DEFAULT '',
			memory_id NVARCHAR(32) NULL REFERENCES memories(id) ON DELETE SET NULL,
			source NVARCHAR(255) NOT NULL DEFAULT '',
			metadata_json NVARCHAR(MAX) NOT NULL DEFAULT '{}',
			created_at DATETIME2 NOT NULL DEFAULT GETUTCDATE()
		)`,
		`IF NOT EXISTS (SELECT * FROM sys.check_constraints WHERE name = 'CK_task_events_type')
		ALTER TABLE task_events ADD CONSTRAINT CK_task_events_type
			CHECK (event_type IN ('status','communication','issue','lesson','pitfall','success','memory_ref','note'))`,
		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_task_events_task_created')
		CREATE INDEX idx_task_events_task_created ON task_events(task_id, created_at)`,
		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_task_events_type_created')
		CREATE INDEX idx_task_events_type_created ON task_events(event_type, created_at)`,
		`IF NOT EXISTS (SELECT * FROM sys.indexes WHERE name = 'idx_task_events_memory')
		CREATE INDEX idx_task_events_memory ON task_events(memory_id)`,
	})
}
