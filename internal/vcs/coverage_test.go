package vcs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// simpleDiff edge cases
// ---------------------------------------------------------------------------

func TestSimpleDiff_Identical(t *testing.T) {
	got := simpleDiff("hello\nworld", "hello\nworld")
	if got != "(no changes)" {
		t.Errorf("simpleDiff identical: got %q, want %q", got, "(no changes)")
	}
}

func TestSimpleDiff_BothEmpty(t *testing.T) {
	got := simpleDiff("", "")
	if got != "(no changes)" {
		t.Errorf("simpleDiff both empty: got %q, want %q", got, "(no changes)")
	}
}

func TestSimpleDiff_OnlyRemovals(t *testing.T) {
	got := simpleDiff("alpha\nbeta\ngamma", "")
	if !strings.Contains(got, "-alpha") {
		t.Errorf("expected -alpha in diff, got:\n%s", got)
	}
	if !strings.Contains(got, "-beta") {
		t.Errorf("expected -beta in diff, got:\n%s", got)
	}
	if !strings.Contains(got, "-gamma") {
		t.Errorf("expected -gamma in diff, got:\n%s", got)
	}
	// The "to" side is a single empty string after split, so it appears as an addition.
	if strings.Contains(got, "+alpha") {
		t.Errorf("should not have +alpha in removal-only diff, got:\n%s", got)
	}
}

func TestSimpleDiff_OnlyAdditions(t *testing.T) {
	got := simpleDiff("", "alpha\nbeta\ngamma")
	if !strings.Contains(got, "+alpha") {
		t.Errorf("expected +alpha in diff, got:\n%s", got)
	}
	if !strings.Contains(got, "+beta") {
		t.Errorf("expected +beta in diff, got:\n%s", got)
	}
	if !strings.Contains(got, "+gamma") {
		t.Errorf("expected +gamma in diff, got:\n%s", got)
	}
}

func TestSimpleDiff_FromOnlyLines(t *testing.T) {
	// Lines in "from" that do NOT appear in "to" at all — exercises the else
	// branch in simpleDiff where containsLine returns false for from lines.
	from := "aaa\nbbb\nccc"
	to := "xxx\nyyy\nzzz"
	got := simpleDiff(from, to)

	for _, line := range []string{"-aaa", "-bbb", "-ccc"} {
		if !strings.Contains(got, line) {
			t.Errorf("expected %q in diff, got:\n%s", line, got)
		}
	}
	for _, line := range []string{"+xxx", "+yyy", "+zzz"} {
		if !strings.Contains(got, line) {
			t.Errorf("expected %q in diff, got:\n%s", line, got)
		}
	}
}

