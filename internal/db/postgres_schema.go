package db

// PostgreSQL schema migrations — mirrors the SQLite/Turso migrations in schema.go
// but uses PostgreSQL-native syntax: pgvector, tsvector, gen_random_uuid(), etc.

const pgMigrationV1 = `
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS memories (
	id          TEXT NOT NULL PRIMARY KEY DEFAULT gen_random_uuid()::text,
	content     TEXT NOT NULL,
	summary     TEXT,
	embedding   vector(384),

	project     TEXT NOT NULL,
	type        TEXT NOT NULL DEFAULT 'note',

	source      TEXT,
	source_file TEXT,
	parent_id   TEXT REFERENCES memories(id),
	chunk_index INTEGER DEFAULT 0,

	created_at  TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
	updated_at  TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
	archived_at TIMESTAMPTZ,
	token_count INTEGER
);

CREATE TABLE IF NOT EXISTS memory_tags (
	memory_id TEXT NOT NULL,
	tag       TEXT NOT NULL,
	PRIMARY KEY (memory_id, tag),
	FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_memories_embedding ON memories USING hnsw (embedding vector_cosine_ops);
CREATE INDEX IF NOT EXISTS idx_memories_project ON memories(project, archived_at);
CREATE INDEX IF NOT EXISTS idx_memories_type ON memories(type, archived_at);
CREATE INDEX IF NOT EXISTS idx_memories_created ON memories(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_memories_parent ON memories(parent_id);
CREATE INDEX IF NOT EXISTS idx_tags_tag ON memory_tags(tag);
`

// pgMigrationV2 adds visibility field for access control.
const pgMigrationV2 = `
ALTER TABLE memories ADD COLUMN visibility TEXT NOT NULL DEFAULT 'internal'
	CHECK(visibility IN ('private', 'internal', 'public'));

CREATE INDEX IF NOT EXISTS idx_memories_visibility ON memories(visibility, archived_at);
`

// pgMigrationV3 adds tsvector FTS column with trigger and GIN index.
const pgMigrationV3 = `
ALTER TABLE memories ADD COLUMN search_vector tsvector;

UPDATE memories SET search_vector = to_tsvector('english', content);

CREATE OR REPLACE FUNCTION memories_search_vector_update() RETURNS trigger AS $$
BEGIN
	NEW.search_vector := to_tsvector('english', NEW.content);
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER memories_search_vector_trigger
	BEFORE INSERT OR UPDATE OF content ON memories
	FOR EACH ROW
	EXECUTE FUNCTION memories_search_vector_update();

CREATE INDEX IF NOT EXISTS idx_memories_search_vector ON memories USING gin(search_vector);
`

// pgMigrationV4 renames old default 'note' to 'memory' and adds type+created index.
const pgMigrationV4 = `
UPDATE memories SET type = 'memory' WHERE type = 'note';
CREATE INDEX IF NOT EXISTS idx_memories_type_created ON memories(type, created_at DESC);
`

// pgMigrationV5 adds structured taxonomy fields.
const pgMigrationV5 = `
ALTER TABLE memories ADD COLUMN speaker TEXT NOT NULL DEFAULT '';
ALTER TABLE memories ADD COLUMN area TEXT NOT NULL DEFAULT '';
ALTER TABLE memories ADD COLUMN sub_area TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_memories_speaker ON memories(speaker) WHERE speaker != '';
CREATE INDEX IF NOT EXISTS idx_memories_area ON memories(area) WHERE area != '';
CREATE INDEX IF NOT EXISTS idx_memories_area_sub ON memories(area, sub_area) WHERE area != '';
`

// pgMigrationV6 adds a dedicated index on created_at for temporal queries.
const pgMigrationV6 = `
CREATE INDEX IF NOT EXISTS idx_memories_created_at ON memories(created_at);
`

// pgMigrationV7 adds memory_links table for explicit memory-to-memory relationships.
const pgMigrationV7 = `
CREATE TABLE IF NOT EXISTS memory_links (
	id          TEXT NOT NULL PRIMARY KEY DEFAULT gen_random_uuid()::text,
	from_id     TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
	to_id       TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
	relation    TEXT NOT NULL CHECK(relation IN ('caused_by','led_to','related_to','supersedes','part_of','contradicts')),
	weight      DOUBLE PRECISION NOT NULL DEFAULT 1.0,
	auto        BOOLEAN NOT NULL DEFAULT FALSE,
	created_at  TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
	UNIQUE(from_id, to_id, relation)
);
CREATE INDEX IF NOT EXISTS idx_memory_links_from ON memory_links(from_id);
CREATE INDEX IF NOT EXISTS idx_memory_links_to ON memory_links(to_id);
`
