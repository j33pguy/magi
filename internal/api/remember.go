package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/pipeline"
	"github.com/j33pguy/magi/internal/remember"
)

type rememberRequest struct {
	Content string `json:"content"`
	Summary string `json:"summary"`
	Project string `json:"project"`
	Type    string `json:"type"`
	// Visibility: "private", "internal" (default), or "public"
	Visibility string   `json:"visibility"`
	Tags       []string `json:"tags"`
	Source     string   `json:"source"`
	Speaker    string   `json:"speaker"`
	Area       string   `json:"area"`
	SubArea    string   `json:"sub_area"`
}

func (s *Server) handleRemember(w http.ResponseWriter, r *http.Request) {
	s.handleRememberWithDefaultSource(w, r, "api")
}

func (s *Server) handleSyncRemember(w http.ResponseWriter, r *http.Request) {
	if !requireMachineOrAdminIdentity(w, r) {
		return
	}
	s.handleRememberWithDefaultSource(w, r, "magi-sync")
}

func (s *Server) handleRememberWithDefaultSource(w http.ResponseWriter, r *http.Request, defaultSource string) {
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
		req.Source = defaultSource
	}
	if req.Speaker == "" {
		req.Speaker = "assistant"
	}

	if s.pipeline != nil {
		s.handleRememberAsync(w, req)
		return
	}

	input := remember.Input{
		Content:    req.Content,
		Summary:    req.Summary,
		Project:    req.Project,
		Type:       req.Type,
		Visibility: req.Visibility,
		Source:     req.Source,
		Speaker:    req.Speaker,
		Area:       req.Area,
		SubArea:    req.SubArea,
		Tags:       req.Tags,
	}
	result, err := remember.Remember(r.Context(), s.db, s.embedder, input, remember.Options{
		TagMode:       remember.TagModeWarn,
		Logger:        s.logger,
		SecretManager: s.secrets,
	})
	if err != nil {
		var secretErr *remember.SecretError
		if errors.As(err, &secretErr) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": secretErr.Error()})
			return
		}
		s.logger.Error("remember failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if result.Deduplicated {
		writeJSON(w, http.StatusOK, map[string]any{
			"id":           result.Match.Memory.ID,
			"ok":           true,
			"deduplicated": true,
			"note":         fmt.Sprintf("existing memory %s is %.1f%% similar", result.Match.Memory.ID, (1.0-result.Match.Distance)*100),
		})
		return
	}

	resp := map[string]any{"id": result.Saved.ID, "ok": true}
	if result.TagWarning != "" {
		resp["tag_warning"] = "tags may not have been saved: " + result.TagWarning[:min(len(result.TagWarning), 80)]
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleRememberAsync(w http.ResponseWriter, req rememberRequest) {
	mem := rememberMemory(req)
	result, err := s.pipeline.Submit(pipelineWriteRequest(&mem, req.Tags))
	if err != nil {
		s.logger.Error("remember async submit failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":    result.ID,
		"ok":    true,
		"async": true,
		"status": map[string]any{
			"state": pipelineStatePending(),
			"url":   fmt.Sprintf("/memory/%s/status", result.ID),
		},
	})
}

func rememberMemory(req rememberRequest) db.Memory {
	return db.Memory{
		Content:    req.Content,
		Summary:    req.Summary,
		Project:    req.Project,
		Type:       req.Type,
		Visibility: req.Visibility,
		Source:     req.Source,
		Speaker:    req.Speaker,
		Area:       req.Area,
		SubArea:    req.SubArea,
		TokenCount: len(req.Content) / 4,
	}
}

func pipelineWriteRequest(mem *db.Memory, tags []string) pipeline.WriteRequest {
	return pipeline.WriteRequest{Memory: mem, Tags: tags}
}

func pipelineStatePending() string {
	return "pending"
}
