package contradiction

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
)

// Compile-time check that mockEmbedder satisfies the interface.
var _ embeddings.Provider = (*mockEmbedder)(nil)

// unitVec returns a 384-dim vector with the first element set to 1.0.
// This avoids NULL cosine distance that occurs with all-zero vectors.
func unitVec() []float32 {
	v := make([]float32, 384)
	v[0] = 1.0
	return v
}

type mockEmbedder struct {
	dims int
	vec  []float32 // optional override; if nil, uses unitVec()
}

func (m *mockEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	if m.vec != nil {
		return m.vec, nil
	}
	return unitVec(), nil
}

func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		if m.vec != nil {
			result[i] = m.vec
		} else {
			result[i] = unitVec()
		}
	}
	return result, nil
}

func (m *mockEmbedder) Dimensions() int { return m.dims }

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

func saveTestMemory(t *testing.T, c *db.Client, content, area, subArea string, emb []float32) *db.Memory {
	t.Helper()
	m, err := c.SaveMemory(&db.Memory{
		Content:   content,
		Embedding: emb,
		Area:      area,
		SubArea:   subArea,
		Type:      "fact",
		Speaker:   "user",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	return m
}

// ---------------------------------------------------------------------------
// Check method tests
// ---------------------------------------------------------------------------

func TestCheck_EmptyDB(t *testing.T) {
	client := newTestDB(t)
	emb := &mockEmbedder{dims: 384}
	d := &Detector{}

	candidates, err := d.Check(context.Background(), client, emb, "the firewall is enabled", "infrastructure", "compute")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates on empty DB, got %d", len(candidates))
	}
}

func TestCheck_ContradictingMemory_BooleanFlip(t *testing.T) {
	client := newTestDB(t)
	emb := &mockEmbedder{dims: 384}

	// unitVec produces identical embeddings so cosine distance = 0 (passes any threshold).
	vec := unitVec()
	saveTestMemory(t, client, "the firewall is enabled on compute", "infrastructure", "compute", vec)

	d := &Detector{}
	candidates, err := d.Check(context.Background(), client, emb, "the firewall is disabled on compute", "infrastructure", "compute")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected at least 1 contradiction candidate, got 0")
	}
	c := candidates[0]
	if c.Score < 0.6 {
		t.Errorf("expected score >= 0.6 for boolean flip, got %.2f", c.Score)
	}
	if c.Similarity < 0.85 {
		t.Errorf("expected similarity >= 0.85, got %.2f", c.Similarity)
	}
	if c.Reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestCheck_AreaFiltering(t *testing.T) {
	client := newTestDB(t)
	emb := &mockEmbedder{dims: 384}
	vec := unitVec()

	// Save memory in area "work"
	saveTestMemory(t, client, "the service is enabled", "work", "infra", vec)

	// Search in area "infrastructure" — should not find the work memory
	d := &Detector{}
	candidates, err := d.Check(context.Background(), client, emb, "the service is disabled", "infrastructure", "infra")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates when area differs, got %d", len(candidates))
	}
}

func TestCheck_SubAreaFiltering(t *testing.T) {
	client := newTestDB(t)
	emb := &mockEmbedder{dims: 384}
	vec := unitVec()

	saveTestMemory(t, client, "port 8080 is used for the API", "infrastructure", "compute", vec)

	// Same area but different subArea — should not match
	d := &Detector{}
	candidates, err := d.Check(context.Background(), client, emb, "port 9090 is used for the API", "infrastructure", "networking")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates when subArea differs, got %d", len(candidates))
	}
}

func TestCheck_CustomThreshold(t *testing.T) {
	client := newTestDB(t)
	emb := &mockEmbedder{dims: 384}
	vec := unitVec()

	saveTestMemory(t, client, "the service is enabled", "infrastructure", "", vec)

	// Threshold 0.99 means maxDistance = 0.01; zero-vector distance = 0 so it passes.
	d := &Detector{Threshold: 0.99}
	candidates, err := d.Check(context.Background(), client, emb, "the service is disabled", "infrastructure", "")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected at least 1 candidate with threshold 0.99")
	}
}

