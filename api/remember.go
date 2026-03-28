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
	Speaker    string   `json:"speaker"`
	Area       string   `json:"area"`
	SubArea    string   `json:"sub_area"`
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
		req.Type = "memory"
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

	speaker := req.Speaker
	if speaker == "" {
		speaker = "gilfoyle"
	}

	memory := &db.Memory{
		Content:    req.Content,
		Summary:    req.Summary,
		Embedding:  embedding,
		Project:    req.Project,
		Type:       req.Type,
		Visibility: req.Visibility, // defaults to "internal" in SaveMemory if empty
		Source:     req.Source,
		Speaker:    speaker,
		Area:       req.Area,
		SubArea:    req.SubArea,
		TokenCount: len(req.Content) / 4,
	}

	saved, err := s.db.SaveMemory(memory)
	if err != nil {
		s.logger.Error("saving memory", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("saving memory: %v", err)})
		return
	}

	var tagErr string
	if len(req.Tags) > 0 {
		if err := s.db.SetTags(saved.ID, req.Tags); err != nil {
			// Tags are non-fatal — log the error but return the memory ID.
			// Turso Hrana streams can expire between the INSERT and the tag write;
			// the memory is stored, tags can be retried later.
			s.logger.Warn("setting tags failed (non-fatal)", "error", err, "memory_id", saved.ID)
			tagErr = err.Error()
		}
	}

	resp := map[string]any{"id": saved.ID, "ok": true}
	if tagErr != "" {
		resp["tag_warning"] = "tags may not have been saved: " + tagErr[:min(len(tagErr), 80)]
	}
	writeJSON(w, http.StatusCreated, resp)
}
