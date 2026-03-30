package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/vcs"
)

func newTestServerWithGit(t *testing.T) (*Server, *vcs.Repo) {
	t.Helper()
	s := newTestServer(t)

	cfg := &vcs.Config{
		Enabled:    true,
		Path:       t.TempDir(),
		CommitMode: "immediate",
	}
	repo, err := vcs.Init(cfg)
	if err != nil {
		t.Fatalf("vcs.Init: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	s.SetGitRepo(repo)
	return s, repo
}

func seedAndCommit(t *testing.T, s *Server, repo *vcs.Repo) *db.Memory {
	t.Helper()
	emb, _ := s.embedder.Embed(context.Background(), "git test memory")
	m, err := s.db.SaveMemory(&db.Memory{
		Content:    "git test memory",
		Embedding:  emb,
		Project:    "test",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	// Write to git
	data, err := vcs.MemoryToJSON(m)
	if err != nil {
		t.Fatalf("MemoryToJSON: %v", err)
	}
	relPath := filepath.Join("memories", m.ID+".json")
	if err := repo.WriteAndCommit(relPath, data, "test: add memory"); err != nil {
		t.Fatalf("WriteAndCommit: %v", err)
	}
	return m
}

func TestSetGitRepo(t *testing.T) {
	s := newTestServer(t)
	if s.gitRepo != nil {
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

	s.SetGitRepo(repo)
	if s.gitRepo == nil {
		t.Error("gitRepo should be set after SetGitRepo")
	}
}

func TestHandleMemoryHistory_NoGit(t *testing.T) {
	s := newTestServer(t)
	// gitRepo is nil
	req := httptest.NewRequest("GET", "/memories/abc/history", nil)
	req.SetPathValue("id", "abc")
	w := httptest.NewRecorder()
	s.handleMemoryHistory(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleMemoryHistory_EmptyID(t *testing.T) {
	s, _ := newTestServerWithGit(t)
	req := httptest.NewRequest("GET", "/memories//history", nil)
	req.SetPathValue("id", "")
	w := httptest.NewRecorder()
	s.handleMemoryHistory(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleMemoryHistory_MemoryNotFound(t *testing.T) {
	s, _ := newTestServerWithGit(t)
	req := httptest.NewRequest("GET", "/memories/nonexistent/history", nil)
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()
	s.handleMemoryHistory(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleMemoryHistory_Success(t *testing.T) {
	s, repo := newTestServerWithGit(t)
	m := seedAndCommit(t, s, repo)

	req := httptest.NewRequest("GET", "/memories/"+m.ID+"/history", nil)
	req.SetPathValue("id", m.ID)
	w := httptest.NewRecorder()
	s.handleMemoryHistory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["id"] != m.ID {
		t.Errorf("response id = %v, want %v", resp["id"], m.ID)
	}
	entries, ok := resp["entries"].([]any)
	if !ok {
		t.Fatal("expected entries array in response")
	}
	if len(entries) < 1 {
		t.Error("expected at least 1 commit entry")
	}
}

func TestHandleMemoryDiff_NoGit(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/memories/abc/diff?from=aaa&to=bbb", nil)
	req.SetPathValue("id", "abc")
	w := httptest.NewRecorder()
	s.handleMemoryDiff(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleMemoryDiff_EmptyID(t *testing.T) {
	s, _ := newTestServerWithGit(t)
	req := httptest.NewRequest("GET", "/memories//diff?from=aaa&to=bbb", nil)
	req.SetPathValue("id", "")
	w := httptest.NewRecorder()
	s.handleMemoryDiff(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleMemoryDiff_MissingParams(t *testing.T) {
	s, _ := newTestServerWithGit(t)

	tests := []struct {
		name  string
		query string
	}{
		{"no params", ""},
		{"only from", "?from=aaa"},
		{"only to", "?to=bbb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/memories/abc/diff"+tt.query, nil)
			req.SetPathValue("id", "abc")
			w := httptest.NewRecorder()
			s.handleMemoryDiff(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}
		})
	}
}

func TestHandleMemoryDiff_Success(t *testing.T) {
	s, repo := newTestServerWithGit(t)
	m := seedAndCommit(t, s, repo)

	// Make a second commit
	m.Content = "updated content"
	data, _ := vcs.MemoryToJSON(m)
	relPath := filepath.Join("memories", m.ID+".json")
	if err := repo.WriteAndCommit(relPath, data, "test: update"); err != nil {
		t.Fatalf("WriteAndCommit v2: %v", err)
	}

	// Get commit hashes
	commits, err := repo.Log(relPath)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(commits) < 2 {
		t.Fatalf("need 2 commits, got %d", len(commits))
	}

	req := httptest.NewRequest("GET", "/memories/"+m.ID+"/diff?from="+commits[1].Hash+"&to="+commits[0].Hash, nil)
	req.SetPathValue("id", m.ID)
	w := httptest.NewRecorder()
	s.handleMemoryDiff(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["id"] != m.ID {
		t.Errorf("response id = %v, want %v", resp["id"], m.ID)
	}
	if resp["diff"] == nil || resp["diff"] == "" {
		t.Error("expected non-empty diff content")
	}
}

func TestHandleMemoryDiff_BadHashes(t *testing.T) {
	s, repo := newTestServerWithGit(t)
	m := seedAndCommit(t, s, repo)

	req := httptest.NewRequest("GET", "/memories/"+m.ID+"/diff?from=badhash1&to=badhash2", nil)
	req.SetPathValue("id", m.ID)
	w := httptest.NewRecorder()
	s.handleMemoryDiff(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}
