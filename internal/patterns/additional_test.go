package patterns

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
)

// --- Mock embedder ---

type mockEmbedder struct {
	dim       int
	failAfter int // fail after N calls; 0 means never fail
	calls     int
}

func (m *mockEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	m.calls++
	if m.failAfter > 0 && m.calls > m.failAfter {
		return nil, fmt.Errorf("mock embed error")
	}
	vec := make([]float32, m.dim)
	for i := range vec {
		vec[i] = 0.01 * float32(i%10)
	}
	return vec, nil
}

func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		v, err := m.Embed(context.Background(), texts[i])
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func (m *mockEmbedder) Dimensions() int { return m.dim }

var _ embeddings.Provider = (*mockEmbedder)(nil)

// --- Helper: create in-memory DB client ---

func newTestDB(t *testing.T) *db.Client {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	sc, err := db.NewSQLiteClient(dbPath, logger)
	if err != nil {
		t.Fatalf("new sqlite client: %v", err)
	}
	// SQLiteClient embeds *TursoClient (= *Client)
	client := sc.TursoClient
	if err := client.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { client.DB.Close() })
	return client
}

// ============================================================
// StorePatterns tests
// ============================================================

func TestStorePatterns_Basic(t *testing.T) {
	client := newTestDB(t)
	embedder := &mockEmbedder{dim: 384}

	patterns := []Pattern{
		{
			Type:        PatternPreference,
			Description: "Prefers Go for backend services",
			Confidence:  0.8,
			Evidence:    []string{"mem-1", "mem-2"},
			Area:        "meta",
		},
	}

	stored, skipped, err := StorePatterns(context.Background(), client, embedder, patterns)
	if err != nil {
		t.Fatalf("StorePatterns error: %v", err)
	}
	if len(stored) != 1 {
		t.Errorf("expected 1 stored, got %d", len(stored))
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}

	// Verify the memory was actually saved
	mem, err := client.GetMemory(stored[0])
	if err != nil {
		t.Fatalf("GetMemory error: %v", err)
	}
	if mem.Speaker != "system" {
		t.Errorf("Speaker = %q, want %q", mem.Speaker, "system")
	}
	if mem.Source != "pattern-analyzer" {
		t.Errorf("Source = %q, want %q", mem.Source, "pattern-analyzer")
	}
	if mem.Visibility != "internal" {
		t.Errorf("Visibility = %q, want %q", mem.Visibility, "internal")
	}
	if !strings.Contains(mem.Content, "Evidence memory IDs:") {
		t.Errorf("Content should contain evidence IDs, got %q", mem.Content)
	}

	// Verify tags
	tags, err := client.GetTags(stored[0])
	if err != nil {
		t.Fatalf("GetTags error: %v", err)
	}
	tagSet := make(map[string]bool)
	for _, tag := range tags {
		tagSet[tag] = true
	}
	for _, expected := range []string{"pattern", "auto-detected", "pattern_type:preference", "speaker:system", "area:meta"} {
		if !tagSet[expected] {
			t.Errorf("missing expected tag %q in %v", expected, tags)
		}
	}
}

func TestStorePatterns_EmptyEvidence(t *testing.T) {
	client := newTestDB(t)
	embedder := &mockEmbedder{dim: 384}

	patterns := []Pattern{
		{
			Type:        PatternCommsStyle,
			Description: "Direct communication style",
			Confidence:  0.7,
			Evidence:    nil, // no evidence
			Area:        "",  // no area
		},
	}

	stored, _, err := StorePatterns(context.Background(), client, embedder, patterns)
	if err != nil {
		t.Fatalf("StorePatterns error: %v", err)
	}
	if len(stored) != 1 {
		t.Errorf("expected 1 stored, got %d", len(stored))
	}

	mem, err := client.GetMemory(stored[0])
	if err != nil {
		t.Fatalf("GetMemory error: %v", err)
	}
	if strings.Contains(mem.Content, "Evidence") {
		t.Errorf("Content should NOT contain evidence section, got %q", mem.Content)
	}
}