func TestCheck_SimilarNonContradicting(t *testing.T) {
	client := newTestDB(t)
	emb := &mockEmbedder{dims: 384}
	vec := unitVec()

	// Two statements that are similar but don't contradict
	saveTestMemory(t, client, "compute runs on the infrastructure server", "infrastructure", "compute", vec)

	d := &Detector{}
	candidates, err := d.Check(context.Background(), client, emb, "compute runs on the infrastructure node", "infrastructure", "compute")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	// scoreContradiction should yield <=0.5 so no candidates are returned
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates for non-contradicting content, got %d (score=%.2f)", len(candidates), candidates[0].Score)
	}
}

func TestCheck_NoAreaFilter(t *testing.T) {
	client := newTestDB(t)
	emb := &mockEmbedder{dims: 384}
	vec := unitVec()

	saveTestMemory(t, client, "the firewall is enabled", "infrastructure", "compute", vec)

	// Empty area/subArea should match any memory
	d := &Detector{}
	candidates, err := d.Check(context.Background(), client, emb, "the firewall is disabled", "", "")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected at least 1 candidate when no area filter")
	}
}

func TestCheck_UsesMemorySummaryWhenAvailable(t *testing.T) {
	client := newTestDB(t)
	emb := &mockEmbedder{dims: 384}
	vec := unitVec()

	m, err := client.SaveMemory(&db.Memory{
		Content:   "the firewall is enabled on all nodes",
		Summary:   "firewall enabled",
		Embedding: vec,
		Area:      "infrastructure",
		Type:      "fact",
		Speaker:   "user",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	_ = m

	d := &Detector{}
	candidates, err := d.Check(context.Background(), client, emb, "the firewall is disabled on all nodes", "infrastructure", "")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected at least 1 candidate")
	}
	if candidates[0].ExistingSummary != "firewall enabled" {
		t.Errorf("expected summary 'firewall enabled', got %q", candidates[0].ExistingSummary)
	}
}

func TestCheck_TruncatesNewContent(t *testing.T) {
	client := newTestDB(t)
	emb := &mockEmbedder{dims: 384}
	vec := unitVec()

	saveTestMemory(t, client, "the firewall is enabled and configured with rules for all services across every node in the cluster to ensure maximum security and compliance with corporate policy standards", "infrastructure", "", vec)

	longContent := "the firewall is disabled and configured with rules for all services across every node in the cluster to ensure maximum security and compliance with corporate policy standards"

	d := &Detector{}
	candidates, err := d.Check(context.Background(), client, emb, longContent, "infrastructure", "")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected at least 1 candidate")
	}
	if len(candidates[0].NewContent) > 120 {
		t.Errorf("NewContent should be truncated to 120 chars, got %d", len(candidates[0].NewContent))
	}
}

func TestCheck_MultipleMemories(t *testing.T) {
	client := newTestDB(t)
	emb := &mockEmbedder{dims: 384}
	vec := unitVec()

	saveTestMemory(t, client, "the firewall is enabled", "infrastructure", "", vec)
	saveTestMemory(t, client, "auto-deploy is true", "infrastructure", "", vec)
	// Non-contradicting memory
	saveTestMemory(t, client, "compute runs well", "infrastructure", "", vec)

	d := &Detector{}
	candidates, err := d.Check(context.Background(), client, emb, "the firewall is disabled and auto-deploy is false", "infrastructure", "")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	// Should find at least 1 candidate (the boolean flips)
	if len(candidates) == 0 {
		t.Fatal("expected at least 1 contradiction candidate")
	}
}

// ---------------------------------------------------------------------------
// numericChangeScore edge cases
// ---------------------------------------------------------------------------

func TestNumericChangeScore_NoKeywords(t *testing.T) {
	// No keyword-number pairs at all
	score := numericChangeScore("hello world", "goodbye world")
	if score != 0 {
		t.Errorf("expected 0, got %v", score)
	}
}

func TestNumericChangeScore_SameNumbers(t *testing.T) {
	score := numericChangeScore("vlan 100 and port 443", "vlan 100 and port 443")
	if score != 0 {
		t.Errorf("expected 0 for identical numbers, got %v", score)
	}
}

func TestNumericChangeScore_DifferentKeywords(t *testing.T) {
	// Keywords don't overlap, so no match
	score := numericChangeScore("vlan 100", "port 443")
	if score != 0 {
		t.Errorf("expected 0 for different keywords, got %v", score)
	}
}

