package db

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// NewSQLiteClient — MkdirAll failure (invalid path)
// ---------------------------------------------------------------------------

func TestExtra_NewSQLiteClientInvalidPath(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// /dev/null is a file, so creating a subdirectory under it should fail
	_, err := NewSQLiteClient("/dev/null/subdir/test.db", logger)
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
	// Should wrap the MkdirAll error
	if got := err.Error(); got == "" {
		t.Error("error message should not be empty")
	}
}

// NewSQLiteClient — path inside a read-only location (non-existent nested)
func TestExtra_NewSQLiteClientNestedInvalidPath(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// /proc/1/root is not writable for a normal user
	_, err := NewSQLiteClient("/proc/1/root/impossible/test.db", logger)
	if err == nil {
		t.Fatal("expected error for unwritable nested path, got nil")
	}
}

// ---------------------------------------------------------------------------
// GetTags — error path: query on closed database
// ---------------------------------------------------------------------------

func TestExtra_GetTagsClosedDB(t *testing.T) {
	c := newTestSQLiteClient(t)

	// Save a memory so we have a valid ID
	saved, err := c.SaveMemory(&Memory{
		Content:   "tags test",
		Embedding: zeroEmbedding(),
		Project:   "p",
		Type:      "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	// Close the underlying DB to trigger query error
	c.DB.Close()

	_, err = c.GetTags(saved.ID)
	if err == nil {
		t.Fatal("expected error querying closed DB, got nil")
	}
}

// ---------------------------------------------------------------------------
// DeleteLink — RowsAffected error path is hard to trigger directly,
// but we can test the sql.ErrNoRows path (0 rows affected) which is
// already tested. Instead, test DeleteLink with a closed DB to cover
// the ExecContext error path.
// ---------------------------------------------------------------------------

func TestExtra_DeleteLinkClosedDB(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	// Close DB to trigger exec error
	c.DB.Close()

	err := c.DeleteLink(ctx, "any-id")
	if err == nil {
		t.Fatal("expected error on closed DB, got nil")
	}
}

// ---------------------------------------------------------------------------
// GetGraphData — link filtering: links referencing nodes outside topN are excluded
// ---------------------------------------------------------------------------

func TestExtra_GetGraphDataLinkFiltering(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	// Create 3 memories with links
	m1, _ := c.SaveMemory(&Memory{Content: "node alpha", Embedding: zeroEmbedding(), Project: "p", Type: "memory", Visibility: "internal"})
	m2, _ := c.SaveMemory(&Memory{Content: "node beta", Embedding: zeroEmbedding(), Project: "p", Type: "memory", Visibility: "internal"})
	m3, _ := c.SaveMemory(&Memory{Content: "node gamma", Embedding: zeroEmbedding(), Project: "p", Type: "memory", Visibility: "internal"})

	// Create links: m1<->m2 and m2<->m3
	_, _ = c.CreateLink(ctx, m1.ID, m2.ID, "related_to", 1.0, false)
	_, _ = c.CreateLink(ctx, m2.ID, m3.ID, "related_to", 1.0, false)

	// Request topN=2: only 2 memories should be returned, and links
	// that reference the excluded node should be filtered out.
	memories, links, err := c.GetGraphData(ctx, 2)
	if err != nil {
		t.Fatalf("GetGraphData: %v", err)
	}

	if len(memories) != 2 {
		t.Errorf("expected 2 memories, got %d", len(memories))
	}

	// Build the set of returned node IDs
	nodeIDs := make(map[string]bool)
	for _, m := range memories {
		nodeIDs[m.ID] = true
	}

	// Every returned link must have both endpoints in the node set
	for _, l := range links {
		if !nodeIDs[l.FromID] || !nodeIDs[l.ToID] {
			t.Errorf("link %s has endpoints (%s, %s) but not all are in node set", l.ID, l.FromID, l.ToID)
		}
	}
}

// ---------------------------------------------------------------------------
// GetGraphData — archived memories excluded from graph
// ---------------------------------------------------------------------------

func TestExtra_GetGraphDataExcludesArchived(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	m1, _ := c.SaveMemory(&Memory{Content: "active node", Embedding: zeroEmbedding(), Project: "p", Type: "memory", Visibility: "internal"})
	m2, _ := c.SaveMemory(&Memory{Content: "archived node", Embedding: zeroEmbedding(), Project: "p", Type: "memory", Visibility: "internal"})

	_, _ = c.CreateLink(ctx, m1.ID, m2.ID, "related_to", 1.0, false)

	// Archive m2
	if err := c.ArchiveMemory(m2.ID); err != nil {
		t.Fatalf("ArchiveMemory: %v", err)
	}

	memories, links, err := c.GetGraphData(ctx, 10)
	if err != nil {
		t.Fatalf("GetGraphData: %v", err)
	}

	// Only the active memory should appear
	if len(memories) != 1 {
		t.Errorf("expected 1 memory (archived excluded), got %d", len(memories))
	}
	// The link references the archived node, so it should be filtered out
	if len(links) != 0 {
		t.Errorf("expected 0 links (archived endpoint), got %d", len(links))
	}
}

// ---------------------------------------------------------------------------
// SearchMemoriesBM25 — nil filter
// ---------------------------------------------------------------------------

func TestExtra_SearchMemoriesBM25NilFilter(t *testing.T) {
	c := newTestSQLiteClient(t)

	_, _ = c.SaveMemory(&Memory{
		Content:    "kubernetes pod scheduling",
		Embedding:  zeroEmbedding(),
		Project:    "p",
		Type:       "memory",
		Visibility: "internal",
	})

	results, err := c.SearchMemoriesBM25("kubernetes", nil, 5)
	if err != nil {
		t.Fatalf("SearchMemoriesBM25 nil filter: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// execMulti — error path: invalid SQL statement
// ---------------------------------------------------------------------------

func TestExtra_ExecMultiInvalidSQL(t *testing.T) {
	c := newTestSQLiteClient(t)
	s := &Schema{client: c.TursoClient}

	err := s.execMulti("CREATE TABLE good (id TEXT); THIS IS INVALID SQL; CREATE TABLE also_good (id TEXT)")
	if err == nil {
		t.Fatal("expected error for invalid SQL statement")
	}
}

// execMulti — empty/whitespace-only input should succeed
func TestExtra_ExecMultiEmptyStatements(t *testing.T) {
	c := newTestSQLiteClient(t)
	s := &Schema{client: c.TursoClient}

	// All empty statements should be skipped
	if err := s.execMulti("  ;  ;  "); err != nil {
		t.Fatalf("execMulti empty statements: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TursoClient.Close — nil connector path (SQLiteClient wraps TursoClient
// without a connector, so Close() only closes DB)
// ---------------------------------------------------------------------------

func TestExtra_TursoClientCloseNilConnector(t *testing.T) {
	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	db, err := sql.Open("libsql", "file:"+filepath.Join(tmp, "close-test.db"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	tc := &TursoClient{
		DB:        db,
		connector: nil, // no remote connector
		logger:    logger,
	}

	// Close should succeed — only closes DB, skips connector branch
	if err := tc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TursoClient.Close — verify DB is actually closed after Close()
// ---------------------------------------------------------------------------

func TestExtra_TursoClientCloseVerify(t *testing.T) {
	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	db, err := sql.Open("libsql", "file:"+filepath.Join(tmp, "close-verify.db"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	tc := &TursoClient{
		DB:        db,
		connector: nil,
		logger:    logger,
	}

	if err := tc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// After Close, Ping should fail
	if err := db.Ping(); err == nil {
		t.Error("expected Ping to fail after Close")
	}
}

// ---------------------------------------------------------------------------
// SQLiteClient.Close — happy path
// ---------------------------------------------------------------------------

func TestExtra_SQLiteClientClose(t *testing.T) {
	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	client, err := NewSQLiteClient(filepath.Join(tmp, "close-happy.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}

	// Close should succeed
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
