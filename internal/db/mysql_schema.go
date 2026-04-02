package db

// mysqlMigrationV1 creates the core memories and memory_tags tables.
// Uses LONGBLOB for embedding storage (MySQL has no native vector type),
// LONGTEXT for content with FULLTEXT index for BM25 search,
// and CHAR(32) for IDs (UUIDs generated in Go).
const mysqlMigrationV1 = `
CREATE TABLE IF NOT EXISTS memories (
	id CHAR(32) NOT NULL PRIMARY KEY,
	content LONGTEXT NOT NULL,
	summary TEXT,
	embedding LONGBLOB,

	project VARCHAR(255) NOT NULL,
	type VARCHAR(64) NOT NULL DEFAULT 'memory',

	source VARCHAR(255),
	source_file VARCHAR(512),
	parent_id CHAR(32),
	chunk_index INT NOT NULL DEFAULT 0,

	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	archived_at DATETIME,
	token_count INT,

	FOREIGN KEY (parent_id) REFERENCES memories(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS memory_tags (
	memory_id CHAR(32) NOT NULL,
	tag VARCHAR(255) NOT NULL,
	PRIMARY KEY (memory_id, tag),
	FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE FULLTEXT INDEX idx_memories_content ON memories(content);
CREATE INDEX idx_memories_project ON memories(project, archived_at);
CREATE INDEX idx_memories_type ON memories(type, archived_at);
CREATE INDEX idx_memories_created ON memories(created_at);
CREATE INDEX idx_memories_parent ON memories(parent_id);
CREATE INDEX idx_tags_tag ON memory_tags(tag);
`

// mysqlMigrationV2 adds the visibility column for access control.
const mysqlMigrationV2 = `
ALTER TABLE memories ADD COLUMN visibility VARCHAR(16) NOT NULL DEFAULT 'internal';
CREATE INDEX idx_memories_visibility ON memories(visibility, archived_at);
`

// mysqlMigrationV3 is a no-op for MySQL — FULLTEXT is built into V1.
const mysqlMigrationV3 = ``

// mysqlMigrationV4 renames old default 'note' to 'memory' and adds type+created index.
const mysqlMigrationV4 = `
UPDATE memories SET type = 'memory' WHERE type = 'note';
CREATE INDEX idx_memories_type_created ON memories(type, created_at);
`

// mysqlMigrationV5 adds structured taxonomy fields: speaker, area, sub_area.
const mysqlMigrationV5 = `
ALTER TABLE memories ADD COLUMN speaker VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE memories ADD COLUMN area VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE memories ADD COLUMN sub_area VARCHAR(128) NOT NULL DEFAULT '';
CREATE INDEX idx_memories_speaker ON memories(speaker);
CREATE INDEX idx_memories_area ON memories(area);
CREATE INDEX idx_memories_area_sub ON memories(area, sub_area);
`

// mysqlMigrationV6 adds a dedicated index on created_at for temporal queries.
const mysqlMigrationV6 = `
CREATE INDEX idx_memories_created_at ON memories(created_at);
`

// mysqlMigrationV7 adds memory_links table for explicit memory-to-memory relationships.
const mysqlMigrationV7 = `
CREATE TABLE IF NOT EXISTS memory_links (
	id          CHAR(32) NOT NULL PRIMARY KEY,
	from_id     CHAR(32) NOT NULL,
	to_id       CHAR(32) NOT NULL,
	relation    VARCHAR(32) NOT NULL,
	weight      DOUBLE NOT NULL DEFAULT 1.0,
	auto        TINYINT(1) NOT NULL DEFAULT 0,
	created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE KEY uq_link (from_id, to_id, relation),
	FOREIGN KEY (from_id) REFERENCES memories(id) ON DELETE CASCADE,
	FOREIGN KEY (to_id) REFERENCES memories(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE INDEX idx_memory_links_from ON memory_links(from_id);
CREATE INDEX idx_memory_links_to ON memory_links(to_id);
`

const mysqlMigrationV8 = `
CREATE TABLE IF NOT EXISTS machine_credentials (
	id CHAR(32) NOT NULL PRIMARY KEY,
	token_hash VARCHAR(128) NOT NULL UNIQUE,
	user_name VARCHAR(255) NOT NULL,
	machine_id VARCHAR(255) NOT NULL,
	agent_name VARCHAR(255) NOT NULL DEFAULT '',
	agent_type VARCHAR(255) NOT NULL DEFAULT '',
	groups_json TEXT NOT NULL,
	display_name VARCHAR(255) NOT NULL DEFAULT '',
	description TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_seen_at DATETIME NULL,
	revoked_at DATETIME NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE INDEX idx_machine_credentials_machine ON machine_credentials(machine_id);
CREATE INDEX idx_machine_credentials_user ON machine_credentials(user_name);
CREATE INDEX idx_machine_credentials_revoked ON machine_credentials(revoked_at);
`

const mysqlMigrationV9 = `
CREATE TABLE IF NOT EXISTS tasks (
	id CHAR(32) NOT NULL PRIMARY KEY,
	project VARCHAR(255) NOT NULL DEFAULT '',
	queue_name VARCHAR(255) NOT NULL DEFAULT 'default',
	title VARCHAR(255) NOT NULL,
	summary TEXT NOT NULL,
	description LONGTEXT NOT NULL,
	status VARCHAR(32) NOT NULL DEFAULT 'queued',
	priority VARCHAR(32) NOT NULL DEFAULT 'normal',
	created_by VARCHAR(255) NOT NULL DEFAULT '',
	orchestrator VARCHAR(255) NOT NULL DEFAULT '',
	worker VARCHAR(255) NOT NULL DEFAULT '',
	parent_task_id CHAR(32) NULL,
	metadata_json LONGTEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	started_at DATETIME NULL,
	completed_at DATETIME NULL,
	failed_at DATETIME NULL,
	blocked_at DATETIME NULL,
	FOREIGN KEY (parent_task_id) REFERENCES tasks(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE INDEX idx_tasks_status_created ON tasks(status, created_at);
CREATE INDEX idx_tasks_project_status ON tasks(project, status);
CREATE INDEX idx_tasks_queue_status ON tasks(queue_name, status);
CREATE INDEX idx_tasks_worker_status ON tasks(worker, status);
CREATE INDEX idx_tasks_orchestrator_status ON tasks(orchestrator, status);

CREATE TABLE IF NOT EXISTS task_events (
	id CHAR(32) NOT NULL PRIMARY KEY,
	task_id CHAR(32) NOT NULL,
	event_type VARCHAR(32) NOT NULL,
	actor_role VARCHAR(64) NOT NULL DEFAULT '',
	actor_name VARCHAR(255) NOT NULL DEFAULT '',
	actor_user VARCHAR(255) NOT NULL DEFAULT '',
	actor_machine VARCHAR(255) NOT NULL DEFAULT '',
	actor_agent VARCHAR(255) NOT NULL DEFAULT '',
	summary TEXT NOT NULL,
	content LONGTEXT NOT NULL,
	status VARCHAR(32) NOT NULL DEFAULT '',
	memory_id CHAR(32) NULL,
	source VARCHAR(255) NOT NULL DEFAULT '',
	metadata_json LONGTEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
	FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE INDEX idx_task_events_task_created ON task_events(task_id, created_at);
CREATE INDEX idx_task_events_type_created ON task_events(event_type, created_at);
CREATE INDEX idx_task_events_memory ON task_events(memory_id);
`
