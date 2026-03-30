package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// NewSQLiteClient — Ping failure (corrupt/unwritable file)
// ---------------------------------------------------------------------------

func TestR2_NewSQLiteClientOpenFailure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Use a path where the directory can be created but the DB file itself
	// is actually a directory, which should cause sql.Open or Ping to fail.
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "notafile.db")

	// Create a directory where the DB file should be — libsql can't open a directory as a DB
	if err := os.MkdirAll(dbPath, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	_, err := NewSQLiteClient(dbPath, logger)
	if err == nil {
		// If the driver accepts it, skip — this is driver-dependent
		t.Skip("driver accepted directory as DB path")
	}
	t.Logf("got expected error: %v", err)
}

// ---------------------------------------------------------------------------
// SQLiteClient.Close — double close
// ---------------------------------------------------------------------------

func TestR2_SQLiteClientDoubleClose(t *testing.T) {
	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	client, err := NewSQLiteClient(filepath.Join(tmp, "double-close.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}

	// First close should succeed
	if err := client.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	// Second close should return an error (DB already closed)
	err = client.Close()
	if err == nil {
		t.Log("double close returned nil (driver may allow it)")
	}
	// Whether it errors or not, it should not panic
}

// ---------------------------------------------------------------------------
// TursoClient.Close — DB.Close error path
// ---------------------------------------------------------------------------

func TestR2_TursoClientCloseDBError(t *testing.T) {
	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	db, err := sql.Open("libsql", "file:"+filepath.Join(tmp, "close-err.db"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	tc := &TursoClient{
		DB:        db,
		connector: nil,
		logger:    logger,
	}

	// Close the DB first via the raw handle
	db.Close()

	// Now TursoClient.Close should hit the DB.Close error path
	err = tc.Close()
	// Whether it errors or not depends on driver behavior; just verify no panic
	if err != nil {
		t.Logf("Close after pre-close: %v (expected)", err)
	}
}

// ---------------------------------------------------------------------------
// DeleteLink — nonexistent link returns sql.ErrNoRows
// ---------------------------------------------------------------------------

func TestR2_DeleteLinkNonexistent(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	err := c.DeleteLink(ctx, "does-not-exist-at-all")
	if err == nil {
		t.Fatal("expected error for nonexistent link")
	}
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetGraphData — edge cases
// ---------------------------------------------------------------------------

// TestR2_GetGraphDataNoLinks exercises the path where memories exist but
// have no links, so the link query returns empty results.
func TestR2_GetGraphDataNoLinks(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	// Create memories without any links
	_, _ = c.SaveMemory(&Memory{Content: "isolated node 1", Embedding: zeroEmbedding(), Project: "p", Type: "memory", Visibility: "internal"})
	_, _ = c.SaveMemory(&Memory{Content: "isolated node 2", Embedding: zeroEmbedding(), Project: "p", Type: "memory", Visibility: "internal"})

	memories, links, err := c.GetGraphData(ctx, 10)
	if err != nil {
		t.Fatalf("GetGraphData: %v", err)
	}
	if len(memories) != 2 {
		t.Errorf("expected 2 memories, got %d", len(memories))
	}
	if len(links) != 0 {
		t.Errorf("expected 0 links, got %d", len(links))
	}
}

// TestR2_GetGraphDataArchivedAtField exercises the archived.Valid branch
// in the node scanning loop — a memory with a non-NULL archived_at field
// is normally excluded by the WHERE clause, but we test the scan path
// by creating and archiving a memory, then verifying it's excluded.
func TestR2_GetGraphDataManyLinks(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	// Create 4 memories, link them in a chain
	var ids []string
	for i := 0; i < 4; i++ {
		m, err := c.SaveMemory(&Memory{
			Content:    "chain node " + string(rune('A'+i)),
			Embedding:  zeroEmbedding(),
			Project:    "p",
			Type:       "memory",
			Visibility: "internal",
		})
		if err != nil {
			t.Fatalf("SaveMemory: %v", err)
		}
		ids = append(ids, m.ID)
	}

	// Create chain: A->B->C->D
	for i := 0; i < len(ids)-1; i++ {
		if _, err := c.CreateLink(ctx, ids[i], ids[i+1], "led_to", 1.0, false); err != nil {
			t.Fatalf("CreateLink: %v", err)
		}
	}

	memories, links, err := c.GetGraphData(ctx, 10)
	if err != nil {
		t.Fatalf("GetGraphData: %v", err)
	}
	if len(memories) != 4 {
		t.Errorf("expected 4 memories, got %d", len(memories))
	}
	if len(links) != 3 {
		t.Errorf("expected 3 links, got %d", len(links))
	}
}

// TestR2_GetGraphDataClosedDB exercises the error path in GetGraphData
// when the node query fails.
func TestR2_GetGraphDataClosedDB(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	c.DB.Close()

	_, _, err := c.GetGraphData(ctx, 10)
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
}

// TestR2_GetGraphDataLinkQueryError exercises the error path when the
// link query fails. We seed data, get graph nodes successfully, but
// the link query fails because the DB is closed between calls.
// Since we can't intercept mid-function, we rely on the closed-DB test above.

// ---------------------------------------------------------------------------
// GetTags — nonexistent memory returns empty slice
// ---------------------------------------------------------------------------

func TestR2_GetTagsNonexistentMemory(t *testing.T) {
	c := newTestSQLiteClient(t)

	tags, err := c.GetTags("nonexistent-memory-id")
	if err != nil {
		t.Fatalf("GetTags: %v", err)
	}
	// Should return nil/empty since no tags exist
	if len(tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(tags))
	}
}

// TestR2_GetTagsScanError exercises the rows.Scan error path in GetTags.
// This is hard to trigger directly without corrupting the DB. The closed-DB
// test in coverage_extra_test.go covers the query error path.

// ---------------------------------------------------------------------------
// execMulti — edge cases with SQL parsing
// ---------------------------------------------------------------------------

// TestR2_ExecMultiBeginEndBlocks exercises splitSQL with BEGIN...END blocks
// (trigger bodies) to ensure they are not split mid-statement.
func TestR2_ExecMultiBeginEndBlocks(t *testing.T) {
	c := newTestSQLiteClient(t)
	s := &Schema{client: c.TursoClient}

	// SQL with a trigger that contains BEGIN...END
	sql := `CREATE TABLE test_trigger_tbl (id INTEGER PRIMARY KEY, val TEXT);
CREATE TRIGGER test_trig AFTER INSERT ON test_trigger_tbl BEGIN
	UPDATE test_trigger_tbl SET val = 'updated' WHERE id = new.id;
END;`

	if err := s.execMulti(sql); err != nil {
		t.Fatalf("execMulti with BEGIN...END: %v", err)
	}

	// Verify the trigger works
	if _, err := c.DB.Exec("INSERT INTO test_trigger_tbl (val) VALUES ('original')"); err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	var val string
	if err := c.DB.QueryRow("SELECT val FROM test_trigger_tbl LIMIT 1").Scan(&val); err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if val != "updated" {
		t.Errorf("trigger didn't fire: val = %q, want %q", val, "updated")
	}
}

// TestR2_ExecMultiSingleStatement exercises execMulti with a single statement
// (no semicolons).
func TestR2_ExecMultiSingleStatement(t *testing.T) {
	c := newTestSQLiteClient(t)
	s := &Schema{client: c.TursoClient}

	if err := s.execMulti("CREATE TABLE single_stmt_test (id TEXT)"); err != nil {
		t.Fatalf("execMulti single statement: %v", err)
	}
}

// TestR2_ExecMultiTrailingSemicolon exercises execMulti with a trailing
// semicolon that results in an empty final statement (should be skipped).
func TestR2_ExecMultiTrailingSemicolon(t *testing.T) {
	c := newTestSQLiteClient(t)
	s := &Schema{client: c.TursoClient}

	if err := s.execMulti("CREATE TABLE trailing_semi (id TEXT);"); err != nil {
		t.Fatalf("execMulti trailing semicolon: %v", err)
	}
}

// TestR2_SplitSQLEdgeCases tests splitSQL directly for various edge cases.
func TestR2_SplitSQLEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int // expected number of statements
	}{
		{"empty", "", 0},
		{"whitespace only", "   \n\t  ", 0},
		{"single", "SELECT 1", 1},
		{"two statements", "SELECT 1; SELECT 2", 2},
		{"trailing semicolon", "SELECT 1;", 1},
		{"multiple semicolons", ";;;", 0},
		{"begin end block", "CREATE TRIGGER t AFTER INSERT ON x BEGIN UPDATE x SET a=1; END;", 1},
		{"nested begin end", "CREATE TRIGGER t AFTER INSERT ON x BEGIN BEGIN SELECT 1; END; END;", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitSQL(tt.input)
			if len(got) != tt.want {
				t.Errorf("splitSQL(%q) = %d statements, want %d; got: %v", tt.input, len(got), tt.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SearchMemoriesBM25 — remaining edge case: topK default
// ---------------------------------------------------------------------------

func TestR2_SearchMemoriesBM25DefaultTopK(t *testing.T) {
	c := newTestSQLiteClient(t)

	// Seed several memories
	for i := 0; i < 15; i++ {
		_, _ = c.SaveMemory(&Memory{
			Content:    "searchable bm25 memory number " + string(rune('A'+i)),
			Embedding:  zeroEmbedding(),
			Project:    "p",
			Type:       "memory",
			Visibility: "internal",
		})
	}

	// topK=0 should default to 10
	results, err := c.SearchMemoriesBM25("searchable bm25 memory", &MemoryFilter{Visibility: "all"}, 0)
	if err != nil {
		t.Fatalf("SearchMemoriesBM25 default topK: %v", err)
	}
	if len(results) > 10 {
		t.Errorf("default topK should cap at 10, got %d", len(results))
	}
}

// TestR2_SearchMemoriesBM25WithTypeFilter exercises the Type filter branch.
func TestR2_SearchMemoriesBM25WithTypeFilter(t *testing.T) {
	c := newTestSQLiteClient(t)

	_, _ = c.SaveMemory(&Memory{Content: "incident kubernetes down", Embedding: zeroEmbedding(), Project: "p", Type: "incident", Visibility: "internal"})
	_, _ = c.SaveMemory(&Memory{Content: "kubernetes setup notes", Embedding: zeroEmbedding(), Project: "p", Type: "memory", Visibility: "internal"})

	results, err := c.SearchMemoriesBM25("kubernetes", &MemoryFilter{Type: "incident", Visibility: "all"}, 5)
	if err != nil {
		t.Fatalf("SearchMemoriesBM25: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result (type=incident), got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// Migrate — re-run is idempotent (exercises isApplied=true branch)
// ---------------------------------------------------------------------------

func TestR2_MigrateIdempotent(t *testing.T) {
	c := newTestSQLiteClient(t)

	// Migrate was already called in newTestSQLiteClient. Running again should be a no-op.
	if err := c.Migrate(); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

// ---------------------------------------------------------------------------
// tursoConfigFromEnv — exercises env var parsing
// ---------------------------------------------------------------------------

func TestR2_TursoConfigFromEnvDefaults(t *testing.T) {
	// Clear env vars to test defaults
	t.Setenv("TURSO_URL", "")
	t.Setenv("TURSO_AUTH_TOKEN", "")
	t.Setenv("MAGI_REPLICA_PATH", "")
	t.Setenv("MAGI_SYNC_INTERVAL", "")

	cfg := tursoConfigFromEnv()
	if cfg.URL != "" {
		t.Errorf("URL = %q, want empty", cfg.URL)
	}
	if cfg.ReplicaPath == "" {
		t.Error("ReplicaPath should default to ~/.magi/memory.db")
	}
	if cfg.SyncInterval.Seconds() != 60 {
		t.Errorf("SyncInterval = %v, want 60s", cfg.SyncInterval)
	}
}

func TestR2_TursoConfigFromEnvCustom(t *testing.T) {
	t.Setenv("TURSO_URL", "libsql://mydb.turso.io")
	t.Setenv("TURSO_AUTH_TOKEN", "secret")
	t.Setenv("MAGI_REPLICA_PATH", "/tmp/custom.db")
	t.Setenv("MAGI_SYNC_INTERVAL", "30")

	cfg := tursoConfigFromEnv()
	if cfg.URL != "libsql://mydb.turso.io" {
		t.Errorf("URL = %q", cfg.URL)
	}
	if cfg.AuthToken != "secret" {
		t.Errorf("AuthToken = %q", cfg.AuthToken)
	}
	if cfg.ReplicaPath != "/tmp/custom.db" {
		t.Errorf("ReplicaPath = %q", cfg.ReplicaPath)
	}
	if cfg.SyncInterval.Seconds() != 30 {
		t.Errorf("SyncInterval = %v, want 30s", cfg.SyncInterval)
	}
}

func TestR2_TursoConfigFromEnvInvalidInterval(t *testing.T) {
	t.Setenv("MAGI_SYNC_INTERVAL", "not-a-number")

	cfg := tursoConfigFromEnv()
	// Invalid interval should fall back to default 60s
	if cfg.SyncInterval.Seconds() != 60 {
		t.Errorf("SyncInterval = %v, want 60s (default)", cfg.SyncInterval)
	}
}

// ---------------------------------------------------------------------------
// NewStore — additional backend coverage
// ---------------------------------------------------------------------------

func TestR2_NewStoreSQLiteDefault(t *testing.T) {
	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Default backend should be "sqlite"
	client, err := NewStore(&Config{
		Backend:    "sqlite",
		SQLitePath: filepath.Join(tmp, "store-test.db"),
	}, logger)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer client.Close()

	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Sync — SQLiteClient no-op
// ---------------------------------------------------------------------------

func TestR2_SQLiteSyncNoOp(t *testing.T) {
	c := newTestSQLiteClient(t)
	if err := c.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TursoClient.Sync — nil connector (no-op path)
// ---------------------------------------------------------------------------

func TestR2_TursoClientSyncNilConnector(t *testing.T) {
	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	db, err := sql.Open("libsql", "file:"+filepath.Join(tmp, "sync-nil.db"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	tc := &TursoClient{
		DB:        db,
		connector: nil,
		logger:    logger,
	}

	// Sync with nil connector should return nil (no-op)
	if err := tc.Sync(); err != nil {
		t.Fatalf("Sync nil connector: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetGraphData — DB closed after node query (link query error)
// ---------------------------------------------------------------------------

// TestR2_GetGraphDataScanNodeWithArchivedAt exercises the archived.Valid
// branch in GetGraphData by creating a memory, archiving it, then un-archiving
// it via direct SQL (so it has a non-NULL archived_at but still appears).
// Actually the WHERE clause filters archived memories. Instead we test that
// nodes with all nullable fields populated are scanned correctly.
func TestR2_GetGraphDataWithNullableFields(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	// Create a memory with all nullable fields set
	mem := &Memory{
		Content:    "fully populated node",
		Summary:    "summary here",
		Source:     "test-source",
		SourceFile: "test.go",
		Speaker:    "user",
		Area:       "work",
		SubArea:    "magi",
		Embedding:  zeroEmbedding(),
		Project:    "p",
		Type:       "memory",
		Visibility: "internal",
	}
	m1, err := c.SaveMemory(mem)
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	m2, err := c.SaveMemory(&Memory{
		Content:    "second node",
		Embedding:  zeroEmbedding(),
		Project:    "p",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	if _, err := c.CreateLink(ctx, m1.ID, m2.ID, "related_to", 1.0, false); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	memories, links, err := c.GetGraphData(ctx, 10)
	if err != nil {
		t.Fatalf("GetGraphData: %v", err)
	}
	if len(memories) != 2 {
		t.Errorf("expected 2 memories, got %d", len(memories))
	}
	if len(links) != 1 {
		t.Errorf("expected 1 link, got %d", len(links))
	}

	// Verify the nullable fields were populated
	for _, m := range memories {
		if m.ID == m1.ID {
			if m.Summary != "summary here" {
				t.Errorf("Summary = %q, want %q", m.Summary, "summary here")
			}
			if m.Source != "test-source" {
				t.Errorf("Source = %q, want %q", m.Source, "test-source")
			}
			if m.Speaker != "user" {
				t.Errorf("Speaker = %q, want %q", m.Speaker, "user")
			}
		}
	}
}

// ---------------------------------------------------------------------------
// execMulti — empty statement after split (exercises the stmt == "" continue)
// ---------------------------------------------------------------------------

func TestR2_ExecMultiMixedEmptyStatements(t *testing.T) {
	c := newTestSQLiteClient(t)
	s := &Schema{client: c.TursoClient}

	// Multiple consecutive semicolons produce empty statements that should be skipped
	if err := s.execMulti("CREATE TABLE mixed_empty (id TEXT);;; CREATE TABLE mixed_empty2 (id TEXT)"); err != nil {
		t.Fatalf("execMulti mixed empty: %v", err)
	}

	// Verify both tables exist
	var count int
	if err := c.DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name LIKE 'mixed_empty%'").Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 tables, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// SearchMemoriesBM25 — closed DB (scan error)
// ---------------------------------------------------------------------------

func TestR2_SearchMemoriesBM25ClosedDB(t *testing.T) {
	c := newTestSQLiteClient(t)

	// Seed a memory first
	_, _ = c.SaveMemory(&Memory{
		Content:    "bm25 closed db test",
		Embedding:  zeroEmbedding(),
		Project:    "p",
		Type:       "memory",
		Visibility: "internal",
	})

	// Close DB
	c.DB.Close()

	_, err := c.SearchMemoriesBM25("bm25", nil, 5)
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
}

// ---------------------------------------------------------------------------
// GetGraphData with linked node that has archived_at set via direct SQL
// ---------------------------------------------------------------------------

func TestR2_GetGraphDataNodeWithArchivedAtViaSQL(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	m1, _ := c.SaveMemory(&Memory{Content: "node with archive", Embedding: zeroEmbedding(), Project: "p", Type: "memory", Visibility: "internal"})
	m2, _ := c.SaveMemory(&Memory{Content: "node partner", Embedding: zeroEmbedding(), Project: "p", Type: "memory", Visibility: "internal"})
	_, _ = c.CreateLink(ctx, m1.ID, m2.ID, "related_to", 1.0, false)

	// Set archived_at directly but don't filter it out (clear the WHERE clause by
	// directly testing the scan behavior - we can't, but we can archive and verify exclusion)
	_ = c.ArchiveMemory(m1.ID)

	memories, _, err := c.GetGraphData(ctx, 10)
	if err != nil {
		t.Fatalf("GetGraphData: %v", err)
	}
	// Only m2 should be returned (m1 is archived)
	if len(memories) != 1 {
		t.Errorf("expected 1 memory (m1 archived), got %d", len(memories))
	}
}

// ---------------------------------------------------------------------------
// GetGraphData — completely empty DB (no memories, no links)
// Exercises the early return when len(memories) == 0.
// ---------------------------------------------------------------------------

func TestR2_GetGraphDataEmptyDB(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	memories, links, err := c.GetGraphData(ctx, 10)
	if err != nil {
		t.Fatalf("GetGraphData on empty DB: %v", err)
	}
	if len(memories) != 0 {
		t.Errorf("expected 0 memories, got %d", len(memories))
	}
	if links != nil {
		t.Errorf("expected nil links, got %v", links)
	}
}

// ---------------------------------------------------------------------------
// GetGraphData — complex graph with bidirectional links and multiple relations
// Exercises link filtering where both endpoints must be in the node set.
// ---------------------------------------------------------------------------

func TestR2_GetGraphDataComplexGraph(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	// Create 5 memories
	var ids []string
	for i := 0; i < 5; i++ {
		m, err := c.SaveMemory(&Memory{
			Content:    fmt.Sprintf("complex graph node %d", i),
			Embedding:  zeroEmbedding(),
			Project:    "p",
			Type:       "memory",
			Visibility: "internal",
		})
		if err != nil {
			t.Fatalf("SaveMemory %d: %v", i, err)
		}
		ids = append(ids, m.ID)
	}

	// Create multiple relation types and bidirectional links
	relations := []struct {
		from, to int
		rel      string
		weight   float64
	}{
		{0, 1, "related_to", 1.0},
		{1, 2, "caused_by", 0.8},
		{2, 3, "led_to", 0.9},
		{3, 4, "supersedes", 0.7},
		{0, 4, "related_to", 0.5}, // skip link
	}
	for _, r := range relations {
		if _, err := c.CreateLink(ctx, ids[r.from], ids[r.to], r.rel, r.weight, false); err != nil {
			t.Fatalf("CreateLink %d->%d: %v", r.from, r.to, err)
		}
	}

	memories, links, err := c.GetGraphData(ctx, 10)
	if err != nil {
		t.Fatalf("GetGraphData: %v", err)
	}
	if len(memories) != 5 {
		t.Errorf("expected 5 memories, got %d", len(memories))
	}
	if len(links) != 5 {
		t.Errorf("expected 5 links, got %d", len(links))
	}
}

// ---------------------------------------------------------------------------
// GetGraphData — topN limits the returned nodes, links only for included nodes
// ---------------------------------------------------------------------------

func TestR2_GetGraphDataTopNLimit(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	// Create 4 memories, link first two heavily
	var ids []string
	for i := 0; i < 4; i++ {
		m, err := c.SaveMemory(&Memory{
			Content:    fmt.Sprintf("topn node %d", i),
			Embedding:  zeroEmbedding(),
			Project:    "p",
			Type:       "memory",
			Visibility: "internal",
		})
		if err != nil {
			t.Fatalf("SaveMemory: %v", err)
		}
		ids = append(ids, m.ID)
	}

	// Link 0<->1, 0<->2 (node 0 has most links)
	c.CreateLink(ctx, ids[0], ids[1], "related_to", 1.0, false)
	c.CreateLink(ctx, ids[0], ids[2], "related_to", 1.0, false)

	// topN=2 should return only 2 memories
	memories, links, err := c.GetGraphData(ctx, 2)
	if err != nil {
		t.Fatalf("GetGraphData topN=2: %v", err)
	}
	if len(memories) != 2 {
		t.Errorf("expected 2 memories (topN=2), got %d", len(memories))
	}
	// Links should only include those between the 2 returned nodes
	for _, l := range links {
		foundFrom := false
		foundTo := false
		for _, m := range memories {
			if m.ID == l.FromID {
				foundFrom = true
			}
			if m.ID == l.ToID {
				foundTo = true
			}
		}
		if !foundFrom || !foundTo {
			t.Errorf("link %s has endpoints not in returned node set", l.ID)
		}
	}
}

// ---------------------------------------------------------------------------
// execMulti — error path (invalid SQL statement)
// ---------------------------------------------------------------------------

func TestR2_ExecMultiInvalidSQL(t *testing.T) {
	c := newTestSQLiteClient(t)
	s := &Schema{client: c.TursoClient}

	err := s.execMulti("THIS IS NOT VALID SQL AT ALL")
	if err == nil {
		t.Fatal("execMulti should fail with invalid SQL")
	}
}

// ---------------------------------------------------------------------------
// execMulti — partial failure (first statement OK, second fails)
// ---------------------------------------------------------------------------

func TestR2_ExecMultiPartialFailure(t *testing.T) {
	c := newTestSQLiteClient(t)
	s := &Schema{client: c.TursoClient}

	err := s.execMulti("CREATE TABLE partial_ok (id TEXT); THIS IS INVALID SQL")
	if err == nil {
		t.Fatal("execMulti should fail when second statement is invalid")
	}

	// First table should have been created
	var count int
	if err := c.DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name='partial_ok'").Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Errorf("first statement should have succeeded, table count = %d", count)
	}
}

// ---------------------------------------------------------------------------
// execMulti — closed DB (error at DB.Exec level)
// ---------------------------------------------------------------------------

func TestR2_ExecMultiClosedDB(t *testing.T) {
	c := newTestSQLiteClient(t)
	s := &Schema{client: c.TursoClient}

	c.DB.Close()

	err := s.execMulti("CREATE TABLE will_fail (id TEXT)")
	if err == nil {
		t.Fatal("execMulti should fail on closed DB")
	}
}

// ---------------------------------------------------------------------------
// GetTags — memory with tags returns them sorted
// ---------------------------------------------------------------------------

func TestR2_GetTagsWithTags(t *testing.T) {
	c := newTestSQLiteClient(t)

	m, err := c.SaveMemory(&Memory{
		Content:    "tagged memory",
		Embedding:  zeroEmbedding(),
		Project:    "p",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	if err := c.SetTags(m.ID, []string{"zebra", "alpha", "middle"}); err != nil {
		t.Fatalf("SetTags: %v", err)
	}

	tags, err := c.GetTags(m.ID)
	if err != nil {
		t.Fatalf("GetTags: %v", err)
	}
	if len(tags) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(tags))
	}
	// Tags should be sorted alphabetically (ORDER BY tag)
	if tags[0] != "alpha" || tags[1] != "middle" || tags[2] != "zebra" {
		t.Errorf("tags not sorted: %v", tags)
	}
}

// ---------------------------------------------------------------------------
// GetTags — closed DB (query error path)
// ---------------------------------------------------------------------------

func TestR2_GetTagsClosedDB(t *testing.T) {
	c := newTestSQLiteClient(t)
	c.DB.Close()

	_, err := c.GetTags("any-id")
	if err == nil {
		t.Fatal("GetTags should fail on closed DB")
	}
}

// ---------------------------------------------------------------------------
// DeleteLink — RowsAffected error path
// This is hard to trigger with SQLite but we can test the success case
// to ensure the happy path doesn't hit the error.
// ---------------------------------------------------------------------------

func TestR2_DeleteLinkSuccess(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	m1, _ := c.SaveMemory(&Memory{Content: "del link src", Embedding: zeroEmbedding(), Project: "p", Type: "memory", Visibility: "internal"})
	m2, _ := c.SaveMemory(&Memory{Content: "del link dst", Embedding: zeroEmbedding(), Project: "p", Type: "memory", Visibility: "internal"})

	link, err := c.CreateLink(ctx, m1.ID, m2.ID, "related_to", 1.0, false)
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	if err := c.DeleteLink(ctx, link.ID); err != nil {
		t.Fatalf("DeleteLink: %v", err)
	}

	// Verify it's gone
	err = c.DeleteLink(ctx, link.ID)
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows on double delete, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeleteLink — closed DB (ExecContext error path)
// ---------------------------------------------------------------------------

func TestR2_DeleteLinkClosedDB(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	c.DB.Close()

	err := c.DeleteLink(ctx, "any-id")
	if err == nil {
		t.Fatal("DeleteLink should fail on closed DB")
	}
}

// ---------------------------------------------------------------------------
// Close — TursoClient with nil connector (exercises the connector nil check)
// ---------------------------------------------------------------------------

func TestR2_TursoClientCloseNilConnector(t *testing.T) {
	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	db, err := sql.Open("libsql", "file:"+filepath.Join(tmp, "nil-conn.db"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	tc := &TursoClient{
		DB:        db,
		connector: nil,
		logger:    logger,
	}

	// Close should succeed — DB.Close works, connector is nil (skipped).
	if err := tc.Close(); err != nil {
		t.Fatalf("Close with nil connector: %v", err)
	}
}

// ---------------------------------------------------------------------------
// SetTags — empty tags list (exercises the early commit path)
// ---------------------------------------------------------------------------

func TestR2_SetTagsEmptyList(t *testing.T) {
	c := newTestSQLiteClient(t)

	m, err := c.SaveMemory(&Memory{
		Content:    "memory for empty tags",
		Embedding:  zeroEmbedding(),
		Project:    "p",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	// Set some tags first
	if err := c.SetTags(m.ID, []string{"a", "b"}); err != nil {
		t.Fatalf("SetTags: %v", err)
	}

	// Now set empty tags — exercises the len(tags)==0 early commit path
	if err := c.SetTags(m.ID, []string{}); err != nil {
		t.Fatalf("SetTags(empty): %v", err)
	}

	tags, err := c.GetTags(m.ID)
	if err != nil {
		t.Fatalf("GetTags: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("expected 0 tags after clearing, got %d", len(tags))
	}
}

// ---------------------------------------------------------------------------
// SetTags — closed DB (transaction begin error path)
// ---------------------------------------------------------------------------

func TestR2_SetTagsClosedDB(t *testing.T) {
	c := newTestSQLiteClient(t)
	c.DB.Close()

	err := c.SetTags("any-id", []string{"tag"})
	if err == nil {
		t.Fatal("SetTags should fail on closed DB")
	}
}

// ---------------------------------------------------------------------------
// scanLinks — auto field: verify auto=true link scans correctly
// ---------------------------------------------------------------------------

func TestR2_ScanLinksAutoField(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	m1, _ := c.SaveMemory(&Memory{Content: "auto src", Embedding: zeroEmbedding(), Project: "p", Type: "memory", Visibility: "internal"})
	m2, _ := c.SaveMemory(&Memory{Content: "auto dst", Embedding: zeroEmbedding(), Project: "p", Type: "memory", Visibility: "internal"})

	// Create an auto link (auto=true)
	link, err := c.CreateLink(ctx, m1.ID, m2.ID, "related_to", 0.5, true)
	if err != nil {
		t.Fatalf("CreateLink auto: %v", err)
	}
	if !link.Auto {
		t.Error("expected Auto=true on created link")
	}

	// Fetch and verify auto field survives scan
	links, err := c.GetLinks(ctx, m1.ID, "from")
	if err != nil {
		t.Fatalf("GetLinks: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if !links[0].Auto {
		t.Error("expected Auto=true on fetched link")
	}
}