func TestNumericChangeScore_MultipleNumbersSameKeyword(t *testing.T) {
	// Same keyword with multiple numbers where at least one differs
	score := numericChangeScore("port 80 and port 443", "port 80 and port 8080")
	if score < 0.7 {
		t.Errorf("expected >= 0.7 for differing numbers on same keyword, got %v", score)
	}
}

func TestNumericChangeScore_DecimalVersionChange(t *testing.T) {
	score := numericChangeScore("version 1.0", "version 2.5")
	if score < 0.7 {
		t.Errorf("expected >= 0.7 for decimal version change, got %v", score)
	}
}

func TestNumericChangeScore_OnlyOneTextHasNumbers(t *testing.T) {
	score := numericChangeScore("vlan 100", "some text without numbers")
	if score != 0 {
		t.Errorf("expected 0 when only one text has numbers, got %v", score)
	}
}

// ---------------------------------------------------------------------------
// booleanFlipScore edge cases
// ---------------------------------------------------------------------------

func TestBooleanFlipScore_SameStateBothSides(t *testing.T) {
	tests := []struct {
		name     string
		newText  string
		existing string
	}{
		{"both enabled", "the firewall is enabled", "the firewall is enabled"},
		{"both disabled", "the firewall is disabled", "the firewall is disabled"},
		{"both true", "flag is true", "flag is true"},
		{"both false", "flag is false", "flag is false"},
		{"both active", "the service is active", "the service is active"},
		{"both inactive", "the service is inactive", "the service is inactive"},
		{"both on", "feature is on", "feature is on"},
		{"both off", "feature is off", "feature is off"},
		{"both open", "the port is open", "the port is open"},
		{"both closed", "the port is closed", "the port is closed"},
		{"both up", "the server is up", "the server is up"},
		{"both down", "the server is down", "the server is down"},
		{"both allow", "the rule is allow", "the rule is allow"},
		{"both deny", "the rule is deny", "the rule is deny"},
		{"both block", "the rule is block", "the rule is block"},
		{"both yes", "the answer is yes", "the answer is yes"},
		{"both no", "the answer is no", "the answer is no"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := booleanFlipScore(tt.newText, tt.existing)
			if score != 0 {
				t.Errorf("expected 0 for same state both sides, got %v", score)
			}
		})
	}
}

func TestBooleanFlipScore_BothStatesPresent(t *testing.T) {
	// If both texts contain BOTH members of a pair, no flip is detected
	// because the condition requires !existHasA or !existHasB
	score := booleanFlipScore("enabled and disabled", "enabled and disabled")
	if score != 0 {
		t.Errorf("expected 0 when both states present in both texts, got %v", score)
	}
}

func TestBooleanFlipScore_AllPairFlips(t *testing.T) {
	tests := []struct {
		name     string
		newText  string
		existing string
	}{
		{"enabled->disabled", "it is disabled", "it is enabled"},
		{"disabled->enabled", "it is enabled", "it is disabled"},
		{"true->false", "value is false", "value is true"},
		{"false->true", "value is true", "value is false"},
		{"active->inactive", "service is inactive", "service is active"},
		{"inactive->active", "service is active", "service is inactive"},
		{"on->off", "feature is off", "feature is on"},
		{"off->on", "feature is on", "feature is off"},
		{"open->closed", "port is closed", "port is open"},
		{"closed->open", "port is open", "port is closed"},
		{"up->down", "server is down", "server is up"},
		{"down->up", "server is up", "server is down"},
		{"allow->deny", "rule is deny", "rule is allow"},
		{"deny->allow", "rule is allow", "rule is deny"},
		{"allow->block", "rule is block", "rule is allow"},
		{"block->allow", "rule is allow", "rule is block"},
		{"yes->no", "answer is no", "answer is yes"},
		{"no->yes", "answer is yes", "answer is no"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := booleanFlipScore(tt.newText, tt.existing)
			if score < 0.6 {
				t.Errorf("expected >= 0.6, got %v", score)
			}
		})
	}
}

func TestBooleanFlipScore_NoRelevantWords(t *testing.T) {
	score := booleanFlipScore("the sky is blue", "the sky is green")
	if score != 0 {
		t.Errorf("expected 0 for no boolean words, got %v", score)
	}
}

// ---------------------------------------------------------------------------
// replacementScore edge cases
// ---------------------------------------------------------------------------

