package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

// ---------- helpers to seed data ----------

// createMemoryViaAPI posts a form to /api/memories and returns the response code.
func createMemoryViaAPI(t *testing.T, mux http.Handler, content, project, tags string) int {
	t.Helper()
	form := "content=" + content
	if project != "" {
		form += "&project=" + project
	}
	if tags != "" {
		form += "&tags=" + tags
	}
	req := httptest.NewRequest("POST", "/api/memories", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w.Code
}

// createAndGetMemoryID creates a memory and returns its ID by querying the list API.
func createAndGetMemoryID(t *testing.T, mux http.Handler, content string) string {
	t.Helper()
	code := createMemoryViaAPI(t, mux, content, "", "")
	if code != http.StatusOK {
		t.Fatalf("createAndGetMemoryID: POST /api/memories returned %d", code)
	}

	req := httptest.NewRequest("GET", "/api/memories", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var memories []db.Memory
	if err := json.Unmarshal(w.Body.Bytes(), &memories); err != nil {
		t.Fatalf("createAndGetMemoryID: unmarshal: %v", err)
	}
	for _, m := range memories {
		if m.Content == content {
			return m.ID
		}
	}
	t.Fatalf("createAndGetMemoryID: memory with content %q not found", content)
	return ""
}

// ---------- apiCreateMemory ----------

func TestAPICreateMemory_Success(t *testing.T) {
	mux := newTestMux(t)
	code := createMemoryViaAPI(t, mux, "test memory content", "myproject", "tag1,tag2")
	if code != http.StatusOK {
		t.Errorf("POST /api/memories status = %d, want %d", code, http.StatusOK)
	}
}

func TestAPICreateMemory_MissingContent(t *testing.T) {
	mux := newTestMux(t)
	req := httptest.NewRequest("POST", "/api/memories", strings.NewReader("content="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /api/memories (empty content) status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAPICreateMemory_NoBody(t *testing.T) {
	mux := newTestMux(t)
	req := httptest.NewRequest("POST", "/api/memories", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /api/memories (no body) status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAPICreateMemory_WithAllFields(t *testing.T) {
	mux := newTestMux(t)
	form := "content=detailed+memory&summary=short&type=note&speaker=user&area=work&sub_area=magi&project=proj1&tags=a,b,c"
	req := httptest.NewRequest("POST", "/api/memories", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST /api/memories (all fields) status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

// ---------- apiDeleteMemory ----------

func TestAPIDeleteMemory_Existing(t *testing.T) {
	mux := newTestMux(t)
	id := createAndGetMemoryID(t, mux, "to be deleted")

	req := httptest.NewRequest("DELETE", "/api/memories/"+id, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("DELETE /api/memories/%s status = %d, want %d", id, w.Code, http.StatusOK)
	}

	// Verify it's gone
	req2 := httptest.NewRequest("GET", "/api/memories", nil)
	req2.Header.Set("Accept", "application/json")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	var memories []db.Memory
	json.Unmarshal(w2.Body.Bytes(), &memories)
	for _, m := range memories {
		if m.ID == id {
			t.Errorf("memory %s still exists after deletion", id)
		}
	}
}

// ---------- apiRelatedMemories ----------

func TestAPIRelatedMemories_NoRelations(t *testing.T) {
	mux := newTestMux(t)
	id := createAndGetMemoryID(t, mux, "lonely memory")

	req := httptest.NewRequest("GET", "/api/memories/"+id+"/related", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/memories/%s/related status = %d, want %d; body: %s", id, w.Code, http.StatusOK, w.Body.String())
	}
}

func TestAPIRelatedMemories_NonexistentID(t *testing.T) {
	mux := newTestMux(t)
	req := httptest.NewRequest("GET", "/api/memories/nonexistent-id/related", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should return 200 with empty results (no links found)
	if w.Code != http.StatusOK {
		t.Errorf("GET /api/memories/nonexistent-id/related status = %d, want %d", w.Code, http.StatusOK)
	}
}

// ---------- apiConversationsList ----------

func TestAPIConversationsList_Empty(t *testing.T) {
	mux := newTestMux(t)
	req := httptest.NewRequest("GET", "/api/conversations/list", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/conversations/list status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAPIConversationsList_WithChannel(t *testing.T) {
	mux := newTestMux(t)
	req := httptest.NewRequest("GET", "/api/conversations/list?channel=discord", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/conversations/list?channel=discord status = %d, want %d", w.Code, http.StatusOK)
	}
}

// ---------- apiConversationsSearch ----------

func TestAPIConversationsSearch_WithQuery(t *testing.T) {
	mux := newTestMux(t)
	body := `{"query":"test search","channel":""}`
	req := httptest.NewRequest("POST", "/api/conversations/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST /api/conversations/search status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestAPIConversationsSearch_EmptyQuery(t *testing.T) {
	mux := newTestMux(t)
	body := `{"query":"","channel":""}`
	req := httptest.NewRequest("POST", "/api/conversations/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Empty query delegates to apiConversationsList
	if w.Code != http.StatusOK {
		t.Errorf("POST /api/conversations/search (empty query) status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAPIConversationsSearch_InvalidJSON(t *testing.T) {
	mux := newTestMux(t)
	req := httptest.NewRequest("POST", "/api/conversations/search", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Invalid JSON returns 200 with empty partial
	if w.Code != http.StatusOK {
		t.Errorf("POST /api/conversations/search (invalid JSON) status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAPIConversationsSearch_WithChannel(t *testing.T) {
	mux := newTestMux(t)
	body := `{"query":"infrastructure","channel":"discord"}`
	req := httptest.NewRequest("POST", "/api/conversations/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST /api/conversations/search (with channel) status = %d, want %d", w.Code, http.StatusOK)
	}
}

// ---------- handleIngest ----------

func TestHandleIngest_Plaintext(t *testing.T) {
	mux := newTestMux(t)
	content := "User: Hello there\nAssistant: Hi! How can I help?"
	req := httptest.NewRequest("POST", "/ingest", strings.NewReader(content))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST /ingest (plaintext) status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp ingestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal ingest response: %v", err)
	}
	if resp.Error != "" {
		t.Errorf("POST /ingest returned error: %s", resp.Error)
	}
}

func TestHandleIngest_EmptyBody(t *testing.T) {
	mux := newTestMux(t)
	req := httptest.NewRequest("POST", "/ingest", strings.NewReader(""))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST /ingest (empty) status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp ingestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal ingest response: %v", err)
	}
	// Empty body should result in a parse error
	if resp.Error == "" {
		t.Error("expected error for empty ingest body")
	}
}

func TestHandleIngest_JSONConversation(t *testing.T) {
	mux := newTestMux(t)
	content := `[
		{"role": "user", "content": "What is Go?"},
		{"role": "assistant", "content": "Go is a programming language."}
	]`
	req := httptest.NewRequest("POST", "/ingest", strings.NewReader(content))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST /ingest (JSON) status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

// ---------- handleDetectFormat ----------

func TestHandleDetectFormat_JSON(t *testing.T) {
	mux := newTestMux(t)
	content := `[{"role":"user","content":"hello"}]`
	req := httptest.NewRequest("POST", "/api/ingest/detect", strings.NewReader(content))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST /api/ingest/detect (JSON) status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp detectResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal detect response: %v", err)
	}
	if resp.Format == "" {
		t.Error("expected non-empty format")
	}
}

func TestHandleDetectFormat_Plaintext(t *testing.T) {
	mux := newTestMux(t)
	content := "just some plain text"
	req := httptest.NewRequest("POST", "/api/ingest/detect", strings.NewReader(content))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST /api/ingest/detect (plaintext) status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp detectResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal detect response: %v", err)
	}
	if resp.Format == "" {
		t.Error("expected non-empty format for plaintext")
	}
}

func TestHandleDetectFormat_Empty(t *testing.T) {
	mux := newTestMux(t)
	req := httptest.NewRequest("POST", "/api/ingest/detect", strings.NewReader(""))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST /api/ingest/detect (empty) status = %d, want %d", w.Code, http.StatusOK)
	}
}

// ---------- apiAnalyzePatterns ----------

func TestAPIAnalyzePatterns_Empty(t *testing.T) {
	mux := newTestMux(t)
	req := httptest.NewRequest("POST", "/api/analyze-patterns", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST /api/analyze-patterns status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal analyze-patterns response: %v", err)
	}
	if _, ok := result["patterns_found"]; !ok {
		t.Error("expected patterns_found key in response")
	}
}

func TestAPIAnalyzePatterns_HTMXRequest(t *testing.T) {
	mux := newTestMux(t)
	req := httptest.NewRequest("POST", "/api/analyze-patterns", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST /api/analyze-patterns (HTMX) status = %d, want %d", w.Code, http.StatusOK)
	}
	if redirect := w.Header().Get("HX-Redirect"); redirect != "/patterns" {
		t.Errorf("expected HX-Redirect=/patterns, got %q", redirect)
	}
}

// ---------- detailPage ----------

func TestDetailPage_ExistingMemory(t *testing.T) {
	mux := newTestMux(t)
	id := createAndGetMemoryID(t, mux, "memory for detail page")

	req := httptest.NewRequest("GET", "/memory/"+id, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /memory/%s status = %d, want %d; body: %s", id, w.Code, http.StatusOK, w.Body.String())
	}

	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("expected Content-Type text/html, got %q", ct)
	}
}

func TestDetailPage_NonexistentMemory(t *testing.T) {
	mux := newTestMux(t)
	req := httptest.NewRequest("GET", "/memory/does-not-exist", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// GetMemory returns error for nonexistent -> serverError -> 500
	if w.Code != http.StatusInternalServerError {
		t.Errorf("GET /memory/does-not-exist status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------- memoryPartial (HTMX) ----------

func TestMemoryPartial_ExistingMemory(t *testing.T) {
	mux := newTestMux(t)
	id := createAndGetMemoryID(t, mux, "memory for partial")

	req := httptest.NewRequest("GET", "/memory/"+id+"/partial", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /memory/%s/partial status = %d, want %d; body: %s", id, w.Code, http.StatusOK, w.Body.String())
	}

	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("expected Content-Type text/html, got %q", ct)
	}
}

func TestMemoryPartial_NonexistentMemory(t *testing.T) {
	mux := newTestMux(t)
	req := httptest.NewRequest("GET", "/memory/does-not-exist/partial", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("GET /memory/does-not-exist/partial status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------- Helper functions ----------

func TestSpeakerBadge(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"user", "badge-green"},
		{"assistant", "badge-purple"},
		{"agent", "badge-blue"},
		{"system", "badge-gray"},
		{"unknown", "badge-gray"},
		{"", "badge-gray"},
	}
	for _, tt := range tests {
		got := speakerBadge(tt.input)
		if got != tt.want {
			t.Errorf("speakerBadge(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAreaBadge(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"work", "badge-blue"},
		{"infrastructure", "badge-amber"},
		{"personal", "badge-green"},
		{"development", "badge-pink"},
		{"project", "badge-purple"},
		{"meta", "badge-gray"},
		{"other", "badge-gray"},
		{"", "badge-gray"},
	}
	for _, tt := range tests {
		got := areaBadge(tt.input)
		if got != tt.want {
			t.Errorf("areaBadge(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSpeakerColor(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"user", "#10b981"},
		{"assistant", "#a855f7"},
		{"agent", "#3b82f6"},
		{"system", "#64748b"},
		{"unknown", "#64748b"},
	}
	for _, tt := range tests {
		got := speakerColor(tt.input)
		if got != tt.want {
			t.Errorf("speakerColor(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAreaColor(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"work", "#3b82f6"},
		{"infrastructure", "#f59e0b"},
		{"personal", "#10b981"},
		{"development", "#ec4899"},
		{"project", "#a855f7"},
		{"meta", "#64748b"},
		{"other", "#64748b"},
	}
	for _, tt := range tests {
		got := areaColor(tt.input)
		if got != tt.want {
			t.Errorf("areaColor(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestChannelBadge(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"discord", "badge-discord"},
		{"webchat", "badge-webchat"},
		{"claude-code", "badge-claude-code"},
		{"slack", "badge-slack"},
		{"irc", "badge-channel"},
		{"", "badge-channel"},
	}
	for _, tt := range tests {
		got := channelBadge(tt.input)
		if got != tt.want {
			t.Errorf("channelBadge(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGroupByDate(t *testing.T) {
	now := time.Now()
	today := now.Format(time.DateTime)
	yesterday := now.AddDate(0, 0, -1).Format(time.DateTime)
	lastWeek := now.AddDate(0, 0, -3).Format(time.DateTime)
	oldDate := now.AddDate(0, -3, 0).Format(time.DateTime)

	memories := []*db.Memory{
		{ID: "1", CreatedAt: today},
		{ID: "2", CreatedAt: yesterday},
		{ID: "3", CreatedAt: lastWeek},
		{ID: "4", CreatedAt: oldDate},
		{ID: "5", CreatedAt: "invalid-date"},
	}

	groups := groupByDate(memories)
	if len(groups) == 0 {
		t.Fatal("expected at least one group")
	}

	// Check that we got the expected labels
	labels := make(map[string]bool)
	for _, g := range groups {
		labels[g.Label] = true
		if len(g.Conversations) == 0 {
			t.Errorf("group %q has no conversations", g.Label)
		}
	}

	if !labels["Today"] {
		t.Error("expected 'Today' group")
	}
	if !labels["Yesterday"] {
		t.Error("expected 'Yesterday' group")
	}
	if !labels["Unknown"] {
		t.Error("expected 'Unknown' group for invalid date")
	}
}

func TestGroupByDate_Empty(t *testing.T) {
	groups := groupByDate(nil)
	if len(groups) != 0 {
		t.Errorf("expected 0 groups for nil input, got %d", len(groups))
	}
}

func TestGetNavFromData(t *testing.T) {
	tests := []struct {
		name string
		data any
		want string
	}{
		{"conversationsData", conversationsData{Nav: "conversations"}, "conversations"},
		{"*conversationsData", &conversationsData{Nav: "conversations"}, "conversations"},
		{"patternsData", patternsData{Nav: "patterns"}, "patterns"},
		{"*patternsData", &patternsData{Nav: "patterns"}, "patterns"},
		{"statsData", statsData{Nav: "stats"}, "stats"},
		{"*statsData", &statsData{Nav: "stats"}, "stats"},
		{"listData", listData{Nav: "list"}, "list"},
		{"*listData", &listData{Nav: "list"}, "list"},
		{"unknown type", "a string", ""},
		{"nil", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getNavFromData(tt.data)
			if got != tt.want {
				t.Errorf("getNavFromData(%v) = %q, want %q", tt.data, got, tt.want)
			}
		})
	}
}

// ---------- apiMemories with JSON accept ----------

func TestAPIMemories_JSONAccept(t *testing.T) {
	mux := newTestMux(t)

	// Seed a memory first
	createMemoryViaAPI(t, mux, "json accept test memory", "", "")

	req := httptest.NewRequest("GET", "/api/memories", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/memories (JSON) status = %d, want %d", w.Code, http.StatusOK)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var memories []db.Memory
	if err := json.Unmarshal(w.Body.Bytes(), &memories); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(memories) == 0 {
		t.Error("expected at least one memory")
	}
}

// ---------- apiMemories with filter params ----------

func TestAPIMemories_WithFilters(t *testing.T) {
	mux := newTestMux(t)

	req := httptest.NewRequest("GET", "/api/memories?speaker=user&area=work&type=note&offset=0", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/memories (filters) status = %d, want %d", w.Code, http.StatusOK)
	}
}

// ---------- apiSearch with JSON accept ----------

func TestAPISearch_JSONAcceptWithQuery(t *testing.T) {
	mux := newTestMux(t)

	// Seed a memory
	createMemoryViaAPI(t, mux, "searchable content about golang", "", "")

	req := httptest.NewRequest("GET", "/api/search?q=golang", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/search?q=golang (JSON) status = %d, want %d", w.Code, http.StatusOK)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

// ---------- apiRelatedMemories with JSON accept ----------

func TestAPIRelatedMemories_JSONAccept(t *testing.T) {
	mux := newTestMux(t)
	id := createAndGetMemoryID(t, mux, "related test json")

	req := httptest.NewRequest("GET", "/api/memories/"+id+"/related", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

// ---------- conversations page with channel filter ----------

func TestConversationsPage_WithChannel(t *testing.T) {
	mux := newTestMux(t)

	req := httptest.NewRequest("GET", "/conversations?channel=discord", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /conversations?channel=discord status = %d, want %d", w.Code, http.StatusOK)
	}
}

// ---------- serverError ----------

func TestServerError(t *testing.T) {
	mux := newTestMux(t)

	// Trigger a serverError by requesting a nonexistent memory detail
	req := httptest.NewRequest("GET", "/memory/nonexistent-uuid", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nonexistent memory detail, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "internal server error") {
		t.Errorf("expected 'internal server error' in body, got %q", body)
	}
}

// ---------- listPage with offset/filter params ----------

func TestListPage_WithParams(t *testing.T) {
	mux := newTestMux(t)

	req := httptest.NewRequest("GET", "/?speaker=user&area=work&offset=0", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET / (with params) status = %d, want %d", w.Code, http.StatusOK)
	}
}

// ---------- apiSearch HTMX (no Accept JSON) ----------

func TestAPISearch_HTMXPartial(t *testing.T) {
	mux := newTestMux(t)

	req := httptest.NewRequest("GET", "/api/search?q=test", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/search?q=test (HTMX) status = %d, want %d", w.Code, http.StatusOK)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("expected text/html for HTMX partial, got %q", ct)
	}
}

// ---------- apiSearch with empty query, no JSON accept (HTMX partial) ----------

func TestAPISearch_EmptyQueryHTMX(t *testing.T) {
	mux := newTestMux(t)

	req := httptest.NewRequest("GET", "/api/search", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/search (empty, HTMX) status = %d, want %d", w.Code, http.StatusOK)
	}
}
