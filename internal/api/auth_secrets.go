package api

import (
	"encoding/json"
	"net/http"
)

type resolveSecretRequest struct {
	Path string `json:"path"`
	Key  string `json:"key"`
}

func (s *Server) handleResolveSecret(w http.ResponseWriter, r *http.Request) {
	if !requireAdminIdentity(w, r) {
		return
	}
	if s.secrets == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "secret backend not configured"})
		return
	}
	var req resolveSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Path == "" || req.Key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path and key are required"})
		return
	}
	value, err := s.secrets.Resolve(r.Context(), req.Path, req.Key)
	if err != nil {
		s.logger.Error("resolve secret failed", "error", err, "path", req.Path, "key", req.Key)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"backend": s.secrets.BackendName(),
		"path":    req.Path,
		"key":     req.Key,
		"value":   value,
	})
}