func TestStorePatterns_Dedup(t *testing.T) {
	client := newTestDB(t)
	embedder := &mockEmbedder{dim: 384}

	pattern := Pattern{
		Type:        PatternPreference,
		Description: "Prefers Go for backend services",
		Confidence:  0.8,
		Evidence:    []string{"mem-1"},
		Area:        "meta",
	}

	// Store once
	stored1, _, err := StorePatterns(context.Background(), client, embedder, []Pattern{pattern})
	if err != nil {
		t.Fatalf("first StorePatterns error: %v", err)
	}
	if len(stored1) != 1 {
		t.Fatalf("expected 1 stored, got %d", len(stored1))
	}

	// Store again — should be deduped (same embedding, same tag)
	stored2, skipped2, err := StorePatterns(context.Background(), client, embedder, []Pattern{pattern})
	if err != nil {
		t.Fatalf("second StorePatterns error: %v", err)
	}
	// The mock always returns the same vector, so similarity = 1.0 > 0.9 => skipped
	if skipped2 != 1 {
		t.Errorf("expected 1 skipped on dedup, got %d (stored %d)", skipped2, len(stored2))
	}
}

func TestStorePatterns_EmbedError(t *testing.T) {
	client := newTestDB(t)
	alwaysFail := &alwaysFailEmbedder{}

	patterns := []Pattern{
		{Type: PatternPreference, Description: "test", Confidence: 0.5},
	}
	_, _, err := StorePatterns(context.Background(), client, alwaysFail, patterns)
	if err == nil {
		t.Error("expected error when embedder fails")
	}
	if !strings.Contains(err.Error(), "embedding pattern") {
		t.Errorf("expected embedding error, got: %v", err)
	}
}

type alwaysFailEmbedder struct{}

func (a *alwaysFailEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, fmt.Errorf("embed failure")
}
func (a *alwaysFailEmbedder) EmbedBatch(_ context.Context, _ []string) ([][]float32, error) {
	return nil, fmt.Errorf("embed failure")
}
func (a *alwaysFailEmbedder) Dimensions() int { return 384 }

var _ embeddings.Provider = (*alwaysFailEmbedder)(nil)

func TestStorePatterns_MultiplePatterns(t *testing.T) {
	client := newTestDB(t)
	// Use an embedder that returns different vectors for each call to avoid dedup
	embedder := &varyingEmbedder{dim: 384}

	patterns := []Pattern{
		{Type: PatternPreference, Description: "Likes Go", Confidence: 0.8, Area: "meta"},
		{Type: PatternWorkPattern, Description: "Works late at night", Confidence: 0.6, Area: "work"},
		{Type: PatternCommsStyle, Description: "Concise style", Confidence: 0.7, Area: "meta"},
	}

	stored, skipped, err := StorePatterns(context.Background(), client, embedder, patterns)
	if err != nil {
		t.Fatalf("StorePatterns error: %v", err)
	}
	if len(stored) != 3 {
		t.Errorf("expected 3 stored, got %d (skipped %d)", len(stored), skipped)
	}
}

type varyingEmbedder struct {
	dim   int
	calls int
}

func (v *varyingEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	v.calls++
	vec := make([]float32, v.dim)
	// Different vector each call to avoid dedup
	for i := range vec {
		vec[i] = float32(v.calls) * 0.01 * float32(i%10+1)
	}
	return vec, nil
}
func (v *varyingEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		vec, err := v.Embed(context.Background(), texts[i])
		if err != nil {
			return nil, err
		}
		out[i] = vec
	}
	return out, nil
}
func (v *varyingEmbedder) Dimensions() int { return v.dim }

var _ embeddings.Provider = (*varyingEmbedder)(nil)

func TestStorePatterns_EmptySlice(t *testing.T) {
	client := newTestDB(t)
	embedder := &mockEmbedder{dim: 384}

	stored, skipped, err := StorePatterns(context.Background(), client, embedder, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stored) != 0 || skipped != 0 {
		t.Errorf("expected 0/0, got stored=%d skipped=%d", len(stored), skipped)
	}
}

// ============================================================
// truncateDesc tests
// ============================================================

