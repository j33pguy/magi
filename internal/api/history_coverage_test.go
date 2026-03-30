package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j33pguy/magi/internal/db"
)

// ---------- failingStore wraps a real db.Store but lets specific methods fail ----------

type failingStore struct {
	db.Store
	archiveErr error
	saveErr    error
}

func (f *failingStore) ArchiveMemory(id string) error {
	if f.archiveErr != nil {
		return f.archiveErr
	}
	return f.Store.ArchiveMemory(id)
}

func (f *failingStore) SaveMemory(m *db.Memory) (*db.Memory, error) {
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	return f.Store.SaveMemory(m)
}

// ---------- handleDeleteMemory: ArchiveMemory error (GetMemory succeeds) ----------

func TestHandleDeleteMemoryArchiveErrorIsolated(t *testing.T) {
	s := newTestServer(t)
	m := seedMemory(t, s, "will fail archive", "proj", "memory")

	// Wrap the real store: GetMemory works, ArchiveMemory fails.
	s.db = &failingStore{
		Store:      s.db,
		archiveErr: errors.New("disk I/O error"),
	}

	req := httptest.NewRequest("DELETE", "/memories/"+m.ID, nil)
	req.SetPathValue("id", m.ID)
	w := httptest.NewRecorder()
	s.handleDeleteMemory(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	errMsg, _ := resp["error"].(string)
	if !strings.Contains(errMsg, "archiving memory") {
		t.Errorf("expected 'archiving memory' in error, got %q", errMsg)
	}
}

// ---------- handleRemember: SaveMemory error (embed + dedup succeed) ----------

func TestHandleRememberSaveMemoryErrorIsolated(t *testing.T) {
	s := newTestServer(t)

	// Wrap so SaveMemory fails but embedding/dedup work.
	s.db = &failingStore{
		Store:   s.db,
		saveErr: errors.New("database write failed"),
	}

	body := `{"content": "this will fail to save", "project": "proj"}`
	req := httptest.NewRequest("POST", "/remember", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleRemember(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	errMsg, _ := resp["error"].(string)
	if !strings.Contains(errMsg, "saving memory") {
		t.Errorf("expected 'saving memory' in error, got %q", errMsg)
	}
}

// ---------- handleMemoryHistory: git Log error ----------

func TestHandleMemoryHistory_GitLogError(t *testing.T) {
	s, repo := newTestServerWithGit(t)
	m := seedAndCommit(t, s, repo)

	// Corrupt the git repo by removing the .git/objects directory.
	// This causes go-git's Log to fail when iterating commits.
	repoRoot := filepath.Dir(repo.MemoriesDir())
	objectsDir := filepath.Join(repoRoot, ".git", "objects")
	if err := os.RemoveAll(objectsDir); err != nil {
		t.Fatalf("removing objects dir: %v", err)
	}

	req := httptest.NewRequest("GET", "/memories/"+m.ID+"/history", nil)
	req.SetPathValue("id", m.ID)
	w := httptest.NewRecorder()
	s.handleMemoryHistory(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	errMsg, _ := resp["error"].(string)
	if !strings.Contains(errMsg, "fetching history") {
		t.Errorf("expected 'fetching history' in error, got %q", errMsg)
	}
}

// ---------- handleDeleteMemory: double-delete (idempotency check) ----------

func TestHandleDeleteMemoryTwice(t *testing.T) {
	s := newTestServer(t)
	m := seedMemory(t, s, "double delete target", "proj", "memory")

	// First delete should succeed.
	req := httptest.NewRequest("DELETE", "/memories/"+m.ID, nil)
	req.SetPathValue("id", m.ID)
	w := httptest.NewRecorder()
	s.handleDeleteMemory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("first delete: status = %d, want %d", w.Code, http.StatusOK)
	}

	// Second delete: GetMemory may still find the archived record or
	// might not, depending on GetMemory behavior with archived records.
	req2 := httptest.NewRequest("DELETE", "/memories/"+m.ID, nil)
	req2.SetPathValue("id", m.ID)
	w2 := httptest.NewRecorder()
	s.handleDeleteMemory(w2, req2)

	// Accept either OK (re-archive is idempotent) or NotFound.
	if w2.Code != http.StatusOK && w2.Code != http.StatusNotFound {
		t.Errorf("second delete: status = %d, want 200 or 404; body: %s", w2.Code, w2.Body.String())
	}
}

// ---------- handleMemoryDiff: memory exists in DB but not in git ----------

func TestHandleMemoryDiff_MemoryNotInGit(t *testing.T) {
	s, _ := newTestServerWithGit(t)

	// Seed a memory in the DB but don't commit it to git.
	m := seedMemory(t, s, "not in git", "proj", "memory")

	req := httptest.NewRequest("GET", "/memories/"+m.ID+"/diff?from=aaaa&to=bbbb", nil)
	req.SetPathValue("id", m.ID)
	w := httptest.NewRecorder()
	s.handleMemoryDiff(w, req)

	// Should 500 because the diff references nonexistent commits.
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------- handleCreateConversation: SaveMemory error (isolated) ----------

func TestHandleCreateConversationSaveErrorIsolated(t *testing.T) {
	s := newTestServer(t)

	s.db = &failingStore{
		Store:   s.db,
		saveErr: errors.New("db unavailable"),
	}

	body := `{"channel": "discord", "summary": "test discussion"}`
	req := httptest.NewRequest("POST", "/conversations", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleCreateConversation(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	errMsg, _ := resp["error"].(string)
	if !strings.Contains(errMsg, "saving conversation") {
		t.Errorf("expected 'saving conversation' in error, got %q", errMsg)
	}
}
