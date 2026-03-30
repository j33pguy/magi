package vcs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/j33pguy/magi/internal/db"
)

// mockEmbedder implements embeddings.Provider for tests.
type mockEmbedder struct{}

func (m *mockEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	emb := make([]float32, 384)
	if len(text) > 0 {
		emb[0] = float32(len(text)) / 100.0
	}
	return emb, nil
}

func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	var results [][]float32
	for _, t := range texts {
		e, _ := m.Embed(context.Background(), t)
		results = append(results, e)
	}
	return results, nil
}

func (m *mockEmbedder) Dimensions() int { return 384 }

// failingEmbedder always returns an error.
type failingEmbedder struct{}

func (m *failingEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, fmt.Errorf("embedding failed")
}
func (m *failingEmbedder) EmbedBatch(_ context.Context, _ []string) ([][]float32, error) {
	return nil, fmt.Errorf("embedding batch failed")
}
func (m *failingEmbedder) Dimensions() int { return 384 }

func newTestDB(t *testing.T) *db.SQLiteClient {
	t.Helper()
	logger := testLogger()
	client, err := db.NewSQLiteClient(filepath.Join(t.TempDir(), "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return client
}

func TestDBIsEmpty_EmptyDB(t *testing.T) {
	client := newTestDB(t)
	if !DBIsEmpty(client) {
		t.Error("DBIsEmpty should return true for empty database")
	}
}

func TestDBIsEmpty_NonEmptyDB(t *testing.T) {
	client := newTestDB(t)
	store := client

	emb := make([]float32, 384)
	_, err := store.SaveMemory(&db.Memory{
		Content:    "test",
		Embedding:  emb,
		Project:    "test",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	if DBIsEmpty(store) {
		t.Error("DBIsEmpty should return false when there are memories")
	}
}

func TestRebuildDB_EmptyDir(t *testing.T) {
	client := newTestDB(t)
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	logger := testLogger()
	err = RebuildDB(client, repo, &mockEmbedder{}, logger)
	if err != nil {
		t.Fatalf("RebuildDB on empty dir: %v", err)
	}

	if !DBIsEmpty(client) {
		t.Error("DB should still be empty after rebuilding from empty dir")
	}
}

func TestRebuildDB_WithMemories(t *testing.T) {
	client := newTestDB(t)
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Write some memory JSON files to the memories directory
	mem := &SerializableMemory{
		ID:         "rebuild-test-1",
		Content:    "rebuilt memory content",
		Project:    "test-project",
		Type:       "memory",
		Visibility: "internal",
		CreatedAt:  "2025-01-01T00:00:00Z",
		UpdatedAt:  "2025-01-01T00:00:00Z",
	}
	data, err := json.MarshalIndent(mem, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	memFile := filepath.Join(repo.MemoriesDir(), "rebuild-test-1.json")
	if err := os.WriteFile(memFile, data, 0o644); err != nil {
		t.Fatalf("writing test memory file: %v", err)
	}

	logger := testLogger()
	err = RebuildDB(client, repo, &mockEmbedder{}, logger)
	if err != nil {
		t.Fatalf("RebuildDB: %v", err)
	}

	if DBIsEmpty(client) {
		t.Error("DB should not be empty after rebuild")
	}
}

func TestRebuildDB_WithTags(t *testing.T) {
	client := newTestDB(t)
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	mem := &SerializableMemory{
		ID:         "tagged-mem-1",
		Content:    "tagged memory content",
		Project:    "test",
		Type:       "memory",
		Visibility: "internal",
		Tags:       []string{"important", "test"},
		CreatedAt:  "2025-01-01T00:00:00Z",
		UpdatedAt:  "2025-01-01T00:00:00Z",
	}
	data, err := json.MarshalIndent(mem, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	memFile := filepath.Join(repo.MemoriesDir(), "tagged-mem-1.json")
	if err := os.WriteFile(memFile, data, 0o644); err != nil {
		t.Fatalf("writing test memory file: %v", err)
	}

	logger := testLogger()
	err = RebuildDB(client, repo, &mockEmbedder{}, logger)
	if err != nil {
		t.Fatalf("RebuildDB: %v", err)
	}
}

func TestRebuildDB_SkipsBadJSON(t *testing.T) {
	client := newTestDB(t)
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Write an invalid JSON file
	badFile := filepath.Join(repo.MemoriesDir(), "bad.json")
	if err := os.WriteFile(badFile, []byte("not json"), 0o644); err != nil {
		t.Fatalf("writing bad file: %v", err)
	}

	// Write a valid memory file too
	mem := &SerializableMemory{
		ID:         "good-mem",
		Content:    "good memory",
		Project:    "test",
		Type:       "memory",
		Visibility: "internal",
		CreatedAt:  "2025-01-01T00:00:00Z",
		UpdatedAt:  "2025-01-01T00:00:00Z",
	}
	data, _ := json.MarshalIndent(mem, "", "  ")
	goodFile := filepath.Join(repo.MemoriesDir(), "good-mem.json")
	if err := os.WriteFile(goodFile, data, 0o644); err != nil {
		t.Fatalf("writing good file: %v", err)
	}

	logger := testLogger()
	err = RebuildDB(client, repo, &mockEmbedder{}, logger)
	if err != nil {
		t.Fatalf("RebuildDB should not fail on bad JSON: %v", err)
	}

	// The good memory should still have been imported
	if DBIsEmpty(client) {
		t.Error("DB should not be empty — good memory should have been imported")
	}
}

func TestRebuildDB_SkipsDirectories(t *testing.T) {
	client := newTestDB(t)
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Create a subdirectory in memories/
	subDir := filepath.Join(repo.MemoriesDir(), "subdir")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("creating subdir: %v", err)
	}

	// Create a non-.json file
	txtFile := filepath.Join(repo.MemoriesDir(), "readme.txt")
	if err := os.WriteFile(txtFile, []byte("not a memory"), 0o644); err != nil {
		t.Fatalf("writing txt file: %v", err)
	}

	logger := testLogger()
	err = RebuildDB(client, repo, &mockEmbedder{}, logger)
	if err != nil {
		t.Fatalf("RebuildDB: %v", err)
	}

	if !DBIsEmpty(client) {
		t.Error("DB should be empty — no valid memory files")
	}
}

func TestRebuildDB_FailingEmbedder(t *testing.T) {
	client := newTestDB(t)
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	mem := &SerializableMemory{
		ID:         "fail-embed-mem",
		Content:    "memory that will fail embedding",
		Project:    "test",
		Type:       "memory",
		Visibility: "internal",
		CreatedAt:  "2025-01-01T00:00:00Z",
		UpdatedAt:  "2025-01-01T00:00:00Z",
	}
	data, _ := json.MarshalIndent(mem, "", "  ")
	memFile := filepath.Join(repo.MemoriesDir(), "fail-embed-mem.json")
	if err := os.WriteFile(memFile, data, 0o644); err != nil {
		t.Fatalf("writing memory file: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	err = RebuildDB(client, repo, &failingEmbedder{}, logger)
	if err != nil {
		t.Fatalf("RebuildDB should not fail with failing embedder: %v", err)
	}

	// Memory should NOT have been saved because embedding failed
	if !DBIsEmpty(client) {
		t.Error("DB should be empty because embedder failed")
	}
}

func TestRebuildDB_BadMemoriesDir(t *testing.T) {
	client := newTestDB(t)
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Remove the memories directory to trigger an error
	os.RemoveAll(repo.MemoriesDir())

	logger := testLogger()
	err = RebuildDB(client, repo, &mockEmbedder{}, logger)
	if err == nil {
		t.Error("RebuildDB should fail when memories dir is missing")
	}
}
