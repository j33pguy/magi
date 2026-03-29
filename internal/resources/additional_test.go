package resources

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// withFailingMarshal replaces marshalJSON with a function that always fails,
// restoring the original after the test.
func withFailingMarshal(t *testing.T) {
	t.Helper()
	orig := marshalJSON
	marshalJSON = func(v any, prefix, indent string) ([]byte, error) {
		return nil, errors.New("synthetic marshal error")
	}
	t.Cleanup(func() { marshalJSON = orig })
}

// ---------- extractParam edge cases ----------

func TestExtractParam_EmptyURI(t *testing.T) {
	result := extractParam("", "memory://recent/")
	if result != "" {
		t.Errorf("extractParam empty = %q, want %q", result, "")
	}
}

func TestExtractParam_NoMatch(t *testing.T) {
	// When the prefix doesn't match, TrimPrefix returns the original string.
	result := extractParam("memory://other/foo", "memory://recent/")
	if result != "memory://other/foo" {
		t.Errorf("extractParam no-match = %q, want original URI", result)
	}
}

func TestExtractParam_SlashInProject(t *testing.T) {
	result := extractParam("memory://recent/org/sub-project", "memory://recent/")
	if result != "org/sub-project" {
		t.Errorf("extractParam slash = %q, want %q", result, "org/sub-project")
	}
}

// ---------- Recent: multiple results and project filtering ----------

func TestRecent_Handle_MultipleResults(t *testing.T) {
	c := newTestDB(t)
	seedMemory(t, c, "first", "alpha", "memory", "internal", nil)
	seedMemory(t, c, "second", "alpha", "memory", "internal", nil)
	seedMemory(t, c, "third", "alpha", "memory", "internal", nil)

	r := &Recent{DB: c}
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "memory://recent/alpha"

	contents, err := r.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents).Text
	for _, want := range []string{"first", "second", "third"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in response", want)
		}
	}
}

func TestRecent_Handle_FiltersProject(t *testing.T) {
	c := newTestDB(t)
	seedMemory(t, c, "alpha-mem", "alpha", "memory", "internal", nil)
	seedMemory(t, c, "beta-mem", "beta", "memory", "internal", nil)

	r := &Recent{DB: c}
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "memory://recent/alpha"

	contents, err := r.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents).Text
	if !strings.Contains(text, "alpha-mem") {
		t.Error("expected alpha-mem in response")
	}
	if strings.Contains(text, "beta-mem") {
		t.Error("should NOT contain beta-mem for project=alpha")
	}
}

func TestRecent_Handle_DBError(t *testing.T) {
	c := newTestDB(t)
	c.DB.Close() // close underlying sql.DB

	r := &Recent{DB: c}
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "memory://recent/proj"

	_, err := r.Handle(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
	if !strings.Contains(err.Error(), "listing recent memories") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------- Decisions: project filtering and type filtering ----------

func TestDecisions_Handle_MultipleResults(t *testing.T) {
	c := newTestDB(t)
	seedMemory(t, c, "decision-1", "proj", "decision", "internal", nil)
	seedMemory(t, c, "decision-2", "proj", "decision", "internal", nil)

	d := &Decisions{DB: c}
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "memory://decisions/proj"

	contents, err := d.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents).Text
	if !strings.Contains(text, "decision-1") || !strings.Contains(text, "decision-2") {
		t.Error("expected both decisions in response")
	}
}

func TestDecisions_Handle_FiltersType(t *testing.T) {
	c := newTestDB(t)
	seedMemory(t, c, "a-decision", "proj", "decision", "internal", nil)
	seedMemory(t, c, "a-memory", "proj", "memory", "internal", nil)

	d := &Decisions{DB: c}
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "memory://decisions/proj"

	contents, err := d.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents).Text
	if !strings.Contains(text, "a-decision") {
		t.Error("expected decision in response")
	}
	if strings.Contains(text, "a-memory") {
		t.Error("should NOT contain non-decision type")
	}
}

func TestDecisions_Handle_FiltersProject(t *testing.T) {
	c := newTestDB(t)
	seedMemory(t, c, "proj-a-dec", "proj-a", "decision", "internal", nil)
	seedMemory(t, c, "proj-b-dec", "proj-b", "decision", "internal", nil)

	d := &Decisions{DB: c}
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "memory://decisions/proj-a"

	contents, err := d.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents).Text
	if !strings.Contains(text, "proj-a-dec") {
		t.Error("expected proj-a decision")
	}
	if strings.Contains(text, "proj-b-dec") {
		t.Error("should NOT contain proj-b decision")
	}
}