func TestSimpleDiff_HeaderPresent(t *testing.T) {
	got := simpleDiff("a", "b")
	if !strings.HasPrefix(got, "--- a (from)\n+++ b (to)\n") {
		t.Errorf("expected diff header, got:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// containsLine
// ---------------------------------------------------------------------------

func TestContainsLine_Found(t *testing.T) {
	if !containsLine([]string{"a", "b", "c"}, "b") {
		t.Error("containsLine should return true when target is present")
	}
}

func TestContainsLine_NotFound(t *testing.T) {
	if containsLine([]string{"a", "b", "c"}, "z") {
		t.Error("containsLine should return false when target is absent")
	}
}

func TestContainsLine_Empty(t *testing.T) {
	if containsLine(nil, "x") {
		t.Error("containsLine should return false for nil slice")
	}
	if containsLine([]string{}, "x") {
		t.Error("containsLine should return false for empty slice")
	}
}

// ---------------------------------------------------------------------------
// magiSignature
// ---------------------------------------------------------------------------

func TestMagiSignature(t *testing.T) {
	sig := magiSignature()
	if sig.Name != "MAGI" {
		t.Errorf("signature name = %q, want %q", sig.Name, "MAGI")
	}
	if sig.Email != "magi@localhost" {
		t.Errorf("signature email = %q, want %q", sig.Email, "magi@localhost")
	}
	if time.Since(sig.When) > 2*time.Second {
		t.Errorf("signature time too old: %v", sig.When)
	}
}

// ---------------------------------------------------------------------------
// Repo.Close — no-op when not in batch mode
// ---------------------------------------------------------------------------

func TestClose_NoBatch(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Close on non-batch repo should not panic or block.
	repo.Close()
	// Call again to verify idempotent safety.
	repo.Close()
}

// ---------------------------------------------------------------------------
// Repo.MemoriesDir
// ---------------------------------------------------------------------------

func TestMemoriesDir(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	want := filepath.Join(cfg.Path, "memories")
	if got := repo.MemoriesDir(); got != want {
		t.Errorf("MemoriesDir() = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// HasMemories — memories dir does not exist
// ---------------------------------------------------------------------------

func TestHasMemories_NoDirReturns_False(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Remove the memories directory entirely.
	if err := os.RemoveAll(filepath.Join(cfg.Path, "memories")); err != nil {
		t.Fatalf("removing memories dir: %v", err)
	}

	if repo.HasMemories() {
		t.Error("HasMemories should return false when memories dir is missing")
	}
}

func TestHasMemories_NonJSONIgnored(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Place a non-json file in memories.
	if err := os.WriteFile(filepath.Join(cfg.Path, "memories", "readme.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("writing txt file: %v", err)
	}

	if repo.HasMemories() {
		t.Error("HasMemories should return false when only non-.json files are present")
	}
}

// ---------------------------------------------------------------------------
// Init with batch mode config
// ---------------------------------------------------------------------------

func TestInit_BatchMode(t *testing.T) {
	cfg := tempConfig(t)
	cfg.CommitMode = "batch"
	cfg.BatchInterval = 50 * time.Millisecond

	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	if !repo.batchMode {
		t.Error("expected batchMode to be true")
	}
	if repo.stopBatch == nil {
		t.Error("stopBatch channel should be initialized")
	}
	if repo.batchDone == nil {
		t.Error("batchDone channel should be initialized")
	}

	repo.Close()
}

// ---------------------------------------------------------------------------
// Init with invalid path
// ---------------------------------------------------------------------------

func TestInit_BadPath(t *testing.T) {
	cfg := &Config{
		Enabled:    true,
		Path:       "/dev/null/not-a-directory",
		CommitMode: "immediate",
	}

	_, err := Init(cfg)
	if err == nil {
		t.Fatal("Init should fail with an invalid path")
	}
}

// ---------------------------------------------------------------------------
// RemoveAndCommit in batch mode (stages but does not commit immediately)
// ---------------------------------------------------------------------------

func TestRemoveAndCommit_BatchMode(t *testing.T) {
	cfg := tempConfig(t)
	cfg.CommitMode = "batch"
	cfg.BatchInterval = 10 * time.Second // long interval — we flush manually via Close

	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Write a file first (in batch mode, this stages only).
	relPath := filepath.Join("memories", "batch-del.json")
	if err := repo.WriteAndCommit(relPath, []byte(`{"id":"bd"}`+"\n"), "add"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	// Flush so the file is actually committed.
	repo.Close()

	// Re-open in batch mode.
	cfg2 := &Config{
		Enabled:       true,
		Path:          cfg.Path,
		CommitMode:    "batch",
		BatchInterval: 10 * time.Second,
	}
	repo2, err := Init(cfg2)
	if err != nil {
		t.Fatalf("Init 2: %v", err)
	}

	// Now remove — should stage but not commit.
	if err := repo2.RemoveAndCommit(relPath, "delete in batch"); err != nil {
		t.Fatalf("RemoveAndCommit: %v", err)
	}

	if !repo2.pending {
		t.Error("expected pending to be true after RemoveAndCommit in batch mode")
	}

	// File should already be deleted from the filesystem.
	if _, err := os.Stat(filepath.Join(cfg.Path, relPath)); !os.IsNotExist(err) {
		t.Error("file should have been removed from disk")
	}

	repo2.Close()
}

// ---------------------------------------------------------------------------
// WriteAndCommit in batch mode sets pending flag
// ---------------------------------------------------------------------------

func TestWriteAndCommit_BatchPending(t *testing.T) {
	cfg := tempConfig(t)
	cfg.CommitMode = "batch"
	cfg.BatchInterval = 10 * time.Second

	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	relPath := filepath.Join("memories", "pending.json")
	if err := repo.WriteAndCommit(relPath, []byte(`{}`+"\n"), "test"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	if !repo.pending {
		t.Error("expected pending = true after WriteAndCommit in batch mode")
	}

	repo.Close()
}

// ---------------------------------------------------------------------------
// Diff with bad commit hashes
// ---------------------------------------------------------------------------

func TestDiff_BadFromHash(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	relPath := filepath.Join("memories", "diff-err.json")
	if err := repo.WriteAndCommit(relPath, []byte(`{}`+"\n"), "v1"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	commits, err := repo.Log(relPath)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	_, err = repo.Diff(relPath, "0000000000000000000000000000000000000000", commits[0].Hash)
	if err == nil {
		t.Error("Diff should fail with a bad fromHash")
	}
}

func TestDiff_BadToHash(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	relPath := filepath.Join("memories", "diff-err2.json")
	if err := repo.WriteAndCommit(relPath, []byte(`{}`+"\n"), "v1"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	commits, err := repo.Log(relPath)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	_, err = repo.Diff(relPath, commits[0].Hash, "0000000000000000000000000000000000000000")
	if err == nil {
		t.Error("Diff should fail with a bad toHash")
	}
}

// ---------------------------------------------------------------------------
// fileAtCommit error paths
// ---------------------------------------------------------------------------

func TestFileAtCommit_BadHash(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	_, err = repo.fileAtCommit("memories/x.json", "badhash")
	if err == nil {
		t.Error("fileAtCommit should fail with an invalid hash")
	}
}

func TestFileAtCommit_MissingFile(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	// Write a file to get a valid commit hash.
	relPath := filepath.Join("memories", "exists.json")
	if err := repo.WriteAndCommit(relPath, []byte(`{}`+"\n"), "add"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	commits, err := repo.Log(relPath)
	if err != nil || len(commits) == 0 {
		t.Fatalf("Log: %v (commits=%d)", err, len(commits))
	}

	// Ask for a file that does not exist at that commit.
	_, err = repo.fileAtCommit("memories/no-such-file.json", commits[0].Hash)
	if err == nil {
		t.Error("fileAtCommit should fail when file is not in the commit tree")
	}
}

// ---------------------------------------------------------------------------
// flushBatch — no-op when nothing is pending
// ---------------------------------------------------------------------------

func TestFlushBatch_NoPending(t *testing.T) {
	cfg := tempConfig(t)
	cfg.CommitMode = "batch"
	cfg.BatchInterval = 10 * time.Second

	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// flushBatch with pending=false should be a safe no-op.
	repo.flushBatch()

	repo.Close()
}

// ---------------------------------------------------------------------------
// Log on a path with no commits
// ---------------------------------------------------------------------------

func TestLog_NoCommitsForPath(t *testing.T) {
	cfg := tempConfig(t)
	repo, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer repo.Close()

	commits, err := repo.Log("memories/nonexistent.json")
	if err != nil {
		t.Fatalf("Log should not error for a path with no commits: %v", err)
	}
	if len(commits) != 0 {
		t.Errorf("expected 0 commits, got %d", len(commits))
	}
}
