package db

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// zeroEmbedding returns a 384-dimensional zero vector for tests.
// The vector index requires exactly 384 dimensions on insert.
func zeroEmbedding() []float32 {
	return make([]float32, 384)
}

// newTestSQLiteClient creates a SQLiteClient backed by a temp file.
func newTestSQLiteClient(t *testing.T) *SQLiteClient {
	t.Helper()
	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	client, err := NewSQLiteClient(filepath.Join(tmp, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return client
}

func TestSQLiteMigrate(t *testing.T) {
	_ = newTestSQLiteClient(t)
}

func TestSQLiteSaveAndGet(t *testing.T) {
	c := newTestSQLiteClient(t)

	m := &Memory{
		Content:    "test memory content",
		Embedding:  zeroEmbedding(),
		Project:    "test-project",
		Type:       "memory",
		Visibility: "internal",
		Speaker:    "alice",
		Area:       "work",
		SubArea:    "testing",
	}

	saved, err := c.SaveMemory(m)
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if saved.ID == "" {
		t.Fatal("SaveMemory returned empty ID")
	}

	got, err := c.GetMemory(saved.ID)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if got.Content != "test memory content" {
		t.Errorf("Content = %q, want %q", got.Content, "test memory content")
	}
	if got.Project != "test-project" {
		t.Errorf("Project = %q, want %q", got.Project, "test-project")
	}
	if got.Speaker != "alice" {
		t.Errorf("Speaker = %q, want %q", got.Speaker, "alice")
	}
	if got.Area != "work" {
		t.Errorf("Area = %q, want %q", got.Area, "work")
	}
}

func TestSQLiteListMemories(t *testing.T) {
	c := newTestSQLiteClient(t)

	for i := 0; i < 3; i++ {
		_, err := c.SaveMemory(&Memory{
			Content:    "memory " + string(rune('A'+i)),
			Embedding:  zeroEmbedding(),
			Project:    "proj",
			Type:       "memory",
			Visibility: "internal",
		})
		if err != nil {
			t.Fatalf("SaveMemory[%d]: %v", i, err)
		}
	}

	list, err := c.ListMemories(&MemoryFilter{Project: "proj", Visibility: "all"})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("ListMemories returned %d, want 3", len(list))
	}
}

func TestSQLiteTags(t *testing.T) {
	c := newTestSQLiteClient(t)

	saved, err := c.SaveMemory(&Memory{
		Content:    "tagged memory",
		Embedding:  zeroEmbedding(),
		Project:    "proj",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	if err := c.SetTags(saved.ID, []string{"alpha", "beta"}); err != nil {
		t.Fatalf("SetTags: %v", err)
	}

	tags, err := c.GetTags(saved.ID)
	if err != nil {
		t.Fatalf("GetTags: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("GetTags returned %d tags, want 2", len(tags))
	}
	if tags[0] != "alpha" || tags[1] != "beta" {
		t.Errorf("tags = %v, want [alpha beta]", tags)
	}
}

func TestSQLiteArchiveAndDelete(t *testing.T) {
	c := newTestSQLiteClient(t)

	saved, err := c.SaveMemory(&Memory{
		Content:    "to be archived",
		Embedding:  zeroEmbedding(),
		Project:    "proj",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	if err := c.ArchiveMemory(saved.ID); err != nil {
		t.Fatalf("ArchiveMemory: %v", err)
	}

	list, err := c.ListMemories(&MemoryFilter{Project: "proj", Visibility: "all"})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 memories after archive, got %d", len(list))
	}

	if err := c.DeleteMemory(saved.ID); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}
	_, err = c.GetMemory(saved.ID)
	if err == nil {
		t.Error("expected error after hard delete, got nil")
	}
}

func TestSQLiteLinks(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	m1, err := c.SaveMemory(&Memory{Content: "first", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "internal"})
	if err != nil {
		t.Fatalf("SaveMemory m1: %v", err)
	}
	m2, err := c.SaveMemory(&Memory{Content: "second", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "internal"})
	if err != nil {
		t.Fatalf("SaveMemory m2: %v", err)
	}

	link, err := c.CreateLink(ctx, m1.ID, m2.ID, "related_to", 1.0, false)
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	if link.ID == "" {
		t.Fatal("CreateLink returned empty ID")
	}

	links, err := c.GetLinks(ctx, m1.ID, "from")
	if err != nil {
		t.Fatalf("GetLinks: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].ToID != m2.ID {
		t.Errorf("link ToID = %q, want %q", links[0].ToID, m2.ID)
	}

	if err := c.DeleteLink(ctx, link.ID); err != nil {
		t.Fatalf("DeleteLink: %v", err)
	}
	links, _ = c.GetLinks(ctx, m1.ID, "both")
	if len(links) != 0 {
		t.Errorf("expected 0 links after delete, got %d", len(links))
	}
}

func TestSQLiteUpdate(t *testing.T) {
	c := newTestSQLiteClient(t)

	saved, err := c.SaveMemory(&Memory{
		Content:    "original",
		Embedding:  zeroEmbedding(),
		Project:    "proj",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	saved.Content = "updated"
	if err := c.UpdateMemory(saved); err != nil {
		t.Fatalf("UpdateMemory: %v", err)
	}

	got, err := c.GetMemory(saved.ID)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if got.Content != "updated" {
		t.Errorf("Content = %q, want %q", got.Content, "updated")
	}
}

func TestSQLiteSyncNoop(t *testing.T) {
	c := newTestSQLiteClient(t)
	if err := c.Sync(); err != nil {
		t.Fatalf("Sync should be no-op, got: %v", err)
	}
}

func TestSQLiteContextMemories(t *testing.T) {
	c := newTestSQLiteClient(t)

	_, err := c.SaveMemory(&Memory{
		Content:    "recent context",
		Embedding:  zeroEmbedding(),
		Project:    "proj",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	memories, err := c.GetContextMemories("proj", 10)
	if err != nil {
		t.Fatalf("GetContextMemories: %v", err)
	}
	if len(memories) != 1 {
		t.Errorf("expected 1 context memory, got %d", len(memories))
	}
}

func TestSQLiteVectorSearch(t *testing.T) {
	c := newTestSQLiteClient(t)

	emb := make([]float32, 384)
	emb[0] = 1.0 // non-zero first element for distinctiveness

	_, err := c.SaveMemory(&Memory{
		Content:    "searchable memory",
		Embedding:  emb,
		Project:    "proj",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	results, err := c.SearchMemories(emb, &MemoryFilter{Project: "proj", Visibility: "all"}, 5)
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 search result, got %d", len(results))
	}
	if results[0].Memory.Content != "searchable memory" {
		t.Errorf("Content = %q, want %q", results[0].Memory.Content, "searchable memory")
	}
}

func TestNewStoreFactory(t *testing.T) {
	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	client, err := NewStore(&Config{
		Backend:    "sqlite",
		SQLitePath: filepath.Join(tmp, "factory.db"),
	}, logger)
	if err != nil {
		t.Fatalf("NewStore(sqlite): %v", err)
	}
	defer client.Close()

	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate via factory: %v", err)
	}
}

func TestNewStoreFactoryUnknown(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	_, err := NewStore(&Config{Backend: "postgres"}, logger)
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}