func TestDecisions_Handle_DBError(t *testing.T) {
	c := newTestDB(t)
	c.DB.Close()

	d := &Decisions{DB: c}
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "memory://decisions/proj"

	_, err := d.Handle(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
	if !strings.Contains(err.Error(), "listing decisions") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDecisions_Handle_URIContents(t *testing.T) {
	d := &Decisions{DB: newTestDB(t)}
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "memory://decisions/my-proj"

	contents, err := d.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents)
	if text.URI != "memory://decisions/my-proj" {
		t.Errorf("URI = %q, want memory://decisions/my-proj", text.URI)
	}
}

// ---------- Preferences: type filtering and DB error ----------

func TestPreferences_Handle_MultipleResults(t *testing.T) {
	c := newTestDB(t)
	seedMemory(t, c, "pref-1", "proj", "preference", "internal", nil)
	seedMemory(t, c, "pref-2", "proj", "preference", "internal", nil)

	p := &Preferences{DB: c}
	contents, err := p.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents).Text
	if !strings.Contains(text, "pref-1") || !strings.Contains(text, "pref-2") {
		t.Error("expected both preferences in response")
	}
}

func TestPreferences_Handle_FiltersType(t *testing.T) {
	c := newTestDB(t)
	seedMemory(t, c, "a-pref", "proj", "preference", "internal", nil)
	seedMemory(t, c, "not-a-pref", "proj", "memory", "internal", nil)

	p := &Preferences{DB: c}
	contents, err := p.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents).Text
	if !strings.Contains(text, "a-pref") {
		t.Error("expected preference")
	}
	if strings.Contains(text, "not-a-pref") {
		t.Error("should NOT contain non-preference type")
	}
}

func TestPreferences_Handle_DBError(t *testing.T) {
	c := newTestDB(t)
	c.DB.Close()

	p := &Preferences{DB: c}
	_, err := p.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
	if !strings.Contains(err.Error(), "listing preferences") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------- Context: PROJECT_NAME filtering and DB error ----------

func TestContext_Handle_WithProjectEnv(t *testing.T) {
	c := newTestDB(t)
	seedMemory(t, c, "proj-context", "myproj", "memory", "internal", nil)

	t.Setenv("PROJECT_NAME", "myproj")
	ctx := &Context{DB: c}
	contents, err := ctx.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents).Text
	if !strings.Contains(text, "proj-context") {
		t.Error("expected context memory for project")
	}
}

func TestContext_Handle_WithoutProjectEnv(t *testing.T) {
	c := newTestDB(t)
	seedMemory(t, c, "any-context", "", "memory", "internal", nil)

	t.Setenv("PROJECT_NAME", "")
	ctx := &Context{DB: c}
	contents, err := ctx.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// Should succeed even without PROJECT_NAME
	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}
}

func TestContext_Handle_MultipleResults(t *testing.T) {
	c := newTestDB(t)
	seedMemory(t, c, "ctx-1", "proj", "memory", "internal", nil)
	seedMemory(t, c, "ctx-2", "proj", "memory", "internal", nil)
	seedMemory(t, c, "ctx-3", "proj", "decision", "internal", nil)

	t.Setenv("PROJECT_NAME", "proj")
	ctx := &Context{DB: c}
	contents, err := ctx.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents).Text
	if !strings.Contains(text, "ctx-1") || !strings.Contains(text, "ctx-2") {
		t.Error("expected multiple context memories in response")
	}
}

func TestContext_Handle_DBError(t *testing.T) {
	c := newTestDB(t)
	c.DB.Close()

	ctx := &Context{DB: c}
	_, err := ctx.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
	if !strings.Contains(err.Error(), "getting context memories") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestContext_Handle_ResponseURI(t *testing.T) {
	ctx := &Context{DB: newTestDB(t)}
	contents, err := ctx.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents)
	if text.URI != "memory://context" {
		t.Errorf("URI = %q, want memory://context", text.URI)
	}
	if text.MIMEType != "application/json" {
		t.Errorf("MIMEType = %q", text.MIMEType)
	}
}

