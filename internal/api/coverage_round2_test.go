package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/j33pguy/magi/internal/db"
)

// ---------- handleCreateConversation: SetTags error branch (line 83-88) ----------

func TestHandleCreateConversationSetTagsError(t *testing.T) {
	// Create a server, save a conversation, then break the DB so SetTags fails.
	// We need SaveMemory to succeed but SetTags to fail.
	// Strategy: seed the DB, then close the underlying connection between
	// SaveMemory and SetTags. Since we cannot intercept mid-request,
	// we use a wrapper approach: replace db with a wrapper that fails on SetTags.

	s := newTestServer(t)

	// Save a conversation successfully, then verify tag_warning path
	// by wrapping the db to fail on SetTags
	origDB := s.db
	wrapper := &setTagsFailDB{Store: origDB}
	s.db = wrapper

	body := `{"channel": "discord", "summary": "conv with broken tags", "topics": ["homelab"]}`
	req := httptest.NewRequest("POST", "/conversations", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleCreateConversation(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}
	// The tag_warning field should be present since SetTags failed
	if resp["tag_warning"] == nil {
		t.Error("expected tag_warning in response when SetTags fails")
	}
}

// ---------- handleListConversations: GetTags error continue (line 136-138) ----------

func TestHandleListConversationsGetTagsError(t *testing.T) {
	s := newTestServer(t)
	seedConversation(t, s, "tagged conv for error", "discord", nil)

	// Swap in a DB wrapper that fails on GetTags
	origDB := s.db
	wrapper := &getTagsFailDB{Store: origDB}
	s.db = wrapper

	req := httptest.NewRequest("GET", "/conversations", nil)
	w := httptest.NewRecorder()
	s.handleListConversations(w, req)

	// GetTags error is non-fatal — handler should still return 200
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

// ---------- handleListConversations: invalid since timestamp (line 155-156) ----------
// Already covered by TestHandleListConversationsInvalidSince in additional_test.go.

// ---------- handleListMemories: GetTags error continue (line 62-64) ----------

func TestHandleListMemoriesGetTagsError(t *testing.T) {
	s := newTestServer(t)
	seedMemory(t, s, "mem with broken tags", "proj", "memory")

	origDB := s.db
	wrapper := &getTagsFailDB{Store: origDB}
	s.db = wrapper

	req := httptest.NewRequest("GET", "/memories?project=proj", nil)
	w := httptest.NewRecorder()
	s.handleListMemories(w, req)

	// GetTags error is non-fatal — handler should still return 200
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

// ---------- handleRemember: dedup match branch (line 56-65) ----------

func TestHandleRememberDeduplicated(t *testing.T) {
	s := newTestServer(t)

	// Insert a memory with a known embedding
	content := "test dedup content for the api layer handler"
	emb, _ := s.embedder.Embed(context.Background(), content)
	_, err := s.db.SaveMemory(&db.Memory{
		Content:    content,
		Embedding:  emb,
		Project:    "proj",
		Type:       "memory",
		Visibility: "internal",
		Speaker:    "assistant",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Now try to remember the same content — should be deduplicated
	body := fmt.Sprintf(`{"content": %q, "project": "proj"}`, content)
	req := httptest.NewRequest("POST", "/remember", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRemember(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["deduplicated"] != true {
		t.Errorf("expected deduplicated=true, got %v", resp["deduplicated"])
	}
	if resp["note"] == nil {
		t.Error("expected 'note' field in dedup response")
	}
}

// ---------- handleRemember: SetTags error branch (line 95-101) ----------

func TestHandleRememberSetTagsError(t *testing.T) {
	s := newTestServer(t)

	origDB := s.db
	wrapper := &setTagsFailDB{Store: origDB}
	s.db = wrapper

	body := `{"content": "memory with failing tags", "project": "proj", "tags": ["alpha", "beta"]}`
	req := httptest.NewRequest("POST", "/remember", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRemember(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}
	// tag_warning should be set
	if resp["tag_warning"] == nil {
		t.Error("expected tag_warning when SetTags fails")
	}
}

// ---------- handleRemember: tag_warning response field (line 105-107) ----------
// This is tested implicitly by TestHandleRememberSetTagsError above.
// The tag_warning field is only set when tagErr != "", which occurs on SetTags failure.

// ---------- DB wrappers for targeted error injection ----------

// setTagsFailDB wraps a real db.Store, injecting errors on SetTags only.
type setTagsFailDB struct {
	db.Store
}

func (s *setTagsFailDB) SetTags(_ string, _ []string) error {
	return fmt.Errorf("simulated SetTags failure")
}

// getTagsFailDB wraps a real db.Store, injecting errors on GetTags only.
type getTagsFailDB struct {
	db.Store
}

func (g *getTagsFailDB) GetTags(_ string) ([]string, error) {
	return nil, fmt.Errorf("simulated GetTags failure")
}
