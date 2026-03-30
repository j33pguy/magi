package db

// SQL Server / Azure SQL schema migrations.
// Mirrors the SQLite/Turso migrations in schema.go but uses T-SQL syntax.
// Embeddings are stored as VARBINARY(MAX) — vector search is computed in Go.
// Full-Text Search uses SQL Server Full-Text Indexing (catalog + index).

// mssqlMigrationV1 creates the core memories and memory_tags tables with indexes.
const mssqlMigrationV1 = `
IF OBJECT_ID('memories', 'U') IS NULL
CREATE TABLE memories (
	id          NVARCHAR(32)   NOT NULL,
	content     NVARCHAR(MAX)  NOT NULL,
	summary     NVARCHAR(MAX)  NULL,
	embedding   VARBINARY(MAX) NULL,

	project     NVARCHAR(255)  NOT NULL,
	type        NVARCHAR(64)   NOT NULL DEFAULT 'memory',

	source      NVARCHAR(MAX)  NULL,
	source_file NVARCHAR(MAX)  NULL,
	parent_id   NVARCHAR(32)   NULL,
	chunk_index INT            NOT NULL DEFAULT 0,

	created_at  DATETIME2      NOT NULL DEFAULT SYSUTCDATETIME(),
	updated_at  DATETIME2      NOT NULL DEFAULT SYSUTCDATETIME(),
	archived_at DATETIME2      NULL,
	token_count INT            NULL,

	CONSTRAINT PK_memories PRIMARY KEY (id),
	CONSTRAINT FK_memories_parent FOREIGN KEY (parent_id) REFERENCES memories(id)
);

IF OBJECT_ID('memory_tags', 'U') IS NULL
CREATE TABLE memory_tags (
	memory_id NVARCHAR(32)  NOT NULL,
	tag       NVARCHAR(255) NOT NULL,
	CONSTRAINT PK_memory_tags PRIMARY KEY (memory_id, tag),
	CONSTRAINT FK_memory_tags_memory FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE
);

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_memories_project' AND object_id = OBJECT_ID('memories'))
	CREATE INDEX idx_memories_project ON memories(project, archived_at);

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_memories_type' AND object_id = OBJECT_ID('memories'))
	CREATE INDEX idx_memories_type ON memories(type, archived_at);

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_memories_created' AND object_id = OBJECT_ID('memories'))
	CREATE INDEX idx_memories_created ON memories(created_at DESC);

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_memories_parent' AND object_id = OBJECT_ID('memories'))
	CREATE INDEX idx_memories_parent ON memories(parent_id);

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_tags_tag' AND object_id = OBJECT_ID('memory_tags'))
	CREATE INDEX idx_tags_tag ON memory_tags(tag);
`

// mssqlMigrationV2 adds the visibility column for access control.
const mssqlMigrationV2 = `
IF COL_LENGTH('memories', 'visibility') IS NULL
	ALTER TABLE memories ADD visibility NVARCHAR(20) NOT NULL DEFAULT 'internal';

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_memories_visibility' AND object_id = OBJECT_ID('memories'))
	CREATE INDEX idx_memories_visibility ON memories(visibility, archived_at);
`

// mssqlMigrationV3 creates a Full-Text catalog and index on the content column.
const mssqlMigrationV3 = `
IF NOT EXISTS (SELECT 1 FROM sys.fulltext_catalogs WHERE name = 'magi_ftcat')
	CREATE FULLTEXT CATALOG magi_ftcat AS DEFAULT;

IF NOT EXISTS (SELECT 1 FROM sys.fulltext_indexes WHERE object_id = OBJECT_ID('memories'))
	CREATE FULLTEXT INDEX ON memories(content) KEY INDEX PK_memories ON magi_ftcat;
`

// mssqlMigrationV4 renames old default 'note' to 'memory' and adds type+created index.
const mssqlMigrationV4 = `
UPDATE memories SET type = 'memory' WHERE type = 'note';

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_memories_type_created' AND object_id = OBJECT_ID('memories'))
	CREATE INDEX idx_memories_type_created ON memories(type, created_at DESC);
`

// mssqlMigrationV5 adds structured taxonomy fields: speaker, area, sub_area.
const mssqlMigrationV5 = `
IF COL_LENGTH('memories', 'speaker') IS NULL
	ALTER TABLE memories ADD speaker NVARCHAR(64) NOT NULL DEFAULT '';

IF COL_LENGTH('memories', 'area') IS NULL
	ALTER TABLE memories ADD area NVARCHAR(64) NOT NULL DEFAULT '';

IF COL_LENGTH('memories', 'sub_area') IS NULL
	ALTER TABLE memories ADD sub_area NVARCHAR(128) NOT NULL DEFAULT '';

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_memories_speaker' AND object_id = OBJECT_ID('memories'))
	CREATE INDEX idx_memories_speaker ON memories(speaker);

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_memories_area' AND object_id = OBJECT_ID('memories'))
	CREATE INDEX idx_memories_area ON memories(area);

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_memories_area_sub' AND object_id = OBJECT_ID('memories'))
	CREATE INDEX idx_memories_area_sub ON memories(area, sub_area);
`

// mssqlMigrationV6 adds a dedicated index on created_at for temporal queries.
const mssqlMigrationV6 = `
IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_memories_created_at' AND object_id = OBJECT_ID('memories'))
	CREATE INDEX idx_memories_created_at ON memories(created_at);
`

// mssqlMigrationV7 adds memory_links table for explicit memory-to-memory relationships.
const mssqlMigrationV7 = `
IF OBJECT_ID('memory_links', 'U') IS NULL
CREATE TABLE memory_links (
	id          NVARCHAR(32)     NOT NULL,
	from_id     NVARCHAR(32)     NOT NULL,
	to_id       NVARCHAR(32)     NOT NULL,
	relation    NVARCHAR(32)     NOT NULL,
	weight      FLOAT            NOT NULL DEFAULT 1.0,
	auto        BIT              NOT NULL DEFAULT 0,
	created_at  DATETIME2        NOT NULL DEFAULT SYSUTCDATETIME(),

	CONSTRAINT PK_memory_links PRIMARY KEY (id),
	CONSTRAINT FK_memory_links_from FOREIGN KEY (from_id) REFERENCES memories(id) ON DELETE CASCADE,
	CONSTRAINT FK_memory_links_to FOREIGN KEY (to_id) REFERENCES memories(id),
	CONSTRAINT CK_memory_links_relation CHECK(relation IN ('caused_by','led_to','related_to','supersedes','part_of','contradicts')),
	CONSTRAINT UQ_memory_links UNIQUE (from_id, to_id, relation)
);

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_memory_links_from' AND object_id = OBJECT_ID('memory_links'))
	CREATE INDEX idx_memory_links_from ON memory_links(from_id);

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_memory_links_to' AND object_id = OBJECT_ID('memory_links'))
	CREATE INDEX idx_memory_links_to ON memory_links(to_id);
`
