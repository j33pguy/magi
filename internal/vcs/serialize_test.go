package vcs

import (
	"encoding/json"
	"testing"

	"github.com/j33pguy/magi/internal/db"
)

func TestMemoryRoundTrip(t *testing.T) {
	original := &db.Memory{
		ID:         "abc123",
		Content:    "test memory content",
		Summary:    "a test",
		Project:    "test-project",
		Type:       "memory",
		Visibility: "internal",
		Source:     "grpc",
		Speaker:    "alice",
		Area:       "infrastructure",
		SubArea:    "magi",
		CreatedAt:  "2025-01-01 00:00:00",
		UpdatedAt:  "2025-01-01 00:00:00",
		TokenCount: 5,
		Tags:       []string{"test", "speaker:alice"},
		Embedding:  []float32{0.1, 0.2, 0.3}, // should be excluded from JSON
	}

	data, err := MemoryToJSON(original)
	if err != nil {
		t.Fatalf("MemoryToJSON: %v", err)
	}

	// Verify embedding is not in JSON
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if _, ok := raw["embedding"]; ok {
		t.Error("JSON should not contain embedding field")
	}

	// Round-trip
	restored, err := JSONToMemory(data)
	if err != nil {
		t.Fatalf("JSONToMemory: %v", err)
	}

	if restored.ID != original.ID {
		t.Errorf("ID = %q, want %q", restored.ID, original.ID)
	}
	if restored.Content != original.Content {
		t.Errorf("Content = %q, want %q", restored.Content, original.Content)
	}
	if restored.Speaker != original.Speaker {
		t.Errorf("Speaker = %q, want %q", restored.Speaker, original.Speaker)
	}
	if restored.Area != original.Area {
		t.Errorf("Area = %q, want %q", restored.Area, original.Area)
	}
	if len(restored.Tags) != len(original.Tags) {
		t.Errorf("Tags len = %d, want %d", len(restored.Tags), len(original.Tags))
	}
	if restored.Embedding != nil {
		t.Error("restored memory should have nil Embedding")
	}
}

func TestLinksToJSON(t *testing.T) {
	links := []*db.MemoryLink{
		{
			ID:        "link1",
			FromID:    "mem1",
			ToID:      "mem2",
			Relation:  "related_to",
			Weight:    1.0,
			Auto:      false,
			CreatedAt: "2025-01-01 00:00:00",
		},
		{
			ID:        "link2",
			FromID:    "mem1",
			ToID:      "mem3",
			Relation:  "caused_by",
			Weight:    0.8,
			Auto:      true,
			CreatedAt: "2025-01-02 00:00:00",
		},
	}

	data, err := LinksToJSON(links)
	if err != nil {
		t.Fatalf("LinksToJSON: %v", err)
	}

	// Verify it's valid JSON array
	var parsed []SerializableLink
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(parsed) != 2 {
		t.Fatalf("got %d links, want 2", len(parsed))
	}
	if parsed[0].ToID != "mem2" {
		t.Errorf("link[0].ToID = %q, want %q", parsed[0].ToID, "mem2")
	}
	if parsed[1].Auto != true {
		t.Error("link[1].Auto should be true")
	}

	restored, err := JSONToLinks("mem1", data)
	if err != nil {
		t.Fatalf("JSONToLinks: %v", err)
	}
	if len(restored) != 2 || restored[0].FromID != "mem1" || restored[1].ToID != "mem3" {
		t.Fatalf("unexpected restored links: %+v", restored)
	}
}

func TestContextRoundTrip(t *testing.T) {
	record := &db.MemoryContextRecord{
		MemoryID: "abc123",
		Repository: db.RepositoryRecord{
			Host:          "github.com",
			Owner:         "j33pguy",
			Name:          "magi",
			CanonicalName: "j33pguy/magi",
			DisplayName:   "j33pguy/magi",
		},
		ScopeMachine:            "gilfoyle",
		ScopeAgent:              "codex",
		ProvenanceTransport:     "http",
		ProvenanceImportedFrom:  "discord",
		ProvenanceHumanAuthored: true,
		DurableAt:               "2026-04-15T23:00:00Z",
	}

	data, err := ContextToJSON(record)
	if err != nil {
		t.Fatalf("ContextToJSON: %v", err)
	}
	restored, err := JSONToContext(data)
	if err != nil {
		t.Fatalf("JSONToContext: %v", err)
	}
	if restored.MemoryID != record.MemoryID || restored.Repository.CanonicalName != record.Repository.CanonicalName || restored.ScopeMachine != record.ScopeMachine || restored.ProvenanceTransport != record.ProvenanceTransport || !restored.ProvenanceHumanAuthored {
		t.Fatalf("unexpected restored context: %+v", restored)
	}
}
