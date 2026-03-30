package web

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

// ---------- getStats: TopArea skips empty-name areas ----------

func TestExtra_GetStatsTopAreaSkipsEmpty(t *testing.T) {
	// Seed memories where the first area (by count) is empty string, so TopArea
	// must skip it and pick the next non-empty one.
	mux, client := newTestMuxAndDB(t)

	embedder := &mockEmbedder{}
	// Create 3 memories with empty area (most common) and 2 with "work".
	for i := 0; i < 3; i++ {
		emb, _ := embedder.Embed(context.Background(), fmt.Sprintf("empty area mem %d", i))
		mem := &db.Memory{
			Content:    fmt.Sprintf("empty area mem %d", i),
			Summary:    "sum",
			Type:       "note",
			Speaker:    "user",
			Area:       "", // empty area
			Embedding:  emb,
			TokenCount: 5,
		}
		if _, err := client.SaveMemory(mem); err != nil {
			t.Fatalf("SaveMemory: %v", err)
		}
	}
	for i := 0; i < 2; i++ {
		emb, _ := embedder.Embed(context.Background(), fmt.Sprintf("work area mem %d", i))
		mem := &db.Memory{
			Content:    fmt.Sprintf("work area mem %d", i),
			Summary:    "sum",
			Type:       "note",
			Speaker:    "user",
			Area:       "work",
			Embedding:  emb,
			TokenCount: 5,
		}
		if _, err := client.SaveMemory(mem); err != nil {
			t.Fatalf("SaveMemory: %v", err)
		}
	}

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/stats status = %d; body: %s", w.Code, w.Body.String())
	}

	var stats statsData
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if stats.TopArea != "work" {
		t.Errorf("TopArea = %q, want %q (should skip empty)", stats.TopArea, "work")
	}
	if stats.TotalMemories != 5 {
		t.Errorf("TotalMemories = %d, want 5", stats.TotalMemories)
	}
	// Verify percentages are calculated
	for _, sc := range stats.SpeakerCounts {
		if sc.Percent == 0 && sc.Count > 0 {
			t.Errorf("SpeakerCounts percent is 0 for %q with count %d", sc.Name, sc.Count)
		}
	}
	for _, ac := range stats.AreaCounts {
		if ac.Percent == 0 && ac.Count > 0 {
			t.Errorf("AreaCounts percent is 0 for %q with count %d", ac.Name, ac.Count)
		}
	}
}

// ---------- getStats: tags are populated ----------

func TestExtra_GetStatsWithTags(t *testing.T) {
	mux, client := newTestMuxAndDB(t)

	embedder := &mockEmbedder{}
	emb, _ := embedder.Embed(context.Background(), "tagged memory")
	mem := &db.Memory{
		Content:    "tagged memory for stats",
		Summary:    "sum",
		Type:       "note",
		Speaker:    "user",
		Area:       "work",
		Embedding:  emb,
		TokenCount: 5,
	}
	saved, err := client.SaveMemory(mem)
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if err := client.SetTags(saved.ID, []string{"topic:golang", "area:work"}); err != nil {
		t.Fatalf("SetTags: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	var stats statsData
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(stats.TopTags) == 0 {
		t.Error("expected TopTags to be non-empty")
	}
}

// ---------- handleIngest: content that triggers area + sub_area classification ----------

func TestExtra_HandleIngestWithClassification(t *testing.T) {
	mux := newTestMux(t)

	// "proxmox" triggers classify.Infer to return area="homelab", subArea="proxmox"
	content := "User: How do I set up a Proxmox cluster with three nodes?\nAssistant: First configure your network, then install Proxmox on each node."
	req := httptest.NewRequest("POST", "/ingest", strings.NewReader(content))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /ingest status = %d; body: %s", w.Code, w.Body.String())
	}

	var resp ingestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
	if resp.Imported == 0 {
		t.Error("expected at least one imported memory")
	}
}

func TestExtra_HandleIngestWithAreaAndSubArea(t *testing.T) {
	// Use "Power BI" which triggers area="work", subArea="power-bi"
	mux := newTestMux(t)

	content := "User: I need help with my Power BI dashboard and DAX formulas\nAssistant: Here are some Power BI tips for DAX measures."
	req := httptest.NewRequest("POST", "/ingest", strings.NewReader(content))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /ingest status = %d; body: %s", w.Code, w.Body.String())
	}

	var resp ingestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
}

