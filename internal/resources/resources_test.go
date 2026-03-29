package resources

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/j33pguy/magi/internal/db"
)

func newTestDB(t *testing.T) *db.Client {
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
	return client.TursoClient
}

func zeroEmbedding() []float32 {
	return make([]float32, 384)
}

func seedMemory(t *testing.T, c *db.Client, content, project, typ, visibility string, tags []string) *db.Memory {
	t.Helper()
	m, err := c.SaveMemory(&db.Memory{
		Content:    content,
		Embedding:  zeroEmbedding(),
		Project:    project,
		Type:       typ,
		Visibility: visibility,
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if len(tags) > 0 {
		if err := c.SetTags(m.ID, tags); err != nil {
			t.Fatalf("SetTags: %v", err)
		}
	}
	return m
}

// --- Recent ---

func TestRecent_Template(t *testing.T) {
	r := &Recent{DB: newTestDB(t)}
	tmpl := r.Template()
	if tmpl.URITemplate.Raw() != "memory://recent/{project}" {
		t.Errorf("URI = %q", tmpl.URITemplate.Raw())
	}
}

func TestRecent_Handle_Empty(t *testing.T) {
	r := &Recent{DB: newTestDB(t)}
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "memory://recent/test-project"

	contents, err := r.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}
	text := contents[0].(mcp.TextResourceContents)
	if text.MIMEType != "application/json" {
		t.Errorf("MIME = %q", text.MIMEType)
	}
}

func TestRecent_Handle_WithData(t *testing.T) {
	c := newTestDB(t)
	seedMemory(t, c, "hello", "proj", "memory", "internal", nil)

	r := &Recent{DB: c}
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "memory://recent/proj"

	contents, err := r.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents)
	if !strings.Contains(text.Text, "hello") {
		t.Error("expected memory content in response")
	}
}

// --- Decisions ---

func TestDecisions_Template(t *testing.T) {
	d := &Decisions{DB: newTestDB(t)}
	tmpl := d.Template()
	if tmpl.URITemplate.Raw() != "memory://decisions/{project}" {
		t.Errorf("URI = %q", tmpl.URITemplate.Raw())
	}
}

func TestDecisions_Handle_Empty(t *testing.T) {
	d := &Decisions{DB: newTestDB(t)}
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "memory://decisions/proj"

	contents, err := d.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}
}

func TestDecisions_Handle_WithData(t *testing.T) {
	c := newTestDB(t)
	seedMemory(t, c, "use postgres", "proj", "decision", "internal", nil)

	d := &Decisions{DB: c}
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "memory://decisions/proj"

	contents, err := d.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents)
	if !strings.Contains(text.Text, "use postgres") {
		t.Error("expected decision content in response")
	}
}

// --- Preferences ---

func TestPreferences_Resource(t *testing.T) {
	p := &Preferences{DB: newTestDB(t)}
	res := p.Resource()
	if res.URI != "memory://preferences" {
		t.Errorf("URI = %q", res.URI)
	}
}

func TestPreferences_Handle_Empty(t *testing.T) {
	p := &Preferences{DB: newTestDB(t)}
	contents, err := p.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}
}

func TestPreferences_Handle_WithData(t *testing.T) {
	c := newTestDB(t)
	seedMemory(t, c, "dark mode", "proj", "preference", "internal", nil)

	p := &Preferences{DB: c}
	contents, err := p.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents)
	if !strings.Contains(text.Text, "dark mode") {
		t.Error("expected preference content in response")
	}
}

// --- Context ---

func TestContext_Resource(t *testing.T) {
	c := &Context{DB: newTestDB(t)}
	res := c.Resource()
	if res.URI != "memory://context" {
		t.Errorf("URI = %q", res.URI)
	}
}

func TestContext_Handle_Empty(t *testing.T) {
	c := &Context{DB: newTestDB(t)}
	contents, err := c.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}
}

func TestContext_Handle_WithData(t *testing.T) {
	dbClient := newTestDB(t)
	seedMemory(t, dbClient, "context info", "proj", "memory", "internal", nil)

	t.Setenv("PROJECT_NAME", "proj")
	c := &Context{DB: dbClient}
	contents, err := c.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents)
	if !strings.Contains(text.Text, "context info") {
		t.Error("expected context memory in response")
	}
}

// --- Conversations ---

func TestRecentConversations_Resource(t *testing.T) {
	r := &RecentConversations{DB: newTestDB(t)}
	res := r.Resource()
	if res.URI != "memory://conversations/recent" {
		t.Errorf("URI = %q", res.URI)
	}
}

func TestRecentConversations_Handle_Empty(t *testing.T) {
	r := &RecentConversations{DB: newTestDB(t)}
	contents, err := r.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}
}

func TestRecentConversations_Handle_WithData(t *testing.T) {
	c := newTestDB(t)
	seedMemory(t, c, "conv summary", "proj", "conversation", "internal", []string{"conversation"})

	r := &RecentConversations{DB: c}
	contents, err := r.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents)
	if !strings.Contains(text.Text, "conv summary") {
		t.Error("expected conversation content in response")
	}
}

// --- Patterns ---

func TestPatterns_Resource(t *testing.T) {
	p := &Patterns{DB: newTestDB(t)}
	res := p.Resource()
	if res.URI != "memory://patterns" {
		t.Errorf("URI = %q", res.URI)
	}
}

func TestPatterns_Handle_Empty(t *testing.T) {
	p := &Patterns{DB: newTestDB(t)}
	contents, err := p.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}
	// Should return empty array, not null
	text := contents[0].(mcp.TextResourceContents)
	if !strings.Contains(text.Text, "[]") {
		t.Error("expected empty array for no patterns")
	}
}

func TestPatterns_Handle_WithData(t *testing.T) {
	c := newTestDB(t)
	seedMemory(t, c, "always uses tabs", "proj", "memory", "internal", []string{"pattern"})

	p := &Patterns{DB: c}
	contents, err := p.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents)
	if !strings.Contains(text.Text, "always uses tabs") {
		t.Error("expected pattern content in response")
	}
}

// --- extractParam ---

func TestExtractParam(t *testing.T) {
	result := extractParam("memory://recent/my-project", "memory://recent/")
	if result != "my-project" {
		t.Errorf("extractParam = %q, want %q", result, "my-project")
	}
}
