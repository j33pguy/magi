package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/patterns"
)

func TestHandleListPatterns(t *testing.T) {
	s := newTestServer(t)

	for i := 0; i < 3; i++ {
		content := "I prefer gRPC for services"
		emb, _ := s.embedder.Embed(context.Background(), content)
		_, err := s.db.SaveMemory(&db.Memory{
			Content:    content,
			Embedding:  emb,
			Project:    "proj",
			Type:       "memory",
			Visibility: "internal",
			Speaker:    "user",
			Source:     "discord",
		})
		if err != nil {
			t.Fatalf("SaveMemory: %v", err)
		}
	}

	req := httptest.NewRequest("GET", "/patterns", nil)
	w := httptest.NewRecorder()
	s.handleListPatterns(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}

	var result []patterns.Pattern
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	found := false
	for _, p := range result {
		if p.Type == patterns.PatternPreference {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected preference pattern")
	}
}

func TestHandleListTrendingPatterns(t *testing.T) {
	s := newTestServer(t)

	for i := 0; i < 3; i++ {
		content := "I prefer Go for backend services"
		emb, _ := s.embedder.Embed(context.Background(), content)
		_, err := s.db.SaveMemory(&db.Memory{
			Content:    content,
			Embedding:  emb,
			Project:    "proj",
			Type:       "memory",
			Visibility: "internal",
			Speaker:    "user",
			Source:     "webchat",
		})
		if err != nil {
			t.Fatalf("SaveMemory: %v", err)
		}
	}

	req := httptest.NewRequest("GET", "/patterns/trending", nil)
	w := httptest.NewRecorder()
	s.handleListTrendingPatterns(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}

	var result []patterns.Pattern
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result) == 0 {
		t.Fatalf("expected trending patterns")
	}
}
