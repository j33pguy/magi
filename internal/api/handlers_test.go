package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j33pguy/magi/internal/db"
)

// mockEmbedder implements embeddings.Provider for tests.
type mockEmbedder struct{}

func (m *mockEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	emb := make([]float32, 384)
	// Use text length to create a somewhat distinct embedding
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

// newTestServer creates a Server with a real SQLite DB and mock embedder.
func newTestServer(t *testing.T) *Server {
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

	s := &Server{
		db:       client.TursoClient,
		embedder: &mockEmbedder{},
		logger:   logger,
		token:    "", // no auth in tests
	}
	return s
}

// seedMemory inserts a test memory and returns it.
func seedMemory(t *testing.T, s *Server, content, project, memType string) *db.Memory {
	t.Helper()
	emb, _ := s.embedder.Embed(context.Background(), content)
	m, err := s.db.SaveMemory(&db.Memory{
		Content:    content,
		Embedding:  emb,
		Project:    project,
		Type:       memType,
		Visibility: "internal",
		Speaker:    "alice",
	})
	if err != nil {
		t.Fatalf("seedMemory: %v", err)
	}
	return m
}

// ---------- Health ----------

func TestHandleHealth(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}
	if resp["version"] != "0.3.0" {
		t.Errorf("version = %v, want 0.3.0", resp["version"])
	}
	if resp["db_status"] != "ok" {
		t.Errorf("db_status = %v, want ok", resp["db_status"])
	}
	if resp["uptime"] == nil || resp["uptime"] == "" {
		t.Error("expected non-empty uptime")
	}
}

func TestHandleReadyz(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	s.handleReadyz(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ready"] != true {
		t.Errorf("ready = %v, want true", resp["ready"])
	}
}