func TestReplacementScore_SwitchedTo(t *testing.T) {
	score := replacementScore("switched to nginx from apache")
	if score < 0.4 {
		t.Errorf("expected >= 0.4 for 'switched to', got %v", score)
	}
}

func TestReplacementScore_SwitchedFrom(t *testing.T) {
	score := replacementScore("switched from apache to nginx")
	if score < 0.4 {
		t.Errorf("expected >= 0.4 for 'switched from', got %v", score)
	}
}

func TestReplacementScore_ReplacedBy(t *testing.T) {
	score := replacementScore("mysql replaced by postgres")
	if score < 0.4 {
		t.Errorf("expected >= 0.4 for 'replaced by', got %v", score)
	}
}

func TestReplacementScore_ReplacedWith(t *testing.T) {
	score := replacementScore("replaced with memcached")
	if score < 0.4 {
		t.Errorf("expected >= 0.4 for 'replaced with', got %v", score)
	}
}

func TestReplacementScore_MigratedTo(t *testing.T) {
	score := replacementScore("migrated to kubernetes")
	if score < 0.4 {
		t.Errorf("expected >= 0.4 for 'migrated to', got %v", score)
	}
}

func TestReplacementScore_MovedFrom(t *testing.T) {
	score := replacementScore("moved from aws to gcp")
	if score < 0.4 {
		t.Errorf("expected >= 0.4 for 'moved from', got %v", score)
	}
}

func TestReplacementScore_NowUses(t *testing.T) {
	score := replacementScore("now uses terraform")
	if score < 0.4 {
		t.Errorf("expected >= 0.4 for 'now uses', got %v", score)
	}
}

func TestReplacementScore_NowIs(t *testing.T) {
	score := replacementScore("the server now is running ubuntu")
	if score < 0.4 {
		t.Errorf("expected >= 0.4 for 'now is', got %v", score)
	}
}

func TestReplacementScore_NowRuns(t *testing.T) {
	score := replacementScore("the cluster now runs on k3s")
	if score < 0.4 {
		t.Errorf("expected >= 0.4 for 'now runs', got %v", score)
	}
}

func TestReplacementScore_NowOn(t *testing.T) {
	score := replacementScore("the service is now on port 9090")
	if score < 0.4 {
		t.Errorf("expected >= 0.4 for 'now on', got %v", score)
	}
}

func TestReplacementScore_NowAt(t *testing.T) {
	score := replacementScore("the server is now at 192.168.1.50")
	if score < 0.4 {
		t.Errorf("expected >= 0.4 for 'now at', got %v", score)
	}
}

func TestReplacementScore_UpdatedTo(t *testing.T) {
	score := replacementScore("updated to version 3.0")
	if score < 0.4 {
		t.Errorf("expected >= 0.4 for 'updated to', got %v", score)
	}
}

func TestReplacementScore_InsteadOf(t *testing.T) {
	score := replacementScore("using podman instead of docker")
	if score < 0.4 {
		t.Errorf("expected >= 0.4 for 'instead of', got %v", score)
	}
}

func TestReplacementScore_NoMatch(t *testing.T) {
	score := replacementScore("the server runs fine and is healthy")
	if score != 0 {
		t.Errorf("expected 0 for no replacement language, got %v", score)
	}
}

// ---------------------------------------------------------------------------
// scoreContradiction cap at 1.0
// ---------------------------------------------------------------------------

func TestScoreContradiction_CappedAtOne(t *testing.T) {
	// Trigger all three heuristics: numeric change (0.7) + boolean flip (0.6) + replacement (0.4) = 1.7 -> capped at 1.0
	score, reason := scoreContradiction(
		"changed to port 9090, service is now disabled",
		"running on port 8080, service is enabled",
	)
	if score > 1.0 {
		t.Errorf("score should be capped at 1.0, got %v", score)
	}
	if score < 1.0 {
		t.Logf("score = %.2f, reason = %s (may not trigger all 3 heuristics)", score, reason)
	}
}

func TestScoreContradiction_HighSimilarityOnlyReason(t *testing.T) {
	// No heuristics triggered → reason should be "high similarity only"
	score, reason := scoreContradiction("the sky is blue today", "the sky is blue today")
	if score != 0 {
		t.Errorf("expected score 0 for identical text, got %v", score)
	}
	if reason != "high similarity only" {
		t.Errorf("expected reason 'high similarity only', got %q", reason)
	}
}
