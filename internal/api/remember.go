package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

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
