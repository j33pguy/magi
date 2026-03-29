package migrate

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j33pguy/magi/internal/chunking"
	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
)

// errEmbedder fails on Embed calls.
type errEmbedder struct{}

func (e *errEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, errors.New("embed failed")
}
func (e *errEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	return nil, errors.New("embed batch failed")
}
func (e *errEmbedder) Dimensions() int { return 384 }

var _ embeddings.Provider = (*errEmbedder)(nil)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

// TestImport_EmbeddingError covers the embedding error path (line 100-102).
func TestImport_EmbeddingError(t *testing.T) {
	c := newTestDB(t)
	imp := &MarkdownImporter{
		DB:       c,
		Embedder: &errEmbedder{},
		Splitter: chunking.NewSplitter(),
		Logger:   testLogger(),
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "err.md"), []byte("some content to embed"), 0644)

	err := imp.Import(context.Background(), dir, []FileMapping{
		{Filename: "err.md", Project: "proj", Type: "context"},
	})
	if err == nil {
		t.Fatal("expected error from embedder")
	}
	if !strings.Contains(err.Error(), "embedding chunk") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestImport_SaveMemoryError covers the save error path (line 120-122).
// We trigger this by closing the DB before import.
func TestImport_SaveMemoryError(t *testing.T) {
	c := newTestDB(t)
	imp := &MarkdownImporter{
		DB:       c,
		Embedder: &mockEmbedder{},
		Splitter: chunking.NewSplitter(),
		Logger:   testLogger(),
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "save.md"), []byte("content"), 0644)

	// Close the DB to cause SaveMemory to fail
	c.Close()

	err := imp.Import(context.Background(), dir, []FileMapping{
		{Filename: "save.md", Project: "proj", Type: "context"},
	})
	if err == nil {
		t.Fatal("expected error from closed DB")
	}
	if !strings.Contains(err.Error(), "saving chunk") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestImport_SetTagsError covers the SetTags error path (line 130-132).
// We import one chunk successfully, then close the DB so SetTags fails.
// Since SaveMemory and SetTags happen sequentially per chunk, we use a
// wrapper DB that fails only on SetTags.
func TestImport_SetTagsError(t *testing.T) {
	c := newTestDB(t)
	imp := &MarkdownImporter{
		DB:       c,
		Embedder: &mockEmbedder{},
		Splitter: chunking.NewSplitter(),
		Logger:   testLogger(),
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "tags.md"), []byte("content for tagging"), 0644)

	// First, do a successful import to confirm DB works
	err := imp.Import(context.Background(), dir, []FileMapping{
		{Filename: "tags.md", Project: "proj1", Type: "context", Tags: []string{"ok"}},
	})
	if err != nil {
		t.Fatalf("first import should succeed: %v", err)
	}

	// Now close the DB and try again with tags — SaveMemory will fail first,
	// but that's the save error path. To specifically hit SetTags error,
	// we need SaveMemory to succeed but SetTags to fail. Since both use
	// the same DB connection, closing it hits SaveMemory first.
	// Instead, let's use a file that imports, then manually test SetTags failure
	// by verifying that when the DB is in a bad state after save, tags fail.

	// Actually, the simplest way is to drop the tags table.
	_, execErr := c.DB.Exec("DROP TABLE IF EXISTS memory_tags")
	if execErr != nil {
		t.Fatalf("drop table: %v", execErr)
	}

	os.WriteFile(filepath.Join(dir, "tags2.md"), []byte("more content"), 0644)
	err = imp.Import(context.Background(), dir, []FileMapping{
		{Filename: "tags2.md", Project: "proj2", Type: "context", Tags: []string{"fail"}},
	})
	if err == nil {
		t.Fatal("expected error from SetTags with dropped table")
	}
	if !strings.Contains(err.Error(), "setting tags") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestImport_AllFilesMissing covers empty directory where all mapped files are missing.