// ---------- Patterns: tag filtering and DB error ----------

func TestPatterns_Handle_MultipleResults(t *testing.T) {
	c := newTestDB(t)
	seedMemory(t, c, "pattern-1", "proj", "memory", "internal", []string{"pattern"})
	seedMemory(t, c, "pattern-2", "proj", "memory", "internal", []string{"pattern"})

	p := &Patterns{DB: c}
	contents, err := p.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents).Text
	if !strings.Contains(text, "pattern-1") || !strings.Contains(text, "pattern-2") {
		t.Error("expected both patterns in response")
	}
}

func TestPatterns_Handle_FiltersByTag(t *testing.T) {
	c := newTestDB(t)
	seedMemory(t, c, "is-pattern", "proj", "memory", "internal", []string{"pattern"})
	seedMemory(t, c, "not-pattern", "proj", "memory", "internal", []string{"other"})

	p := &Patterns{DB: c}
	contents, err := p.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents).Text
	if !strings.Contains(text, "is-pattern") {
		t.Error("expected pattern-tagged memory")
	}
	if strings.Contains(text, "not-pattern") {
		t.Error("should NOT contain non-pattern-tagged memory")
	}
}

func TestPatterns_Handle_DBError(t *testing.T) {
	c := newTestDB(t)
	c.DB.Close()

	p := &Patterns{DB: c}
	_, err := p.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
	if !strings.Contains(err.Error(), "listing patterns") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------- RecentConversations: type/tag filtering and DB error ----------

func TestRecentConversations_Handle_MultipleResults(t *testing.T) {
	c := newTestDB(t)
	seedMemory(t, c, "conv-1", "proj", "conversation", "internal", []string{"conversation"})
	seedMemory(t, c, "conv-2", "proj", "conversation", "internal", []string{"conversation"})

	r := &RecentConversations{DB: c}
	contents, err := r.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents).Text
	if !strings.Contains(text, "conv-1") || !strings.Contains(text, "conv-2") {
		t.Error("expected both conversations in response")
	}
}

func TestRecentConversations_Handle_FiltersType(t *testing.T) {
	c := newTestDB(t)
	seedMemory(t, c, "is-conv", "proj", "conversation", "internal", []string{"conversation"})
	seedMemory(t, c, "not-conv", "proj", "memory", "internal", []string{"conversation"})

	r := &RecentConversations{DB: c}
	contents, err := r.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents).Text
	if !strings.Contains(text, "is-conv") {
		t.Error("expected conversation-type memory")
	}
}

func TestRecentConversations_Handle_DBError(t *testing.T) {
	c := newTestDB(t)
	c.DB.Close()

	r := &RecentConversations{DB: c}
	_, err := r.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
	if !strings.Contains(err.Error(), "listing recent conversations") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRecentConversations_Handle_ResponseURI(t *testing.T) {
	r := &RecentConversations{DB: newTestDB(t)}
	contents, err := r.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents)
	if text.URI != "memory://conversations/recent" {
		t.Errorf("URI = %q, want memory://conversations/recent", text.URI)
	}
	if text.MIMEType != "application/json" {
		t.Errorf("MIMEType = %q", text.MIMEType)
	}
}

// ---------- Resource/Template URI format verification ----------

func TestRecent_Template_Format(t *testing.T) {
	r := &Recent{DB: newTestDB(t)}
	tmpl := r.Template()
	raw := tmpl.URITemplate.Raw()
	if !strings.Contains(raw, "{project}") {
		t.Errorf("template URI %q missing {project} parameter", raw)
	}
	if !strings.HasPrefix(raw, "memory://") {
		t.Errorf("template URI %q missing memory:// prefix", raw)
	}
}

func TestDecisions_Template_Format(t *testing.T) {
	d := &Decisions{DB: newTestDB(t)}
	tmpl := d.Template()
	raw := tmpl.URITemplate.Raw()
	if !strings.Contains(raw, "{project}") {
		t.Errorf("template URI %q missing {project} parameter", raw)
	}
	if !strings.HasPrefix(raw, "memory://") {
		t.Errorf("template URI %q missing memory:// prefix", raw)
	}
}