// ---------- handleIngest: multi-turn conversation producing multiple memories ----------

func TestExtra_HandleIngestMultiTurn(t *testing.T) {
	mux := newTestMux(t)

	// Multi-turn conversation with distinct content that should extract multiple memories
	content := `User: I decided to use Ansible for all Proxmox LXC provisioning instead of manual setup
Assistant: That makes sense. Ansible gives you repeatable infrastructure as code.
User: Also I set up Grafana dashboards for monitoring the cluster
Assistant: Great choice for monitoring. You can track CPU, memory, and network metrics.
User: My wife suggested we get the kids into coding with Scratch
Assistant: That's a wonderful family activity! Scratch is very beginner-friendly.`

	req := httptest.NewRequest("POST", "/ingest", strings.NewReader(content))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /ingest status = %d; body: %s", w.Code, w.Body.String())
	}

	var resp ingestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
	// Should have imported multiple memories covering different areas
	if resp.Imported == 0 {
		t.Error("expected at least one imported memory from multi-turn conversation")
	}
}

// ---------- apiConversationsSearch: empty channel with query triggers tag-loading path ----------

func TestExtra_APIConversationsSearchLoadsTags(t *testing.T) {
	mux, client := newTestMuxAndDB(t)

	// Create a conversation memory without pre-loaded tags (Tags field will be nil
	// when returned from HybridSearch), which triggers the tag-loading branch.
	embedder := &mockEmbedder{}
	emb, _ := embedder.Embed(context.Background(), "kubernetes discussion")
	mem := &db.Memory{
		Content:    "kubernetes discussion about pods and services",
		Summary:    "k8s talk",
		Type:       "conversation",
		Speaker:    "user",
		Area:       "work",
		Embedding:  emb,
		TokenCount: 10,
	}
	saved, err := client.SaveMemory(mem)
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if err := client.SetTags(saved.ID, []string{"conversation", "topic:kubernetes"}); err != nil {
		t.Fatalf("SetTags: %v", err)
	}

	body := `{"query":"kubernetes","channel":""}`
	req := httptest.NewRequest("POST", "/api/conversations/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST /api/conversations/search status = %d; body: %s", w.Code, w.Body.String())
	}
}

// ---------- apiConversationsSearch: with channel filter on seeded data ----------

func TestExtra_APIConversationsSearchWithChannelAndData(t *testing.T) {
	mux, client := newTestMuxAndDB(t)

	embedder := &mockEmbedder{}
	emb, _ := embedder.Embed(context.Background(), "discord chat about golang")
	mem := &db.Memory{
		Content:    "discord chat about golang generics and interfaces",
		Summary:    "go generics",
		Type:       "conversation",
		Speaker:    "user",
		Area:       "work",
		Embedding:  emb,
		TokenCount: 10,
	}
	saved, err := client.SaveMemory(mem)
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if err := client.SetTags(saved.ID, []string{"conversation", "channel:discord", "topic:golang"}); err != nil {
		t.Fatalf("SetTags: %v", err)
	}

	body := `{"query":"golang","channel":"discord"}`
	req := httptest.NewRequest("POST", "/api/conversations/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; body: %s", w.Code, w.Body.String())
	}
}

// ---------- apiConversationsList: tag loading path with seeded conversation data ----------