func TestImport_AllFilesMissing(t *testing.T) {
	imp, _ := newImporter(t)
	dir := t.TempDir()

	mappings := []FileMapping{
		{Filename: "a.md", Project: "p", Type: "context"},
		{Filename: "b.md", Project: "p", Type: "context"},
		{Filename: "c.md", Project: "p", Type: "context"},
	}

	err := imp.Import(context.Background(), dir, mappings)
	if err != nil {
		t.Fatalf("all missing files should be skipped: %v", err)
	}
}

// TestImport_OnlyFrontmatter covers a file whose body is empty after stripping frontmatter.
func TestImport_OnlyFrontmatter(t *testing.T) {
	imp, c := newImporter(t)
	dir := t.TempDir()

	content := "---\ntitle: Only Frontmatter\ndate: 2026-01-01\n---\n"
	os.WriteFile(filepath.Join(dir, "empty_body.md"), []byte(content), 0644)

	err := imp.Import(context.Background(), dir, []FileMapping{
		{Filename: "empty_body.md", Project: "proj", Type: "context"},
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	// The splitter may produce zero or one chunk for empty/whitespace-only content.
	memories, err := c.ListMemories(&db.MemoryFilter{Project: "proj", Visibility: "all"})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	// Either no memories (empty string produces no chunks) or one empty chunk
	_ = memories
}

// TestImport_UnicodeContent verifies import works with unicode content.
func TestImport_UnicodeContent(t *testing.T) {
	imp, c := newImporter(t)
	dir := t.TempDir()

	content := "# Unicode Test\n日本語テスト\nEmoji: 🎉🚀\nAccénts: café résumé naïve"
	os.WriteFile(filepath.Join(dir, "unicode.md"), []byte(content), 0644)

	err := imp.Import(context.Background(), dir, []FileMapping{
		{Filename: "unicode.md", Project: "proj", Type: "context"},
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	memories, err := c.ListMemories(&db.MemoryFilter{Project: "proj", Visibility: "all"})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(memories) < 1 {
		t.Fatal("expected at least 1 memory")
	}
}

// TestImport_MultiChunkParentChaining verifies parent_id is set on subsequent chunks.
func TestImport_MultiChunkParentChaining(t *testing.T) {
	imp, c := newImporter(t)
	// Very small splitter to force many chunks
	imp.Splitter = &chunking.Splitter{MaxTokens: 8, Overlap: 1}

	dir := t.TempDir()
	// Build content large enough to produce 3+ chunks
	content := "# Part One\nThis is the first section with enough words.\n\n# Part Two\nThis is the second section also with many words.\n\n# Part Three\nAnd a third section with additional content here."
	os.WriteFile(filepath.Join(dir, "multi.md"), []byte(content), 0644)

	err := imp.Import(context.Background(), dir, []FileMapping{
		{Filename: "multi.md", Project: "chain", Type: "context", Tags: []string{"chained"}},
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	memories, err := c.ListMemories(&db.MemoryFilter{Project: "chain", Visibility: "all", Limit: 100})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(memories) < 3 {
		t.Fatalf("expected at least 3 chunks, got %d", len(memories))
	}

	// Find the first chunk (ChunkIndex == 0)
	var parentID string
	for _, m := range memories {
		if m.ChunkIndex == 0 {
			parentID = m.ID
			if m.ParentID != "" {
				t.Error("first chunk should have empty parent_id")
			}
			break
		}
	}
	if parentID == "" {
		t.Fatal("could not find chunk 0")
	}

	// Check subsequent chunks have parentID set
	for _, m := range memories {
		if m.ChunkIndex > 0 && m.ParentID != parentID {
			t.Errorf("chunk %d: parent_id = %q, want %q", m.ChunkIndex, m.ParentID, parentID)
		}
	}

	// Verify tags on all chunks
	for _, m := range memories {
		tags, err := c.GetTags(m.ID)
		if err != nil {
			t.Fatalf("GetTags(%s): %v", m.ID, err)
		}
		if len(tags) != 1 || tags[0] != "chained" {
			t.Errorf("chunk %d: tags = %v, want [chained]", m.ChunkIndex, tags)
		}
	}
}

// TestImport_ImportFileReadError covers the file-read error path (line 80-82)
// by passing a path to a directory instead of a file.
func TestImport_ImportFileReadError(t *testing.T) {
	imp, _ := newImporter(t)
	dir := t.TempDir()

	// Create a subdirectory with the same name as the mapping filename
	subdir := filepath.Join(dir, "isdir.md")
	os.Mkdir(subdir, 0755)

	err := imp.Import(context.Background(), dir, []FileMapping{
		{Filename: "isdir.md", Project: "proj", Type: "context"},
	})
	if err == nil {
		t.Fatal("expected error reading directory as file")
	}
	if !strings.Contains(err.Error(), "reading file") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestImport_ImportFileReturnedError covers the importFile error propagation (line 70-72).
func TestImport_ImportFileReturnedError(t *testing.T) {
	c := newTestDB(t)
	imp := &MarkdownImporter{
		DB:       c,
		Embedder: &errEmbedder{},
		Splitter: chunking.NewSplitter(),
		Logger:   testLogger(),
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "fail.md"), []byte("content"), 0644)

	err := imp.Import(context.Background(), dir, []FileMapping{
		{Filename: "fail.md", Project: "proj", Type: "context"},
	})
	if err == nil {
		t.Fatal("expected importFile error to propagate")
	}
	if !strings.Contains(err.Error(), "importing fail.md") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}

// TestDefaultMappings_AllFieldsNonEmpty validates every field in DefaultMappings.
func TestDefaultMappings_AllFieldsNonEmpty(t *testing.T) {
	mappings := DefaultMappings()
	if len(mappings) != 6 {
		t.Errorf("expected 6 default mappings, got %d", len(mappings))
	}
	for i, m := range mappings {
		if m.Filename == "" {
			t.Errorf("mapping[%d]: empty Filename", i)
		}
		if m.Project == "" {
			t.Errorf("mapping[%d]: empty Project", i)
		}
		if m.Type == "" {
			t.Errorf("mapping[%d]: empty Type", i)
		}
		if len(m.Tags) == 0 {
			t.Errorf("mapping[%d]: empty Tags", i)
		}
		for j, tag := range m.Tags {
			if tag == "" {
				t.Errorf("mapping[%d].Tags[%d]: empty tag", i, j)
			}
		}
	}
}

// TestStripFrontmatter_Additional covers edge cases for stripFrontmatter.
func TestStripFrontmatter_Additional(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty string", "", ""},
		{"only dashes", "---", "---"},
		{"just opening and closing", "------", ""},
		{"frontmatter with extra whitespace body", "---\nk: v\n---\n  \n  body  \n  ", "body"},
		{"multiple frontmatter blocks", "---\na: 1\n---\ntext\n---\nb: 2\n---\nmore", "text\n---\nb: 2\n---\nmore"},
		{"frontmatter no newline before close", "---\nkey: val\n---body", "body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripFrontmatter(tt.in)
			if got != tt.want {
				t.Errorf("stripFrontmatter(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestImport_NoTags covers the case where no tags are provided (skips SetTags).
func TestImport_NoTags(t *testing.T) {
	imp, c := newImporter(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "notags.md"), []byte("no tags content"), 0644)

	err := imp.Import(context.Background(), dir, []FileMapping{
		{Filename: "notags.md", Project: "proj", Type: "context"},
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	memories, err := c.ListMemories(&db.MemoryFilter{Project: "proj", Visibility: "all"})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(memories) < 1 {
		t.Fatal("expected at least 1 memory")
	}

	tags, err := c.GetTags(memories[0].ID)
	if err != nil {
		t.Fatalf("GetTags: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(tags))
	}
}

// TestImport_EmptyMappings covers calling Import with zero mappings.
func TestImport_EmptyMappings(t *testing.T) {
	imp, _ := newImporter(t)
	dir := t.TempDir()

	err := imp.Import(context.Background(), dir, []FileMapping{})
	if err != nil {
		t.Fatalf("Import with empty mappings should succeed: %v", err)
	}
}