func TestContext_Resource_Format(t *testing.T) {
	c := &Context{DB: newTestDB(t)}
	res := c.Resource()
	if !strings.HasPrefix(res.URI, "memory://") {
		t.Errorf("resource URI %q missing memory:// prefix", res.URI)
	}
}

func TestPreferences_Resource_Format(t *testing.T) {
	p := &Preferences{DB: newTestDB(t)}
	res := p.Resource()
	if !strings.HasPrefix(res.URI, "memory://") {
		t.Errorf("resource URI %q missing memory:// prefix", res.URI)
	}
}

func TestPatterns_Resource_Format(t *testing.T) {
	p := &Patterns{DB: newTestDB(t)}
	res := p.Resource()
	if !strings.HasPrefix(res.URI, "memory://") {
		t.Errorf("resource URI %q missing memory:// prefix", res.URI)
	}
}

func TestRecentConversations_Resource_Format(t *testing.T) {
	r := &RecentConversations{DB: newTestDB(t)}
	res := r.Resource()
	if !strings.HasPrefix(res.URI, "memory://") {
		t.Errorf("resource URI %q missing memory:// prefix", res.URI)
	}
}

// ---------- Recent: response URI matches request ----------

func TestRecent_Handle_ResponseURIMatchesRequest(t *testing.T) {
	r := &Recent{DB: newTestDB(t)}
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "memory://recent/special-project"

	contents, err := r.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents)
	if text.URI != "memory://recent/special-project" {
		t.Errorf("URI = %q, want memory://recent/special-project", text.URI)
	}
}

func TestPreferences_Handle_ResponseURI(t *testing.T) {
	p := &Preferences{DB: newTestDB(t)}
	contents, err := p.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents)
	if text.URI != "memory://preferences" {
		t.Errorf("URI = %q, want memory://preferences", text.URI)
	}
}

func TestPatterns_Handle_ResponseURI(t *testing.T) {
	p := &Patterns{DB: newTestDB(t)}
	contents, err := p.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents)
	if text.URI != "memory://patterns" {
		t.Errorf("URI = %q, want memory://patterns", text.URI)
	}
}

// ---------- Marshal error paths ----------

func TestRecent_Handle_MarshalError(t *testing.T) {
	withFailingMarshal(t)
	r := &Recent{DB: newTestDB(t)}
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "memory://recent/proj"

	_, err := r.Handle(context.Background(), req)
	if err == nil {
		t.Fatal("expected marshal error")
	}
	if !strings.Contains(err.Error(), "marshaling memories") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDecisions_Handle_MarshalError(t *testing.T) {
	withFailingMarshal(t)
	d := &Decisions{DB: newTestDB(t)}
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "memory://decisions/proj"

	_, err := d.Handle(context.Background(), req)
	if err == nil {
		t.Fatal("expected marshal error")
	}
	if !strings.Contains(err.Error(), "marshaling decisions") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPreferences_Handle_MarshalError(t *testing.T) {
	withFailingMarshal(t)
	p := &Preferences{DB: newTestDB(t)}

	_, err := p.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err == nil {
		t.Fatal("expected marshal error")
	}
	if !strings.Contains(err.Error(), "marshaling preferences") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestContext_Handle_MarshalError(t *testing.T) {
	withFailingMarshal(t)
	ctx := &Context{DB: newTestDB(t)}

	_, err := ctx.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err == nil {
		t.Fatal("expected marshal error")
	}
	if !strings.Contains(err.Error(), "marshaling context memories") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPatterns_Handle_MarshalError(t *testing.T) {
	withFailingMarshal(t)
	p := &Patterns{DB: newTestDB(t)}

	_, err := p.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err == nil {
		t.Fatal("expected marshal error")
	}
	if !strings.Contains(err.Error(), "marshaling patterns") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRecentConversations_Handle_MarshalError(t *testing.T) {
	withFailingMarshal(t)
	c := newTestDB(t)
	seedMemory(t, c, "conv", "proj", "conversation", "internal", []string{"conversation"})

	r := &RecentConversations{DB: c}
	_, err := r.Handle(context.Background(), mcp.ReadResourceRequest{})
	if err == nil {
		t.Fatal("expected marshal error")
	}
	if !strings.Contains(err.Error(), "marshaling conversations") {
		t.Errorf("unexpected error: %v", err)
	}
}
