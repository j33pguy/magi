package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/j33pguy/magi/internal/auth"
	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/pipeline"
	"github.com/j33pguy/magi/internal/secretstore"
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
		tasks:    client.TursoClient,
		embedder: &mockEmbedder{},
		logger:   logger,
		auth:     &auth.Resolver{}, // no auth in tests
	}
	if machines, ok := any(client.TursoClient).(MachineRegistryStore); ok {
		s.machines = machines
	}
	if lookup, ok := any(client.TursoClient).(auth.MachineLookup); ok {
		s.auth.SetMachineLookup(lookup)
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

func TestHandleRememberAsync(t *testing.T) {
	s := newTestServer(t)
	s.SetPipeline(pipeline.NewWriter(s.db, s.embedder, pipeline.Config{
		Enabled:       true,
		Workers:       2,
		QueueSize:     16,
		FlushInterval: 5 * time.Millisecond,
		BatchMaxSize:  4,
	}, s.logger))
	defer s.pipeline.Close()

	body := `{"content": "async memory content", "project": "test-proj", "type": "memory", "tags": ["alpha"]}`
	req := httptest.NewRequest("POST", "/remember", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleRemember(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["ok"] != true {
		t.Fatalf("ok = %v, want true", resp["ok"])
	}
	if resp["async"] != true {
		t.Fatalf("async = %v, want true", resp["async"])
	}
	id, _ := resp["id"].(string)
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status := s.pipeline.Status(id)
		if status != nil && status.State == pipeline.StateComplete {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	status := s.pipeline.Status(id)
	if status == nil {
		t.Fatal("expected async status")
	}
	if status.State != pipeline.StateComplete {
		t.Fatalf("state = %s, want %s", status.State, pipeline.StateComplete)
	}

	memories, err := s.db.ListMemories(&db.MemoryFilter{Project: "test-proj"})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	var found *db.Memory
	for _, m := range memories {
		if m.Content == "async memory content" {
			found = m
			break
		}
	}
	if found == nil {
		t.Fatal("expected async memory to be saved")
	}
	if found.Speaker != "assistant" {
		t.Fatalf("speaker = %q, want %q", found.Speaker, "assistant")
	}
	tags, err := s.db.GetTags(found.ID)
	if err != nil {
		t.Fatalf("GetTags: %v", err)
	}
	gotAlpha := false
	for _, tag := range tags {
		if tag == "alpha" {
			gotAlpha = true
			break
		}
	}
	if !gotAlpha {
		t.Fatalf("expected alpha tag, got %v", tags)
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
	tagSet := make(map[string]bool)
	for _, tag := range tags {
		tagSet[tag] = true
	}
	if !tagSet["alpha"] || !tagSet["beta"] {
		t.Errorf("missing expected tags: %v", tags)
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

func TestHandleRecallAcceptsLimitAlias(t *testing.T) {
	s := newTestServer(t)
	seedMemory(t, s, "API changes from v3 rollout", "proj", "decision")

	body := `{"query":"API changes","limit":1}`
	req := httptest.NewRequest("POST", "/recall", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleRecall(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Results []json.RawMessage `json:"results"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected exactly 1 result with limit alias, got %d", len(resp.Results))
	}
}

func TestTaskQueueLifecycle(t *testing.T) {
	s := newTestServer(t)
	memory := seedMemory(t, s, "worker found a deployment pitfall", "proj", "lesson")

	createBody := `{
		"title":"Build task queue",
		"project":"proj",
		"queue":"agents",
		"status":"queued",
		"priority":"high",
		"orchestrator":"claude-main",
		"worker":"codex-worker",
		"metadata":{"epic":"task-queue"}
	}`
	createReq := httptest.NewRequest("POST", "/tasks", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	s.handleCreateTask(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create task status = %d, want %d; body=%s", createW.Code, http.StatusCreated, createW.Body.String())
	}

	var created db.Task
	if err := json.NewDecoder(createW.Body).Decode(&created); err != nil {
		t.Fatalf("decode created task: %v", err)
	}
	if created.ID == "" || created.Status != db.TaskStatusQueued {
		t.Fatalf("unexpected created task: %+v", created)
	}

	updateBody := `{"status":"started","status_comment":"worker picked it up"}`
	updateReq := httptest.NewRequest("PATCH", "/tasks/"+created.ID, strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.SetPathValue("id", created.ID)
	updateW := httptest.NewRecorder()
	s.handleUpdateTask(updateW, updateReq)
	if updateW.Code != http.StatusOK {
		t.Fatalf("update task status = %d, want %d; body=%s", updateW.Code, http.StatusOK, updateW.Body.String())
	}

	eventBody := fmt.Sprintf(`{
		"event_type":"memory_ref",
		"summary":"linked worker lesson",
		"memory_id":"%s",
		"actor_role":"worker"
	}`, memory.ID)
	eventReq := httptest.NewRequest("POST", "/tasks/"+created.ID+"/events", strings.NewReader(eventBody))
	eventReq.Header.Set("Content-Type", "application/json")
	eventReq.SetPathValue("id", created.ID)
	eventW := httptest.NewRecorder()
	s.handleCreateTaskEvent(eventW, eventReq)
	if eventW.Code != http.StatusCreated {
		t.Fatalf("create task event status = %d, want %d; body=%s", eventW.Code, http.StatusCreated, eventW.Body.String())
	}

	commReq := httptest.NewRequest("POST", "/tasks/"+created.ID+"/events", strings.NewReader(`{
		"event_type":"communication",
		"summary":"worker update",
		"content":"embedding cache is wired, moving to queue API",
		"actor_role":"worker",
		"actor_name":"codex-worker"
	}`))
	commReq.Header.Set("Content-Type", "application/json")
	commReq.SetPathValue("id", created.ID)
	commW := httptest.NewRecorder()
	s.handleCreateTaskEvent(commW, commReq)
	if commW.Code != http.StatusCreated {
		t.Fatalf("create communication event status = %d, want %d; body=%s", commW.Code, http.StatusCreated, commW.Body.String())
	}

	listReq := httptest.NewRequest("GET", "/tasks?project=proj&status=started", nil)
	listW := httptest.NewRecorder()
	s.handleListTasks(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("list tasks status = %d, want %d", listW.Code, http.StatusOK)
	}
	var tasks []*db.Task
	if err := json.NewDecoder(listW.Body).Decode(&tasks); err != nil {
		t.Fatalf("decode task list: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != created.ID {
		t.Fatalf("unexpected task list: %+v", tasks)
	}

	eventsReq := httptest.NewRequest("GET", "/tasks/"+created.ID+"/events", nil)
	eventsReq.SetPathValue("id", created.ID)
	eventsW := httptest.NewRecorder()
	s.handleListTaskEvents(eventsW, eventsReq)
	if eventsW.Code != http.StatusOK {
		t.Fatalf("list task events status = %d, want %d", eventsW.Code, http.StatusOK)
	}
	var events []*db.TaskEvent
	if err := json.NewDecoder(eventsW.Body).Decode(&events); err != nil {
		t.Fatalf("decode task events: %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("expected 4 task events, got %d", len(events))
	}
	foundMemoryRef := false
	foundCommunication := false
	for _, event := range events {
		if event.EventType == db.TaskEventMemoryRef && event.MemoryID == memory.ID {
			foundMemoryRef = true
		}
		if event.EventType == db.TaskEventCommunication && event.ActorRole == "worker" {
			foundCommunication = true
		}
	}
	if !foundMemoryRef || !foundCommunication {
		t.Fatalf("missing expected task events: %+v", events)
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
	req = req.WithContext(auth.NewContext(req.Context(), &auth.Identity{Kind: "admin"}))
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
	s.auth = mustResolver(t, "test-secret", "")

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
	s.auth = mustResolver(t, "test-secret", "")

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
	s.auth = mustResolver(t, "test-secret", "")

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

func TestRequireAuthValidMachineToken(t *testing.T) {
	s := newTestServer(t)
	s.auth = mustResolver(t, "", `[{"token":"machine-secret","user":"UserA","machine_id":"MachineA","groups":["platform"]}]`)

	handler := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-MAGI-Auth-User"); got != "UserA" {
			t.Fatalf("X-MAGI-Auth-User=%q want UserA", got)
		}
		if got := r.Header.Get("X-MAGI-Auth-Machine"); got != "MachineA" {
			t.Fatalf("X-MAGI-Auth-Machine=%q want MachineA", got)
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer machine-secret")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("machine token: status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestMachineEnrollmentAndRevocationFlow(t *testing.T) {
	s := newTestServer(t)
	s.auth = mustResolver(t, "admin-secret", "")
	if lookup, ok := s.db.(auth.MachineLookup); ok {
		s.auth.SetMachineLookup(lookup)
	}

	enroll := s.requireAuth(s.handleEnrollMachine)
	body := `{"user":"UserA","machine_id":"MachineA","agent_name":"claude-main","agent_type":"claude","groups":["platform"]}`
	req := httptest.NewRequest("POST", "/auth/machines/enroll", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin-secret")
	w := httptest.NewRecorder()
	enroll(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("enroll status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var enrollResp struct {
		OK     bool                 `json:"ok"`
		Token  string               `json:"token"`
		Record db.MachineCredential `json:"record"`
	}
	if err := json.NewDecoder(w.Body).Decode(&enrollResp); err != nil {
		t.Fatalf("decode enroll response: %v", err)
	}
	if !enrollResp.OK || enrollResp.Token == "" || enrollResp.Record.ID == "" {
		t.Fatalf("unexpected enroll response: %+v", enrollResp)
	}

	authd := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-MAGI-Auth-User"); got != "UserA" {
			t.Fatalf("X-MAGI-Auth-User=%q want UserA", got)
		}
		if got := r.Header.Get("X-MAGI-Auth-Machine"); got != "MachineA" {
			t.Fatalf("X-MAGI-Auth-Machine=%q want MachineA", got)
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("Authorization", "Bearer "+enrollResp.Token)
	w2 := httptest.NewRecorder()
	authd(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("machine auth status = %d, want %d; body: %s", w2.Code, http.StatusOK, w2.Body.String())
	}

	revoke := s.requireAuth(s.handleRevokeMachineCredential)
	req3 := httptest.NewRequest("POST", "/auth/machines/"+enrollResp.Record.ID+"/revoke", nil)
	req3.SetPathValue("id", enrollResp.Record.ID)
	req3.Header.Set("Authorization", "Bearer admin-secret")
	w3 := httptest.NewRecorder()
	revoke(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("revoke status = %d, want %d; body: %s", w3.Code, http.StatusOK, w3.Body.String())
	}

	req4 := httptest.NewRequest("GET", "/", nil)
	req4.Header.Set("Authorization", "Bearer "+enrollResp.Token)
	w4 := httptest.NewRecorder()
	authd(w4, req4)
	if w4.Code != http.StatusUnauthorized {
		t.Fatalf("revoked machine token status = %d, want %d", w4.Code, http.StatusUnauthorized)
	}
}

type apiSecretManager struct{}

func (a *apiSecretManager) BackendName() string { return "vault" }

func (a *apiSecretManager) Externalize(_ context.Context, _ string, content string) (*secretstore.ExternalizeResult, error) {
	return &secretstore.ExternalizeResult{RedactedContent: content}, nil
}

func (a *apiSecretManager) Resolve(_ context.Context, path, key string) (string, error) {
	return "resolved:" + path + "#" + key, nil
}

func TestHandleResolveSecret(t *testing.T) {
	s := newTestServer(t)
	s.auth = mustResolver(t, "admin-secret", "")
	s.secrets = &apiSecretManager{}

	handler := s.requireAuth(s.handleResolveSecret)
	req := httptest.NewRequest("POST", "/auth/secrets/resolve", strings.NewReader(`{"path":"magi/proj/1","key":"api_key"}`))
	req.Header.Set("Authorization", "Bearer admin-secret")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if resp["backend"] != "vault" {
		t.Fatalf("backend = %v want vault", resp["backend"])
	}
	if resp["value"] != "resolved:magi/proj/1#api_key" {
		t.Fatalf("value = %v", resp["value"])
	}
}

func TestHandleSyncRememberWithMachineToken(t *testing.T) {
	s := newTestServer(t)
	s.auth = mustResolver(t, "", `[{"token":"machine-secret","user":"UserA","machine_id":"MachineA","groups":["platform"]}]`)

	handler := s.requireAuth(s.handleSyncRemember)
	req := httptest.NewRequest("POST", "/sync/memories", strings.NewReader(`{"content":"synced memory","project":"proj-sync"}`))
	req.Header.Set("Authorization", "Bearer machine-secret")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	id, _ := resp["id"].(string)
	if id == "" {
		t.Fatal("expected id")
	}
	mem, err := s.db.GetMemory(id)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if mem.Source != "magi-sync" {
		t.Fatalf("source = %q want magi-sync", mem.Source)
	}
}

func TestHandleConversationsAreScopedToOwner(t *testing.T) {
	s := newTestServer(t)
	s.auth = mustResolver(t, "", `[{"token":"token-a","user":"UserA","machine_id":"MachineA"},{"token":"token-b","user":"UserB","machine_id":"MachineB"}]`)

	create := s.requireAuth(s.handleCreateConversation)
	createReq := httptest.NewRequest("POST", "/conversations", strings.NewReader(`{"channel":"discord","summary":"deployment planning discussion"}`))
	createReq.Header.Set("Authorization", "Bearer token-a")
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	create(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body: %s", createW.Code, http.StatusCreated, createW.Body.String())
	}

	var createResp map[string]any
	if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	id, _ := createResp["id"].(string)
	if id == "" {
		t.Fatal("expected created conversation id")
	}

	list := s.requireAuth(s.handleListConversations)
	listReqB := httptest.NewRequest("GET", "/conversations", nil)
	listReqB.Header.Set("Authorization", "Bearer token-b")
	listWB := httptest.NewRecorder()
	list(listWB, listReqB)
	if listWB.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listWB.Code, http.StatusOK)
	}
	var listRespB []*db.Memory
	if err := json.NewDecoder(listWB.Body).Decode(&listRespB); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listRespB) != 0 {
		t.Fatalf("expected no conversations for UserB, got %d", len(listRespB))
	}

	get := s.requireAuth(s.handleGetConversation)
	getReqB := httptest.NewRequest("GET", "/conversations/"+id, nil)
	getReqB.SetPathValue("id", id)
	getReqB.Header.Set("Authorization", "Bearer token-b")
	getWB := httptest.NewRecorder()
	get(getWB, getReqB)
	if getWB.Code != http.StatusForbidden {
		t.Fatalf("get status = %d, want %d", getWB.Code, http.StatusForbidden)
	}

	searchHandler := s.requireAuth(s.handleSearchConversations)
	searchReqB := httptest.NewRequest("POST", "/conversations/search", strings.NewReader(`{"query":"deployment planning","limit":5}`))
	searchReqB.Header.Set("Authorization", "Bearer token-b")
	searchReqB.Header.Set("Content-Type", "application/json")
	searchWB := httptest.NewRecorder()
	searchHandler(searchWB, searchReqB)
	if searchWB.Code != http.StatusOK {
		t.Fatalf("search status = %d, want %d", searchWB.Code, http.StatusOK)
	}
	var searchRespB struct {
		Results []any `json:"results"`
	}
	if err := json.NewDecoder(searchWB.Body).Decode(&searchRespB); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	if len(searchRespB.Results) != 0 {
		t.Fatalf("expected no search results for UserB, got %d", len(searchRespB.Results))
	}

	listReqA := httptest.NewRequest("GET", "/conversations", nil)
	listReqA.Header.Set("Authorization", "Bearer token-a")
	listWA := httptest.NewRecorder()
	list(listWA, listReqA)
	if listWA.Code != http.StatusOK {
		t.Fatalf("owner list status = %d, want %d", listWA.Code, http.StatusOK)
	}
	var listRespA []*db.Memory
	if err := json.NewDecoder(listWA.Body).Decode(&listRespA); err != nil {
		t.Fatalf("decode owner list response: %v", err)
	}
	if len(listRespA) != 1 {
		t.Fatalf("expected one conversation for owner, got %d", len(listRespA))
	}
}

func TestHandleLegacyPrivateConversationHiddenFromMachine(t *testing.T) {
	s := newTestServer(t)
	s.auth = mustResolver(t, "", `[{"token":"token-b","user":"UserB","machine_id":"MachineB"}]`)

	emb, _ := s.embedder.Embed(context.Background(), "legacy private conversation")
	mem, err := s.db.SaveMemory(&db.Memory{
		Content:    "legacy private conversation",
		Summary:    "legacy",
		Embedding:  emb,
		Type:       "conversation",
		Visibility: "private",
		Source:     "discord",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if err := s.db.SetTags(mem.ID, []string{"conversation", "channel:discord"}); err != nil {
		t.Fatalf("SetTags: %v", err)
	}

	list := s.requireAuth(s.handleListConversations)
	listReq := httptest.NewRequest("GET", "/conversations", nil)
	listReq.Header.Set("Authorization", "Bearer token-b")
	listW := httptest.NewRecorder()
	list(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listW.Code, http.StatusOK)
	}
	var listResp []*db.Memory
	if err := json.NewDecoder(listW.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp) != 0 {
		t.Fatalf("expected no visible legacy private conversations, got %d", len(listResp))
	}

	get := s.requireAuth(s.handleGetConversation)
	getReq := httptest.NewRequest("GET", "/conversations/"+mem.ID, nil)
	getReq.SetPathValue("id", mem.ID)
	getReq.Header.Set("Authorization", "Bearer token-b")
	getW := httptest.NewRecorder()
	get(getW, getReq)
	if getW.Code != http.StatusForbidden {
		t.Fatalf("get status = %d, want %d", getW.Code, http.StatusForbidden)
	}
}

func TestHandleDeleteMemoryRequiresOwnerForMachine(t *testing.T) {
	s := newTestServer(t)
	s.auth = mustResolver(t, "admin-secret", `[{"token":"token-a","user":"UserA","machine_id":"MachineA"},{"token":"token-b","user":"UserB","machine_id":"MachineB"}]`)

	mem := seedMemory(t, s, "delete me", "proj", "memory")
	if err := s.db.SetTags(mem.ID, []string{"owner:UserA"}); err != nil {
		t.Fatalf("SetTags: %v", err)
	}

	handler := s.requireAuth(s.handleDeleteMemory)
	reqB := httptest.NewRequest("DELETE", "/memories/"+mem.ID, nil)
	reqB.SetPathValue("id", mem.ID)
	reqB.Header.Set("Authorization", "Bearer token-b")
	wB := httptest.NewRecorder()
	handler(wB, reqB)
	if wB.Code != http.StatusForbidden {
		t.Fatalf("machine delete status = %d, want %d", wB.Code, http.StatusForbidden)
	}

	reqAdmin := httptest.NewRequest("DELETE", "/memories/"+mem.ID, nil)
	reqAdmin.SetPathValue("id", mem.ID)
	reqAdmin.Header.Set("Authorization", "Bearer admin-secret")
	wAdmin := httptest.NewRecorder()
	handler(wAdmin, reqAdmin)
	if wAdmin.Code != http.StatusOK {
		t.Fatalf("admin delete status = %d, want %d", wAdmin.Code, http.StatusOK)
	}
}

func TestHandleSearchDoesNotRewriteWithPrivateParent(t *testing.T) {
	s := newTestServer(t)
	s.auth = mustResolver(t, "", `[{"token":"token-b","user":"UserB","machine_id":"MachineB"}]`)

	parentEmb, _ := s.embedder.Embed(context.Background(), "TOP SECRET parent content")
	parent, err := s.db.SaveMemory(&db.Memory{
		Content:    "TOP SECRET parent content",
		Summary:    "secret parent",
		Embedding:  parentEmb,
		Project:    "proj",
		Type:       "memory",
		Visibility: "private",
	})
	if err != nil {
		t.Fatalf("SaveMemory parent: %v", err)
	}
	if err := s.db.SetTags(parent.ID, []string{"owner:UserA"}); err != nil {
		t.Fatalf("SetTags parent: %v", err)
	}

	childEmb, _ := s.embedder.Embed(context.Background(), "child-query-marker visible child")
	child, err := s.db.SaveMemory(&db.Memory{
		Content:    "child-query-marker visible child",
		Summary:    "child",
		Embedding:  childEmb,
		Project:    "proj",
		Type:       "memory",
		Visibility: "internal",
		ParentID:   parent.ID,
	})
	if err != nil {
		t.Fatalf("SaveMemory child: %v", err)
	}
	if err := s.db.SetTags(child.ID, []string{"child"}); err != nil {
		t.Fatalf("SetTags child: %v", err)
	}

	handler := s.requireAuth(s.handleSearch)
	req := httptest.NewRequest("GET", "/search?q=child-query-marker", nil)
	req.Header.Set("Authorization", "Bearer token-b")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("search status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp []*db.HybridResult
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp) == 0 || resp[0].Memory == nil {
		t.Fatal("expected at least one search result")
	}
	if resp[0].Memory.Content == "TOP SECRET parent content" {
		t.Fatalf("expected parent content to remain hidden, got %q", resp[0].Memory.Content)
	}
}

func mustResolver(t *testing.T, adminToken, machineJSON string) *auth.Resolver {
	t.Helper()
	t.Setenv("MAGI_API_TOKEN", adminToken)
	t.Setenv("MAGI_MACHINE_TOKENS_JSON", machineJSON)
	t.Setenv("MAGI_MACHINE_TOKENS_FILE", "")
	resolver, err := auth.LoadResolverFromEnv()
	if err != nil {
		t.Fatalf("LoadResolverFromEnv: %v", err)
	}
	return resolver
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
