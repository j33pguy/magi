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
