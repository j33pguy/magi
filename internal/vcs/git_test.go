package vcs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempConfig(t *testing.T) *Config {
	t.Helper()
	dir := t.TempDir()
	return &Config{
		Enabled:       true,
		Path:          dir,
		CommitMode:    "immediate",
		BatchInterval: time.Second,
	}
}

func TestInit(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Verify directory structure
	for _, dir := range []string{"memories", "links", ".magi-meta"} {
		if _, err := os.Stat(filepath.Join(cfg.Path, dir)); err != nil {
			t.Errorf("directory %s not created: %v", dir, err)
		}
	}

	// Verify version file
	data, err := os.ReadFile(filepath.Join(cfg.Path, ".magi-meta", "version"))
	if err != nil {
		t.Fatalf("reading version file: %v", err)
	}
	if string(data) != "1\n" {
		t.Errorf("version file = %q, want %q", string(data), "1\n")
	}
}

func TestInitIdempotent(t *testing.T) {
	cfg := tempConfig(t)

	repo1, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init 1: %v", err)
	}
	repo1.Close()

	// Second init should open the existing repo
	repo2, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init 2: %v", err)
	}
	repo2.Close()
}

func TestWriteAndCommit(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	relPath := filepath.Join("memories", "abc123.json")
	data := []byte(`{"id":"abc123","content":"hello"}` + "\n")

	if err := repo.WriteAndCommit(relPath, data, "test: add memory"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	// Verify file exists
	got, err := os.ReadFile(filepath.Join(cfg.Path, relPath))
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("file content = %q, want %q", string(got), string(data))
	}

	// Verify commit was created
	commits, err := repo.Log(relPath)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("got %d commits, want 1", len(commits))
	}
	if commits[0].Message != "test: add memory" {
		t.Errorf("commit message = %q, want %q", commits[0].Message, "test: add memory")
	}
}

func TestWriteUpdateAndLog(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	relPath := filepath.Join("memories", "def456.json")

	// First write
	if err := repo.WriteAndCommit(relPath, []byte(`{"v":1}`+"\n"), "v1"); err != nil {
		t.Fatalf("WriteAndCommit v1: %v", err)
	}

	// Second write (update)
	if err := repo.WriteAndCommit(relPath, []byte(`{"v":2}`+"\n"), "v2"); err != nil {
		t.Fatalf("WriteAndCommit v2: %v", err)
	}

	commits, err := repo.Log(relPath)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("got %d commits, want 2", len(commits))
	}
	// Most recent first
	if commits[0].Message != "v2" {
		t.Errorf("commits[0].Message = %q, want %q", commits[0].Message, "v2")
	}
	if commits[1].Message != "v1" {
		t.Errorf("commits[1].Message = %q, want %q", commits[1].Message, "v1")
	}
}

func TestDiff(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	relPath := filepath.Join("memories", "diff-test.json")

	if err := repo.WriteAndCommit(relPath, []byte(`{"content":"hello"}`+"\n"), "v1"); err != nil {
		t.Fatalf("WriteAndCommit v1: %v", err)
	}
	if err := repo.WriteAndCommit(relPath, []byte(`{"content":"world"}`+"\n"), "v2"); err != nil {
		t.Fatalf("WriteAndCommit v2: %v", err)
	}

	commits, err := repo.Log(relPath)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(commits) < 2 {
		t.Fatalf("need at least 2 commits, got %d", len(commits))
	}

	diff, err := repo.Diff(relPath, commits[1].Hash, commits[0].Hash)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	if diff.From == diff.To {
		t.Error("diff.From == diff.To, expected different content")
	}
	if diff.Content == "(no changes)" {
		t.Error("diff should show changes")
	}
}

func TestRemoveAndCommit(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	relPath := filepath.Join("memories", "to-delete.json")

	if err := repo.WriteAndCommit(relPath, []byte(`{"id":"del"}`+"\n"), "add"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}
	if err := repo.RemoveAndCommit(relPath, "delete"); err != nil {
		t.Fatalf("RemoveAndCommit: %v", err)
	}

	if _, err := os.Stat(filepath.Join(cfg.Path, relPath)); !os.IsNotExist(err) {
		t.Error("file should have been removed")
	}
}

func TestRemoveNonexistent(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Should not error on missing file
	if err := repo.RemoveAndCommit("memories/nope.json", "delete nope"); err != nil {
		t.Fatalf("RemoveAndCommit on nonexistent: %v", err)
	}
}

func TestHasMemories(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	if repo.HasMemories() {
		t.Error("HasMemories should return false for empty repo")
	}

	relPath := filepath.Join("memories", "test.json")
	if err := repo.WriteAndCommit(relPath, []byte(`{}`+"\n"), "add"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	if !repo.HasMemories() {
		t.Error("HasMemories should return true after adding a memory")
	}
}

func TestBatchMode(t *testing.T) {
	cfg := tempConfig(t)
	cfg.CommitMode = "batch"
	cfg.BatchInterval = 100 * time.Millisecond

	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	relPath := filepath.Join("memories", "batch.json")
	if err := repo.WriteAndCommit(relPath, []byte(`{"batch":true}`+"\n"), "batch write"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	// In batch mode, the commit happens later
	// Wait for the batch interval to pass
	time.Sleep(250 * time.Millisecond)

	// Close triggers a final flush
	repo.Close()

	// Verify commit was made
	repo2, err := Init(&Config{
		Enabled:    true,
		Path:       cfg.Path,
		CommitMode: "immediate",
	})
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	defer repo2.Close()

	commits, err := repo2.Log(relPath)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(commits) == 0 {
		t.Error("expected at least one commit after batch flush")
	}
}
