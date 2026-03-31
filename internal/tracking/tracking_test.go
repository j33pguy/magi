package tracking

import (
	"context"
	"log/slog"
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

func newTestTracker(t *testing.T) *Tracker {
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

	return &Tracker{
		DB:       client.TursoClient,
		Embedder: &mockEmbedder{},
	}
}

func TestTrackTask(t *testing.T) {
	tr := newTestTracker(t)

	m, err := tr.TrackTask(context.Background(), "TASK-123", "in_progress", map[string]string{
		"assignee": "alice",
		"priority": "high",
	})
	if err != nil {
		t.Fatalf("TrackTask: %v", err)
	}

	if m.ID == "" {
		t.Error("expected non-empty ID")
	}
	if m.Type != "task" {
		t.Errorf("type = %q, want %q", m.Type, "task")
	}
	if !strings.Contains(m.Content, "TASK-123") {
		t.Errorf("content missing task ID: %s", m.Content)
	}
	if !strings.Contains(m.Content, "in_progress") {
		t.Errorf("content missing state: %s", m.Content)
	}

	tags, _ := tr.DB.GetTags(m.ID)
	if len(tags) == 0 {
		t.Error("expected tags to be set")
	}
}

func TestTrackDecision(t *testing.T) {
	tr := newTestTracker(t)

	m, err := tr.TrackDecision(context.Background(), "Use PostgreSQL for production", "Better scalability and monitoring support")
	if err != nil {
		t.Fatalf("TrackDecision: %v", err)
	}

	if m.ID == "" {
		t.Error("expected non-empty ID")
	}
	if m.Type != "decision" {
		t.Errorf("type = %q, want %q", m.Type, "decision")
	}
	if m.Summary != "Use PostgreSQL for production" {
		t.Errorf("summary = %q", m.Summary)
	}
	if !strings.Contains(m.Content, "Better scalability") {
		t.Errorf("content missing context: %s", m.Content)
	}

	tags, _ := tr.DB.GetTags(m.ID)
	found := false
	for _, tag := range tags {
		if tag == "decision" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'decision' tag")
	}
}

func TestTrackConversation(t *testing.T) {
	tr := newTestTracker(t)

	m, err := tr.TrackConversation(
		context.Background(),
		"Discussed migration strategy",
		[]string{"database", "migration"},
		[]string{"Use blue-green deployment"},
		[]string{"Write migration script", "Update runbook"},
	)
	if err != nil {
		t.Fatalf("TrackConversation: %v", err)
	}

	if m.ID == "" {
		t.Error("expected non-empty ID")
	}
	if m.Type != "conversation" {
		t.Errorf("type = %q, want %q", m.Type, "conversation")
	}
	if !strings.Contains(m.Content, "migration strategy") {
		t.Error("content missing summary")
	}
	if !strings.Contains(m.Content, "blue-green deployment") {
		t.Error("content missing decisions")
	}
	if !strings.Contains(m.Content, "Write migration script") {
		t.Error("content missing action items")
	}

	tags, _ := tr.DB.GetTags(m.ID)
	if len(tags) < 3 {
		t.Errorf("expected at least 3 tags, got %d", len(tags))
	}
}

func TestTrackConversationMinimal(t *testing.T) {
	tr := newTestTracker(t)

	m, err := tr.TrackConversation(
		context.Background(),
		"Quick sync",
		nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("TrackConversation: %v", err)
	}

	if m.ID == "" {
		t.Error("expected non-empty ID")
	}
	if !strings.Contains(m.Content, "Quick sync") {
		t.Error("content missing summary")
	}
}
