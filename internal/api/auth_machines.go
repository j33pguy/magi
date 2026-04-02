package api

import (
	"encoding/json"
	"net/http"

	"github.com/j33pguy/magi/internal/auth"
	"github.com/j33pguy/magi/internal/db"
)

type enrollMachineRequest struct {
	User        string   `json:"user"`
	MachineID   string   `json:"machine_id"`
	AgentName   string   `json:"agent_name"`
	AgentType   string   `json:"agent_type"`
	Groups      []string `json:"groups"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
}

func (s *Server) handleEnrollMachine(w http.ResponseWriter, r *http.Request) {
	if !requireAdminIdentity(w, r) {
		return
	}
	if s.machines == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "machine registry not configured"})
		return
	}

	var req enrollMachineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.User == "" || req.MachineID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user and machine_id are required"})
		return
	}

	token, tokenHash, err := auth.GenerateToken()
	if err != nil {
		s.logger.Error("generating machine token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	cred, err := s.machines.CreateMachineCredential(&db.MachineCredential{
		TokenHash:   tokenHash,
		User:        req.User,
		MachineID:   req.MachineID,
		AgentName:   req.AgentName,
		AgentType:   req.AgentType,
		Groups:      req.Groups,
		DisplayName: req.DisplayName,
		Description: req.Description,
	})
	if err != nil {
		s.logger.Error("creating machine credential", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":     true,
		"token":  token,
		"record": cred,
	})
}

func (s *Server) handleListMachineCredentials(w http.ResponseWriter, r *http.Request) {
	if !requireAdminIdentity(w, r) {
		return
	}
	if s.machines == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "machine registry not configured"})
		return
	}

	creds, err := s.machines.ListMachineCredentials()
	if err != nil {
		s.logger.Error("listing machine credentials", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, creds)
}

func (s *Server) handleRevokeMachineCredential(w http.ResponseWriter, r *http.Request) {
	if !requireAdminIdentity(w, r) {
		return
	}
	if s.machines == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "machine registry not configured"})
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	if err := s.machines.RevokeMachineCredential(id); err != nil {
		s.logger.Error("revoking machine credential", "error", err, "id", id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func requireAdminIdentity(w http.ResponseWriter, r *http.Request) bool {
	identity, ok := auth.FromContext(r.Context())
	if !ok || identity.Kind != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin token required"})
		return false
	}
	return true
}

func requireMachineOrAdminIdentity(w http.ResponseWriter, r *http.Request) bool {
	identity, ok := auth.FromContext(r.Context())
	if !ok || (identity.Kind != "admin" && identity.Kind != "machine") {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "machine or admin token required"})
		return false
	}
	return true
}