func TestHandleLivez(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("GET", "/livez", nil)
	w := httptest.NewRecorder()
	s.handleLivez(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["alive"] != true {
		t.Errorf("alive = %v, want true", resp["alive"])
	}
}

// ---------- Remember ----------

func TestHandleRemember(t *testing.T) {
	s := newTestServer(t)

	body := `{"content": "test memory content", "project": "test-proj", "type": "memory"}`
	req := httptest.NewRequest("POST", "/remember", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleRemember(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Error("expected non-empty id")
	}
}

func TestHandleRememberMissingContent(t *testing.T) {
	s := newTestServer(t)

	body := `{"project": "test-proj"}`
	req := httptest.NewRequest("POST", "/remember", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRemember(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleRememberInvalidJSON(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("POST", "/remember", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	s.handleRemember(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleRememberWithTags(t *testing.T) {
	s := newTestServer(t)

	body := `{"content": "tagged memory", "project": "proj", "tags": ["alpha", "beta"]}`
	req := httptest.NewRequest("POST", "/remember", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRemember(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	id := resp["id"].(string)

	tags, err := s.db.GetTags(id)
	if err != nil {
		t.Fatalf("GetTags: %v", err)
	}
	if len(tags) != 2 {
		t.Errorf("got %d tags, want 2", len(tags))
	}
}

func TestHandleRememberDefaults(t *testing.T) {
	s := newTestServer(t)

	body := `{"content": "minimal memory"}`
	req := httptest.NewRequest("POST", "/remember", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRemember(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	id := resp["id"].(string)

	got, _ := s.db.GetMemory(id)
	if got.Type != "memory" {
		t.Errorf("default type = %q, want %q", got.Type, "memory")
	}
	if got.Source != "api" {
		t.Errorf("default source = %q, want %q", got.Source, "api")
	}
	// Default speaker is set by handleRemember
	if got.Speaker == "" {
		t.Error("expected non-empty default speaker")
	}
}

// ---------- List Memories ----------

func TestHandleListMemories(t *testing.T) {
	s := newTestServer(t)
	seedMemory(t, s, "list test 1", "proj", "memory")
	seedMemory(t, s, "list test 2", "proj", "memory")

	req := httptest.NewRequest("GET", "/memories?project=proj", nil)
	w := httptest.NewRecorder()
	s.handleListMemories(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var memories []*db.Memory
	json.NewDecoder(w.Body).Decode(&memories)
	if len(memories) != 2 {
		t.Errorf("got %d memories, want 2", len(memories))
	}
}

func TestHandleListMemoriesWithLimit(t *testing.T) {
	s := newTestServer(t)
	for i := 0; i < 5; i++ {
		seedMemory(t, s, "limit test", "proj", "memory")
	}

	req := httptest.NewRequest("GET", "/memories?project=proj&limit=2", nil)
	w := httptest.NewRecorder()
	s.handleListMemories(w, req)

	var memories []*db.Memory
	json.NewDecoder(w.Body).Decode(&memories)
	if len(memories) != 2 {
		t.Errorf("got %d memories, want 2", len(memories))
	}
}

func TestHandleListMemoriesEmpty(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("GET", "/memories?project=nonexistent", nil)
	w := httptest.NewRecorder()
	s.handleListMemories(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

// ---------- Delete Memory ----------

func TestHandleDeleteMemory(t *testing.T) {
	s := newTestServer(t)
	m := seedMemory(t, s, "to delete", "proj", "memory")

	req := httptest.NewRequest("DELETE", "/memories/"+m.ID, nil)
	req.SetPathValue("id", m.ID)
	w := httptest.NewRecorder()
	s.handleDeleteMemory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}
}

func TestHandleDeleteMemoryNotFound(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("DELETE", "/memories/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()
	s.handleDeleteMemory(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleDeleteMemoryEmptyID(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("DELETE", "/memories/", nil)
	req.SetPathValue("id", "")
	w := httptest.NewRecorder()
	s.handleDeleteMemory(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ---------- Recall ----------

func TestHandleRecall(t *testing.T) {
	s := newTestServer(t)
	seedMemory(t, s, "kubernetes cluster backup strategy for infrastructure", "proj", "memory")

	body := `{"query": "kubernetes backup", "top_k": 5}`
	req := httptest.NewRequest("POST", "/recall", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRecall(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHandleRecallMissingQuery(t *testing.T) {
	s := newTestServer(t)

	body := `{"top_k": 5}`
	req := httptest.NewRequest("POST", "/recall", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleRecall(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleRecallInvalidJSON(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("POST", "/recall", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	s.handleRecall(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ---------- Search ----------

func TestHandleSearch(t *testing.T) {
	s := newTestServer(t)
	seedMemory(t, s, "searchable content about compute-cluster", "proj", "memory")

	req := httptest.NewRequest("GET", "/search?q=compute-cluster&top_k=5", nil)
	w := httptest.NewRecorder()
	s.handleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHandleSearchMissingQuery(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("GET", "/search", nil)
	w := httptest.NewRecorder()
	s.handleSearch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ---------- Conversations ----------

func TestHandleCreateConversation(t *testing.T) {
	s := newTestServer(t)

	body := `{"channel": "discord", "summary": "Discussed infrastructure rebuild", "topics": ["infrastructure"]}`
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
}

func TestHandleCreateConversationMissingFields(t *testing.T) {
	s := newTestServer(t)

	tests := []struct {
		name string
		body string
	}{
		{"missing summary", `{"channel": "discord"}`},
		{"missing channel", `{"summary": "test"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/conversations", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			s.handleCreateConversation(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestHandleListConversations(t *testing.T) {
	s := newTestServer(t)

	// Seed a conversation
	emb, _ := s.embedder.Embed(context.Background(), "conv content")
	m, _ := s.db.SaveMemory(&db.Memory{
		Content:    "Conversation on discord\n\nTest summary",
		Summary:    "Test summary",
		Embedding:  emb,
		Type:       "conversation",
		Visibility: "private",
		Source:     "discord",
	})
	_ = s.db.SetTags(m.ID, []string{"conversation", "channel:discord"})

	req := httptest.NewRequest("GET", "/conversations?channel=discord", nil)
	w := httptest.NewRecorder()
	s.handleListConversations(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHandleGetConversation(t *testing.T) {
	s := newTestServer(t)

	emb, _ := s.embedder.Embed(context.Background(), "conv")
	m, _ := s.db.SaveMemory(&db.Memory{
		Content:    "Conversation content",
		Embedding:  emb,
		Type:       "conversation",
		Visibility: "private",
	})

	req := httptest.NewRequest("GET", "/conversations/"+m.ID, nil)
	req.SetPathValue("id", m.ID)
	w := httptest.NewRecorder()
	s.handleGetConversation(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHandleGetConversationNotConversation(t *testing.T) {
	s := newTestServer(t)
	m := seedMemory(t, s, "not a conversation", "proj", "memory")

	req := httptest.NewRequest("GET", "/conversations/"+m.ID, nil)
	req.SetPathValue("id", m.ID)
	w := httptest.NewRecorder()
	s.handleGetConversation(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleGetConversationNotFound(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest("GET", "/conversations/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()
	s.handleGetConversation(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleSearchConversations(t *testing.T) {
	s := newTestServer(t)

	emb, _ := s.embedder.Embed(context.Background(), "conv search")
	m, _ := s.db.SaveMemory(&db.Memory{
		Content:    "Conversation about infrastructure rebuild",
		Summary:    "Infrastructure rebuild",
		Embedding:  emb,
		Type:       "conversation",
		Visibility: "private",
	})
	_ = s.db.SetTags(m.ID, []string{"conversation"})

	body := `{"query": "infrastructure rebuild"}`
	req := httptest.NewRequest("POST", "/conversations/search", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSearchConversations(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHandleSearchConversationsMissingQuery(t *testing.T) {
	s := newTestServer(t)

	body := `{"limit": 5}`
	req := httptest.NewRequest("POST", "/conversations/search", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSearchConversations(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ---------- Auth Middleware ----------

func TestRequireAuthNoToken(t *testing.T) {
	s := newTestServer(t)
	// Token is empty, so auth should be skipped
	handler := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("no token: status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRequireAuthValidToken(t *testing.T) {
	s := newTestServer(t)
	s.token = "test-secret"

	handler := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("valid token: status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRequireAuthInvalidToken(t *testing.T) {
	s := newTestServer(t)
	s.token = "test-secret"

	handler := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("invalid token: status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuthMissingBearer(t *testing.T) {
	s := newTestServer(t)
	s.token = "test-secret"

	handler := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic test-secret")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("missing bearer: status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ---------- writeJSON ----------

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusCreated, map[string]string{"hello": "world"})

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["hello"] != "world" {
		t.Errorf("body = %v", resp)
	}
}
