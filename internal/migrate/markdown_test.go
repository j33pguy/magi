package migrate

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/j33pguy/magi/internal/chunking"
	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
)

// mockEmbedder returns fixed 384-dim zero vectors.
type mockEmbedder struct{}

func (m *mockEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, 384), nil
}
func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = make([]float32, 384)
	}
	return result, nil
}
func (m *mockEmbedder) Dimensions() int { return 384 }

var _ embeddings.Provider = (*mockEmbedder)(nil)

func newTestDB(t *testing.T) *db.Client {
	t.Helper()
	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client, err := db.NewSQLiteClient(filepath.Join(tmp, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return client.TursoClient
}

func newImporter(t *testing.T) (*MarkdownImporter, *db.Client) {
	t.Helper()
	c := newTestDB(t)
	return &MarkdownImporter{
		DB:       c,
		Embedder: &mockEmbedder{},
		Splitter: chunking.NewSplitter(),
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	}, c
}

func TestDefaultMappings(t *testing.T) {
	mappings := DefaultMappings()
	if len(mappings) == 0 {
		t.Fatal("DefaultMappings returned empty")
	}
	for _, m := range mappings {
		if m.Filename == "" {
			t.Error("empty filename in mapping")
		}
		if m.Project == "" {
			t.Error("empty project in mapping")
		}
	}
}

func TestStripFrontmatter(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no frontmatter", "hello world", "hello world"},
		{"with frontmatter", "---\ntitle: test\n---\nbody text", "body text"},
		{"unclosed frontmatter", "---\ntitle: test\nbody text", "---\ntitle: test\nbody text"},
		{"empty body", "---\nx: y\n---\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripFrontmatter(tt.in)
			if got != tt.want {
				t.Errorf("stripFrontmatter = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestImport_SingleFile(t *testing.T) {
	imp, c := newImporter(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.md"), []byte("# Test\nHello world"), 0644)

	mappings := []FileMapping{
		{Filename: "test.md", Project: "proj", Type: "context", Tags: []string{"tag1"}},
	}

	err := imp.Import(context.Background(), dir, mappings)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	memories, err := c.ListMemories(&db.MemoryFilter{Project: "proj", Visibility: "all"})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(memories) < 1 {
		t.Fatal("expected at least 1 memory after import")
	}
}

func TestImport_WithFrontmatter(t *testing.T) {
	imp, c := newImporter(t)
	dir := t.TempDir()
	content := "---\ntitle: Test\n---\n# Body\nActual content"
	os.WriteFile(filepath.Join(dir, "fm.md"), []byte(content), 0644)

	mappings := []FileMapping{
		{Filename: "fm.md", Project: "proj", Type: "context"},
	}

	err := imp.Import(context.Background(), dir, mappings)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	memories, err := c.ListMemories(&db.MemoryFilter{Project: "proj", Visibility: "all"})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(memories) < 1 {
		t.Fatal("expected memory after frontmatter import")
	}
	// Frontmatter should be stripped
	if memories[0].Content == "" {
		t.Error("content should not be empty")
	}
}

func TestImport_MissingFile(t *testing.T) {
	imp, _ := newImporter(t)
	dir := t.TempDir()

	mappings := []FileMapping{
		{Filename: "nonexistent.md", Project: "proj", Type: "context"},
	}

	// Should skip missing files without error
	err := imp.Import(context.Background(), dir, mappings)
	if err != nil {
		t.Fatalf("Import should skip missing: %v", err)
	}
}

func TestImport_PathTraversal(t *testing.T) {
	imp, _ := newImporter(t)
	dir := t.TempDir()

	tests := []struct {
		name     string
		filename string
	}{
		{"slash", "../../etc/passwd"},
		{"backslash", "..\\etc\\passwd"},
		{"dotdot", "../secret.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := imp.Import(context.Background(), dir, []FileMapping{
				{Filename: tt.filename, Project: "proj", Type: "context"},
			})
			if err == nil {
				t.Error("expected error for path traversal")
			}
		})
	}
}

func TestImport_WithTags(t *testing.T) {
	imp, c := newImporter(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "tagged.md"), []byte("tagged content"), 0644)

	err := imp.Import(context.Background(), dir, []FileMapping{
		{Filename: "tagged.md", Project: "proj", Type: "context", Tags: []string{"alpha", "beta"}},
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	memories, err := c.ListMemories(&db.MemoryFilter{Project: "proj", Visibility: "all"})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(memories) < 1 {
		t.Fatal("expected memory")
	}

	tags, err := c.GetTags(memories[0].ID)
	if err != nil {
		t.Fatalf("GetTags: %v", err)
	}
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(tags))
	}
}

func TestImport_MultiChunk(t *testing.T) {
	imp, c := newImporter(t)
	// Use a small splitter to force multiple chunks
	imp.Splitter = &chunking.Splitter{MaxTokens: 10, Overlap: 2}

	dir := t.TempDir()
	// Create content large enough to be split (>40 chars per section)
	content := "# Section 1\n" + string(make([]byte, 60)) + "\n# Section 2\n" + string(make([]byte, 60))
	os.WriteFile(filepath.Join(dir, "big.md"), []byte(content), 0644)

	err := imp.Import(context.Background(), dir, []FileMapping{
		{Filename: "big.md", Project: "proj", Type: "context", Tags: []string{"multi"}},
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	memories, err := c.ListMemories(&db.MemoryFilter{Project: "proj", Visibility: "all", Limit: 100})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(memories) < 2 {
		t.Errorf("expected multiple chunks, got %d", len(memories))
	}
}
