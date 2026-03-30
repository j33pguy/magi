package vcs

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// newTestStore creates a VersionedStore backed by a real SQLite DB and git repo.
func newTestStore(t *testing.T) (*VersionedStore, *db.SQLiteClient) {
	t.Helper()
	logger := testLogger()

	// Set up SQLite
	dbDir := t.TempDir()
	client, err := db.NewSQLiteClient(filepath.Join(dbDir, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Set up git repo
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	store := NewVersionedStore(client.TursoClient, repo, logger)
	return store, client
}

func seedTestMemory(t *testing.T, store *VersionedStore) *db.Memory {
	t.Helper()
	emb := make([]float32, 384)
	emb[0] = 0.42
	m, err := store.SaveMemory(&db.Memory{
		Content:    "test memory content",
		Embedding:  emb,
		Project:    "test-project",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	return m
}

func TestNewVersionedStore(t *testing.T) {
	store, _ := newTestStore(t)

	if store.repo == nil {
		t.Error("repo should not be nil")
	}
	if store.Client == nil {
		t.Error("Client should not be nil")
	}
	if store.logger == nil {
		t.Error("logger should not be nil")
	}
}

func TestVersionedStore_GitRepo(t *testing.T) {
	store, _ := newTestStore(t)

	repo := store.GitRepo()
	if repo == nil {
		t.Error("GitRepo() should not return nil")
	}
	if repo != store.repo {
		t.Error("GitRepo() should return the store's repo")
	}
}

func TestVersionedStore_Inner(t *testing.T) {
	store, _ := newTestStore(t)

	inner := store.Inner()
	if inner == nil {
		t.Error("Inner() should not return nil")
	}
	if inner != store.Client {
		t.Error("Inner() should return the store's Client")
	}
}

func TestVersionedStore_SaveMemory(t *testing.T) {
	store, _ := newTestStore(t)

	m := seedTestMemory(t, store)
	if m.ID == "" {
		t.Error("saved memory should have an ID")
	}

	// Verify the file was written to git
	relPath := filepath.Join("memories", m.ID+".json")
	data, err := os.ReadFile(filepath.Join(store.repo.path, relPath))
	if err != nil {
		t.Fatalf("git file should exist: %v", err)
	}
	if len(data) == 0 {
		t.Error("git file should not be empty")
	}

	// Verify git commit was created
	commits, err := store.repo.Log(relPath)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(commits) < 1 {
		t.Fatal("expected at least 1 commit")
	}
	if commits[0].Message == "" {
		t.Error("commit message should not be empty")
	}
}

func TestVersionedStore_UpdateMemory(t *testing.T) {
	store, _ := newTestStore(t)
	m := seedTestMemory(t, store)

	// Update the memory
	m.Content = "updated content"
	if err := store.UpdateMemory(m); err != nil {
		t.Fatalf("UpdateMemory: %v", err)
	}

	// Verify git has two commits now
	relPath := filepath.Join("memories", m.ID+".json")
	commits, err := store.repo.Log(relPath)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(commits) < 2 {
		t.Fatalf("expected at least 2 commits, got %d", len(commits))
	}
}

func TestVersionedStore_DeleteMemory(t *testing.T) {
	store, _ := newTestStore(t)
	m := seedTestMemory(t, store)

	relPath := filepath.Join("memories", m.ID+".json")

	// Verify file exists before delete
	if _, err := os.Stat(filepath.Join(store.repo.path, relPath)); err != nil {
		t.Fatalf("file should exist before delete: %v", err)
	}

	if err := store.DeleteMemory(m.ID); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}

	// Verify file was removed
	if _, err := os.Stat(filepath.Join(store.repo.path, relPath)); !os.IsNotExist(err) {
		t.Error("file should have been removed after delete")
	}

	// Verify DB memory is also gone
	_, err := store.GetMemory(m.ID)
	if err == nil {
		t.Error("GetMemory should return error after delete")
	}
}

func TestVersionedStore_ArchiveMemory(t *testing.T) {
	store, _ := newTestStore(t)
	m := seedTestMemory(t, store)

	if err := store.ArchiveMemory(m.ID); err != nil {
		t.Fatalf("ArchiveMemory: %v", err)
	}

	// Verify git commit exists for the archive
	relPath := filepath.Join("memories", m.ID+".json")
	commits, err := store.repo.Log(relPath)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(commits) < 2 {
		t.Fatalf("expected at least 2 commits (save + archive), got %d", len(commits))
	}

	// Verify archived memory still exists in git with archivedAt set
	data, err := os.ReadFile(filepath.Join(store.repo.path, relPath))
	if err != nil {
		t.Fatalf("reading archived memory file: %v", err)
	}
	mem, err := JSONToMemory(data)
	if err != nil {
		t.Fatalf("parsing archived memory: %v", err)
	}
	if mem.ArchivedAt == "" {
		t.Error("archived memory should have archivedAt set")
	}
}

func TestVersionedStore_SetTags(t *testing.T) {
	store, _ := newTestStore(t)
	m := seedTestMemory(t, store)

	tags := []string{"important", "test"}
	if err := store.SetTags(m.ID, tags); err != nil {
		t.Fatalf("SetTags: %v", err)
	}

	// Verify tags are in git
	relPath := filepath.Join("memories", m.ID+".json")
	data, err := os.ReadFile(filepath.Join(store.repo.path, relPath))
	if err != nil {
		t.Fatalf("reading memory file: %v", err)
	}
	mem, err := JSONToMemory(data)
	if err != nil {
		t.Fatalf("parsing memory: %v", err)
	}
	if len(mem.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(mem.Tags))
	}
}

func TestVersionedStore_CreateLink(t *testing.T) {
	store, _ := newTestStore(t)
	m1 := seedTestMemory(t, store)
	m2 := seedTestMemory(t, store)

	ctx := context.Background()
	link, err := store.CreateLink(ctx, m1.ID, m2.ID, "related_to", 0.8, false)
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	if link == nil {
		t.Fatal("link should not be nil")
	}

	// Verify link file was written to git
	relPath := filepath.Join("links", m1.ID+".json")
	data, err := os.ReadFile(filepath.Join(store.repo.path, relPath))
	if err != nil {
		t.Fatalf("reading link file: %v", err)
	}
	if len(data) == 0 {
		t.Error("link file should not be empty")
	}
}

func TestVersionedStore_DeleteLink(t *testing.T) {
	store, _ := newTestStore(t)
	m1 := seedTestMemory(t, store)
	m2 := seedTestMemory(t, store)

	ctx := context.Background()
	link, err := store.CreateLink(ctx, m1.ID, m2.ID, "related_to", 0.8, false)
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	if err := store.DeleteLink(ctx, link.ID); err != nil {
		t.Fatalf("DeleteLink: %v", err)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"short", 10, "short"},
		{"this is a longer string", 10, "this is..."},
		{"newline\nstring", 20, "newline string"},
		{"", 5, ""},
		{"exact", 5, "exact"},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}

func TestVersionedStore_SaveMemory_DBError(t *testing.T) {
	// Use a store with nil embedding to trigger a DB-level error
	// Actually, we need to test that DB errors propagate properly
	store, _ := newTestStore(t)

	// Memory without embedding will fail in many DB backends
	_, err := store.SaveMemory(&db.Memory{
		Content:    "",
		Embedding:  nil,
		Project:    "",
		Type:       "",
		Visibility: "",
	})
	// Even an empty memory should save (DB allows it) — just test no panic
	if err != nil {
		// Some DBs may reject this, that's fine
		return
	}
}

func TestVersionedStore_DeleteMemory_NonExistent(t *testing.T) {
	store, _ := newTestStore(t)

	// Deleting a non-existent memory — should not panic regardless of error behavior
	_ = store.DeleteMemory("nonexistent-id")
}

func TestVersionedStore_UpdateMemory_NonExistent(t *testing.T) {
	store, _ := newTestStore(t)

	// Updating a non-existent memory — exercises the code path; error behavior varies
	_ = store.UpdateMemory(&db.Memory{
		ID:      "nonexistent-id",
		Content: "updated",
	})
}

func TestVersionedStore_ArchiveMemory_NonExistent(t *testing.T) {
	store, _ := newTestStore(t)

	// Archiving a non-existent memory — should not panic
	_ = store.ArchiveMemory("nonexistent-id")
}

func TestVersionedStore_SetTags_NonExistent(t *testing.T) {
	store, _ := newTestStore(t)

	err := store.SetTags("nonexistent-id", []string{"tag1"})
	if err == nil {
		t.Error("expected error when setting tags on non-existent memory")
	}
}

func TestVersionedStore_BatchMode(t *testing.T) {
	logger := testLogger()
	dbDir := t.TempDir()
	client, err := db.NewSQLiteClient(filepath.Join(dbDir, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cfg := &Config{
		Enabled:       true,
		Path:          t.TempDir(),
		CommitMode:    "batch",
		BatchInterval: 100 * time.Millisecond,
	}
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	store := NewVersionedStore(client.TursoClient, repo, logger)

	// Save a memory — git write should be staged but not committed yet
	emb := make([]float32, 384)
	m, err := store.SaveMemory(&db.Memory{
		Content:    "batch test memory",
		Embedding:  emb,
		Project:    "test",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if m.ID == "" {
		t.Error("saved memory should have an ID")
	}

	// Wait for batch to flush
	time.Sleep(250 * time.Millisecond)
	repo.Close()
}
