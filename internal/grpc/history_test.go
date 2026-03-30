package grpc

import (
	"path/filepath"
	"testing"

	"github.com/j33pguy/magi/internal/vcs"
)

func TestGRPC_SetGitRepo(t *testing.T) {
	srv, _ := newTestGRPCServer(t)

	if srv.gitRepo != nil {
		t.Error("gitRepo should be nil initially")
	}

	cfg := &vcs.Config{
		Enabled:    true,
		Path:       t.TempDir(),
		CommitMode: "immediate",
	}
	repo, err := vcs.Init(cfg)
	if err != nil {
		t.Fatalf("vcs.Init: %v", err)
	}
	defer repo.Close()

	srv.SetGitRepo(repo)
	if srv.gitRepo == nil {
		t.Error("gitRepo should be set after SetGitRepo")
	}
}

func TestGRPC_MemoryHistory_NoGit(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	// gitRepo is nil
	commits, err := srv.MemoryHistory("some-id")
	if err != nil {
		t.Fatalf("MemoryHistory should not error: %v", err)
	}
	if commits != nil {
		t.Error("expected nil when git is disabled")
	}
}

func TestGRPC_MemoryHistory_WithGit(t *testing.T) {
	srv, _ := newTestGRPCServer(t)

	cfg := &vcs.Config{
		Enabled:    true,
		Path:       t.TempDir(),
		CommitMode: "immediate",
	}
	repo, err := vcs.Init(cfg)
	if err != nil {
		t.Fatalf("vcs.Init: %v", err)
	}
	defer repo.Close()

	srv.SetGitRepo(repo)

	// Write a file so there's history
	relPath := filepath.Join("memories", "test-mem.json")
	if err := repo.WriteAndCommit(relPath, []byte(`{"id":"test-mem"}`+"\n"), "test commit"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}

	commits, err := srv.MemoryHistory("test-mem")
	if err != nil {
		t.Fatalf("MemoryHistory: %v", err)
	}
	if len(commits) < 1 {
		t.Error("expected at least 1 commit")
	}
}

func TestGRPC_MemoryDiff_NoGit(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	// gitRepo is nil
	diff, err := srv.MemoryDiff("some-id", "aaa", "bbb")
	if err != nil {
		t.Fatalf("MemoryDiff should not error: %v", err)
	}
	if diff != nil {
		t.Error("expected nil when git is disabled")
	}
}

func TestGRPC_MemoryDiff_WithGit(t *testing.T) {
	srv, _ := newTestGRPCServer(t)

	cfg := &vcs.Config{
		Enabled:    true,
		Path:       t.TempDir(),
		CommitMode: "immediate",
	}
	repo, err := vcs.Init(cfg)
	if err != nil {
		t.Fatalf("vcs.Init: %v", err)
	}
	defer repo.Close()

	srv.SetGitRepo(repo)

	relPath := filepath.Join("memories", "diff-test.json")
	if err := repo.WriteAndCommit(relPath, []byte(`{"v":1}`+"\n"), "v1"); err != nil {
		t.Fatalf("WriteAndCommit v1: %v", err)
	}
	if err := repo.WriteAndCommit(relPath, []byte(`{"v":2}`+"\n"), "v2"); err != nil {
		t.Fatalf("WriteAndCommit v2: %v", err)
	}

	commits, err := repo.Log(relPath)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(commits) < 2 {
		t.Fatalf("need 2 commits, got %d", len(commits))
	}

	diff, err := srv.MemoryDiff("diff-test", commits[1].Hash, commits[0].Hash)
	if err != nil {
		t.Fatalf("MemoryDiff: %v", err)
	}
	if diff == nil {
		t.Fatal("expected non-nil diff")
	}
	if diff.From == diff.To {
		t.Error("diff.From should differ from diff.To")
	}
}