func TestExtra_APIConversationsListTagLoading(t *testing.T) {
	mux, client := newTestMuxAndDB(t)

	embedder := &mockEmbedder{}
	emb, _ := embedder.Embed(context.Background(), "conversation for tag loading")
	mem := &db.Memory{
		Content:    "conversation for tag loading test",
		Summary:    "conv tags",
		Type:       "conversation",
		Speaker:    "user",
		Area:       "work",
		Embedding:  emb,
		TokenCount: 5,
	}
	saved, err := client.SaveMemory(mem)
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if err := client.SetTags(saved.ID, []string{"conversation", "channel:webchat"}); err != nil {
		t.Fatalf("SetTags: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/conversations/list", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/conversations/list status = %d", w.Code)
	}
}

// ---------- patternsPage: tag loading for pattern memories ----------

func TestExtra_PatternsPageTagLoading(t *testing.T) {
	mux, client := newTestMuxAndDB(t)

	embedder := &mockEmbedder{}
	emb, _ := embedder.Embed(context.Background(), "user prefers vim over emacs")
	mem := &db.Memory{
		Content:    "user prefers vim over emacs for all text editing",
		Summary:    "vim preference",
		Type:       "pattern",
		Speaker:    "user",
		Area:       "work",
		Embedding:  emb,
		TokenCount: 10,
	}
	saved, err := client.SaveMemory(mem)
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if err := client.SetTags(saved.ID, []string{"pattern", "pattern_type:preference"}); err != nil {
		t.Fatalf("SetTags: %v", err)
	}

	req := httptest.NewRequest("GET", "/patterns", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /patterns status = %d; body: %s", w.Code, w.Body.String())
	}
}

// ---------- apiRelatedMemories: neighbor deleted (GetMemory error -> continue) ----------

func TestExtra_APIRelatedMemoriesDeletedNeighbor(t *testing.T) {
	mux, client := newTestMuxAndDB(t)

	embedder := &mockEmbedder{}
	emb1, _ := embedder.Embed(context.Background(), "mem1")
	mem1 := &db.Memory{Content: "source memory", Summary: "src", Type: "note", Speaker: "user", Embedding: emb1, TokenCount: 3}
	saved1, err := client.SaveMemory(mem1)
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	emb2, _ := embedder.Embed(context.Background(), "mem2")
	mem2 := &db.Memory{Content: "neighbor memory", Summary: "nbr", Type: "note", Speaker: "user", Embedding: emb2, TokenCount: 3}
	saved2, err := client.SaveMemory(mem2)
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	// Create link
	if _, err := client.CreateLink(context.Background(), saved1.ID, saved2.ID, "related_to", 0.9, true); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	// Delete the neighbor so GetMemory will fail for it
	if err := client.DeleteMemory(saved2.ID); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/memories/"+saved1.ID+"/related", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; body: %s", w.Code, w.Body.String())
	}

	// Should return empty results since the only neighbor is deleted
	var results []relatedMemoryResult
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// The neighbor was deleted, so it should be skipped
	if len(results) != 0 {
		t.Errorf("expected 0 results (neighbor deleted), got %d", len(results))
	}
}

// ---------- apiAnalyzePatterns: with user memories that produce patterns ----------

func TestExtra_APIAnalyzePatternsWithUserMemories(t *testing.T) {
	mux, client := newTestMuxAndDB(t)

	embedder := &mockEmbedder{}
	// Create several user memories with similar content to trigger pattern detection.
	// The patterns.Analyzer looks for repeated themes.
	contents := []string{
		"I always prefer using Go for backend services because of its simplicity",
		"Go is my preferred language for building APIs and microservices",
		"I chose Go again for this project because of strong typing and goroutines",
		"For this new service I went with Go over Python for performance reasons",
		"I decided to use Go for the CLI tool since it compiles to a single binary",
	}
	for i, c := range contents {
		emb, _ := embedder.Embed(context.Background(), c)
		if i < 384 {
			emb[i] = float32(i+1) * 0.1
		}
		mem := &db.Memory{
			Content:    c,
			Summary:    fmt.Sprintf("go preference %d", i),
			Type:       "note",
			Speaker:    "user",
			Area:       "work",
			Embedding:  emb,
			TokenCount: 15,
		}
		if _, err := client.SaveMemory(mem); err != nil {
			t.Fatalf("SaveMemory: %v", err)
		}
	}

	req := httptest.NewRequest("POST", "/api/analyze-patterns", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}

	var result map[string]int
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := result["patterns_found"]; !ok {
		t.Error("expected patterns_found key")
	}
	if _, ok := result["patterns_stored"]; !ok {
		t.Error("expected patterns_stored key")
	}
	if _, ok := result["skipped_duplicates"]; !ok {
		t.Error("expected skipped_duplicates key")
	}
}

