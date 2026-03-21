package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/j33pguy/claude-memory/db"
)

type rememberRequest struct {
	Content    string   `json:"content"`
	Summary    string   `json:"summary"`
	Project    string   `json:"project"`
	Type       string   `json:"type"`
	// Visibility: "private", "internal" (default), or "public"
	Visibility string   `json:"visibility"`
	Tags       []string `json:"tags"`
	Source     string   `json:"source"`
}

func (s *Server) handleRemember(w http.ResponseWriter, r *http.Request) {
	var req rememberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content is required"})
		return
	}

	if req.Type == "" {
		req.Type = "note"
	}
	if req.Source == "" {
		req.Source = "api"
	}

	embedding, err := s.embedder.Embed(r.Context(), req.Content)
	if err != nil {
		s.logger.Error("generating embedding", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("generating embedding: %v", err)})
		return
	}

	memory := &db.Memory{
		Content:    req.Content,
		Summary:    req.Summary,
		Embedding:  embedding,
		Project:    req.Project,
		Type:       req.Type,
		Visibility: req.Visibility, // defaults to "internal" in SaveMemory if empty
		Source:     req.Source,
		TokenCount: len(req.Content) / 4,
	}

	saved, err := s.db.SaveMemory(memory)
	if err != nil {
		s.logger.Error("saving memory", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("saving memory: %v", err)})
		return
	}

	if len(req.Tags) > 0 {
		if err := s.db.SetTags(saved.ID, req.Tags); err != nil {
			s.logger.Error("setting tags", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("setting tags: %v", err)})
			return
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id": saved.ID,
		"ok": true,
	})
}