func TestTruncateDesc(t *testing.T) {
	tests := []struct {
		input string
		n     int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a longer string", 10, "this is a ..."},
		{"", 5, ""},
	}
	for _, tt := range tests {
		got := truncateDesc(tt.input, tt.n)
		if got != tt.want {
			t.Errorf("truncateDesc(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
		}
	}
}

// ============================================================
// detectDecisionPatterns — comparative & decisive branches
// ============================================================

func TestAnalyze_DecisionPattern_Comparative(t *testing.T) {
	memories := make([]*db.Memory, 0, 10)

	// At least 3 comparative discussions
	comparativeContents := []string{
		"Weighing the tradeoff between latency and throughput",
		"Comparing vs. the old architecture for pros and cons",
		"Evaluating the tradeoffs between cost and reliability",
		"Some other technical discussion",
		"Another discussion about implementation",
	}
	for i, c := range comparativeContents {
		memories = append(memories, &db.Memory{
			ID:      fmt.Sprintf("comp-%d", i),
			Content: c,
			Speaker: "user",
		})
	}

	a := &Analyzer{}
	patterns := a.Analyze(memories)

	found := false
	for _, p := range patterns {
		if p.Type == PatternDecisionStyle && strings.Contains(p.Description, "tradeoffs") {
			found = true
		}
	}
	if !found {
		t.Error("expected comparative decision pattern, not found")
	}
}

func TestAnalyze_DecisionPattern_Decisive(t *testing.T) {
	var memories []*db.Memory

	// Need decisiveCount >= 5 and ratio > 2.0 (decisions vs questions)
	decisiveContents := []string{
		"Decided to refactor the auth module",
		"Going with the microservices approach",
		"Settled on PostgreSQL for the main DB",
		"Chose gRPC over REST for internal services",
		"Going with Terraform for infra",
		"Picked the monorepo layout",
	}
	for i, c := range decisiveContents {
		memories = append(memories, &db.Memory{
			ID:      fmt.Sprintf("dec-%d", i),
			Content: c,
			Speaker: "user",
		})
	}

	// Add only 1 question (ratio = 6/1 = 6.0 > 2.0)
	memories = append(memories, &db.Memory{
		ID:      "q-1",
		Content: "Should we add caching?",
		Speaker: "user",
	})

	a := &Analyzer{}
	patterns := a.Analyze(memories)

	found := false
	for _, p := range patterns {
		if p.Type == PatternDecisionStyle && strings.Contains(p.Description, "Decisive") {
			found = true
		}
	}
	if !found {
		t.Error("expected decisive communicator pattern, not found")
	}
}

func TestAnalyze_DecisionPattern_TooManyQuestions(t *testing.T) {
	var memories []*db.Memory

	// 5 decisions but also 5 questions (ratio = 1.0 < 2.0)
	for i := 0; i < 5; i++ {
		memories = append(memories, &db.Memory{
			ID:      fmt.Sprintf("dec-%d", i),
			Content: fmt.Sprintf("Decided on approach %d for the service", i),
			Speaker: "user",
		})
		memories = append(memories, &db.Memory{
			ID:      fmt.Sprintf("q-%d", i),
			Content: fmt.Sprintf("Should we do thing %d?", i),
			Speaker: "user",
		})
	}

	a := &Analyzer{}
	patterns := a.Analyze(memories)

	for _, p := range patterns {
		if p.Type == PatternDecisionStyle && strings.Contains(p.Description, "Decisive") {
			t.Error("should NOT detect decisive pattern when question ratio is low")
		}
	}
}

// ============================================================
// detectCommsStyle — detailed communicator and directness
// ============================================================

func TestAnalyze_CommsStyle_Detailed(t *testing.T) {
	longContent := strings.Repeat("This is a detailed explanation of the architecture including many nuances and edge cases. ", 10)

	var memories []*db.Memory
	for i := 0; i < 10; i++ {
		memories = append(memories, &db.Memory{
			ID:      fmt.Sprintf("m-%d", i),
			Content: longContent,
			Speaker: "user",
		})
	}

	a := &Analyzer{}
	patterns := a.Analyze(memories)

	found := false
	for _, p := range patterns {
		if p.Type == PatternCommsStyle && strings.Contains(p.Description, "Detailed") {
			found = true
		}
	}
	if !found {
		t.Error("expected detailed communicator pattern, not found")
	}
}

func TestAnalyze_CommsStyle_Direct(t *testing.T) {
	// Need 10+ memories with < 15% containing questions
	var memories []*db.Memory
	for i := 0; i < 20; i++ {
		content := fmt.Sprintf("Statement about task %d without any interrogatives", i)
		memories = append(memories, &db.Memory{
			ID:      fmt.Sprintf("m-%d", i),
			Content: content,
			Speaker: "user",
		})
	}

	a := &Analyzer{}
	patterns := a.Analyze(memories)

	found := false
	for _, p := range patterns {
		if p.Type == PatternCommsStyle && strings.Contains(p.Description, "Direct") {
			found = true
		}
	}
	if !found {
		t.Error("expected direct communication style pattern, not found")
	}
}

func TestAnalyze_CommsStyle_TooFewMemories(t *testing.T) {
	memories := []*db.Memory{
		{ID: "1", Content: "hi", Speaker: "user"},
		{ID: "2", Content: "ok", Speaker: "user"},
	}

	a := &Analyzer{}
	patterns := a.Analyze(memories)

	for _, p := range patterns {
		if p.Type == PatternCommsStyle {
			t.Error("should not detect comms style with fewer than 5 memories")
		}
	}
}

// ============================================================
// detectWorkTimingPatterns — peak hours
// ============================================================

func TestAnalyze_WorkTiming_PeakHour(t *testing.T) {
	var memories []*db.Memory
	baseTime := time.Date(2025, 3, 10, 14, 0, 0, 0, time.UTC) // Monday 14:00

	// Create 10+ memories, 6 at hour 14 to make it the clear peak
	for i := 0; i < 6; i++ {
		memories = append(memories, &db.Memory{
			ID:        fmt.Sprintf("peak-%d", i),
			Content:   fmt.Sprintf("Working on task %d", i),
			Speaker:   "user",
			Area:      "work",
			CreatedAt: baseTime.Add(time.Duration(i) * time.Minute).Format(time.DateTime),
		})
	}

	// A few at other hours to reach totalHours >= 10
	for i := 0; i < 5; i++ {
		t2 := baseTime.Add(time.Duration(i+1) * time.Hour)
		memories = append(memories, &db.Memory{
			ID:        fmt.Sprintf("other-%d", i),
			Content:   fmt.Sprintf("Other task %d", i),
			Speaker:   "user",
			Area:      "work",
			CreatedAt: t2.Format(time.DateTime),
		})
	}

	a := &Analyzer{}
	patterns := a.Analyze(memories)

	found := false
	for _, p := range patterns {
		if p.Type == PatternWorkPattern && strings.Contains(p.Description, "Peak activity") && strings.Contains(p.Description, "14:00") {
			found = true
		}
	}
	if !found {
		t.Error("expected peak hour pattern at 14:00, not found")
	}
}

func TestAnalyze_WorkTiming_BadTimestamp(t *testing.T) {
	memories := []*db.Memory{
		{ID: "1", Content: "task 1", Speaker: "user", Area: "work", CreatedAt: "not-a-time"},
		{ID: "2", Content: "task 2", Speaker: "user", Area: "work", CreatedAt: "invalid"},
	}

	a := &Analyzer{}
	patterns := a.Analyze(memories)

	for _, p := range patterns {
		if p.Type == PatternWorkPattern {
			t.Error("should not detect work timing patterns with invalid timestamps")
		}
	}
}

// ============================================================
// parseTime edge cases
// ============================================================

func TestParseTime_RFC3339(t *testing.T) {
	ts := "2025-03-15T14:30:00Z"
	got, err := parseTime(ts)
	if err != nil {
		t.Fatalf("parseTime(%q) error: %v", ts, err)
	}
	if got.Hour() != 14 {
		t.Errorf("expected hour 14, got %d", got.Hour())
	}
}

func TestParseTime_Invalid(t *testing.T) {
	_, err := parseTime("not-a-time")
	if err == nil {
		t.Error("expected error for invalid time string")
	}
}

// ============================================================
// sumValues edge case
// ============================================================

func TestSumValues_Empty(t *testing.T) {
	got := sumValues(nil)
	if got != 0 {
		t.Errorf("sumValues(nil) = %d, want 0", got)
	}
}