// ---------- render: exercise map[string]interface{} branch ----------

func TestExtra_RenderMapStringInterface(t *testing.T) {
	mux, _ := newTestMuxAndDB(t)

	// The stats page uses *statsData which goes through the getNavFromData default branch.
	// The search page uses map[string]string.
	// We exercise the code by hitting endpoints that use different data types for render.

	// statsPage uses *statsData -> getNavFromData
	req := httptest.NewRequest("GET", "/stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /stats status = %d", w.Code)
	}

	// searchPage uses map[string]string -> render with map[string]string branch
	req2 := httptest.NewRequest("GET", "/search", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("GET /search status = %d", w2.Code)
	}

	// newPage uses map[string]string
	req3 := httptest.NewRequest("GET", "/new", nil)
	w3 := httptest.NewRecorder()
	mux.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Errorf("GET /new status = %d", w3.Code)
	}
}

// ---------- handleIngest: plaintext with Proxmox and Grafana content ----------

func TestExtra_HandleIngestPlaintextMultiArea(t *testing.T) {
	mux := newTestMux(t)

	// Plaintext conversation about Proxmox and Grafana (triggers area classification)
	content := "User: I want to set up Proxmox VE on my homelab server with LXC containers\nAssistant: Here is how to configure Proxmox VE with LXC containers for your homelab.\nUser: I also need to set up Grafana monitoring dashboards\nAssistant: You can connect Grafana to Prometheus for monitoring your Proxmox cluster."
	req := httptest.NewRequest("POST", "/ingest", strings.NewReader(content))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /ingest (plaintext multi-area) status = %d; body: %s", w.Code, w.Body.String())
	}

	var resp ingestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
}

// ---------- apiSearch: empty query returns empty partial (HTMX) ----------

func TestExtra_APISearchEmptyQueryHTMXPartial(t *testing.T) {
	mux := newTestMux(t)

	req := httptest.NewRequest("GET", "/api/search?q=", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/search?q= status = %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("expected text/html, got %q", ct)
	}
}

// ---------- apiCreateMemory: default type when type is empty ----------

func TestExtra_APICreateMemoryDefaultType(t *testing.T) {
	mux := newTestMux(t)

	// Post without type field -> defaults to "note"
	form := "content=memory+without+type"
	req := httptest.NewRequest("POST", "/api/memories", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; body: %s", w.Code, w.Body.String())
	}
}

// ---------- apiConversationsSearch: empty query with channel falls to list ----------

func TestExtra_APIConversationsSearchEmptyQueryWithChannel(t *testing.T) {
	mux, client := newTestMuxAndDB(t)

	embedder := &mockEmbedder{}
	emb, _ := embedder.Embed(context.Background(), "slack convo")
	mem := &db.Memory{
		Content:    "slack conversation about deployment",
		Summary:    "deploy chat",
		Type:       "conversation",
		Speaker:    "user",
		Area:       "work",
		Embedding:  emb,
		TokenCount: 5,
	}
	saved, err := client.SaveMemory(mem)
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if err := client.SetTags(saved.ID, []string{"conversation", "channel:slack"}); err != nil {
		t.Fatalf("SetTags: %v", err)
	}

	// Empty query with channel -> delegates to apiConversationsList
	body := `{"query":"","channel":"slack"}`
	req := httptest.NewRequest("POST", "/api/conversations/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
}
