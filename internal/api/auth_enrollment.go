package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/j33pguy/magi/internal/auth"
	"github.com/j33pguy/magi/internal/db"
)

type createEnrollmentTokenRequest struct {
	Label         string   `json:"label"`
	MaxUses       int      `json:"max_uses"`
	DefaultUser   string   `json:"default_user"`
	DefaultGroups []string `json:"default_groups"`
	ExpiresIn     string   `json:"expires_in"` // e.g. "24h", "1h", "30m"
}

type selfEnrollRequest struct {
	Token       string   `json:"token"`
	MachineID   string   `json:"machine_id"`
	User        string   `json:"user"`
	AgentName   string   `json:"agent_name"`
	AgentType   string   `json:"agent_type"`
	Groups      []string `json:"groups"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
}

// handleCreateEnrollmentToken creates a new enrollment token (admin-only).
func (s *Server) handleCreateEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	if !requireAdminIdentity(w, r) {
		return
	}
	if s.enrollment == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "enrollment not configured"})
		return
	}

	var req createEnrollmentTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.MaxUses <= 0 {
		req.MaxUses = 1
	}

	// Generate the token (plaintext returned once, hash stored)
	plaintext, tokenHash, err := auth.GenerateToken()
	if err != nil {
		s.logger.Error("generating enrollment token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	et := &db.EnrollmentToken{
		TokenHash:     tokenHash,
		Label:         req.Label,
		MaxUses:       req.MaxUses,
		DefaultUser:   req.DefaultUser,
		DefaultGroups: req.DefaultGroups,
	}

	if req.ExpiresIn != "" {
		dur, err := time.ParseDuration(req.ExpiresIn)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid expires_in duration"})
			return
		}
		et.ExpiresAt = time.Now().UTC().Add(dur).Format(time.DateTime)
	}

	et, err = s.enrollment.CreateEnrollmentToken(et)
	if err != nil {
		s.logger.Error("creating enrollment token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":    true,
		"token": plaintext,
		"record": map[string]any{
			"id":             et.ID,
			"label":          et.Label,
			"max_uses":       et.MaxUses,
			"default_user":   et.DefaultUser,
			"default_groups": et.DefaultGroups,
			"expires_at":     et.ExpiresAt,
			"created_at":     et.CreatedAt,
		},
	})
}

// handleListEnrollmentTokens lists all enrollment tokens (admin-only).
func (s *Server) handleListEnrollmentTokens(w http.ResponseWriter, r *http.Request) {
	if !requireAdminIdentity(w, r) {
		return
	}
	if s.enrollment == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "enrollment not configured"})
		return
	}
	tokens, err := s.enrollment.ListEnrollmentTokens()
	if err != nil {
		s.logger.Error("listing enrollment tokens", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, tokens)
}

// handleRevokeEnrollmentToken revokes an enrollment token (admin-only).
func (s *Server) handleRevokeEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	if !requireAdminIdentity(w, r) {
		return
	}
	if s.enrollment == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "enrollment not configured"})
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	if err := s.enrollment.RevokeEnrollmentToken(id); err != nil {
		s.logger.Error("revoking enrollment token", "error", err, "id", id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

// handleSelfEnroll exchanges an enrollment token for a machine credential.
// This endpoint is UNAUTHENTICATED — the enrollment token in the body IS the auth.
func (s *Server) handleSelfEnroll(w http.ResponseWriter, r *http.Request) {
	if s.enrollment == nil || s.machines == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "enrollment not configured"})
		return
	}

	var req selfEnrollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Token == "" || req.MachineID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token and machine_id are required"})
		return
	}

	// Look up the enrollment token by hash
	tokenHash := auth.HashToken(req.Token)
	et, err := s.enrollment.GetEnrollmentTokenByHash(tokenHash)
	if err != nil {
		s.logger.Error("looking up enrollment token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if et == nil || !et.IsValid() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid or expired enrollment token"})
		return
	}

	// Use defaults from enrollment token if not provided
	user := req.User
	if user == "" {
		user = et.DefaultUser
	}
	if user == "" {
		user = "unknown"
	}

	groups := req.Groups
	if len(groups) == 0 {
		groups = et.DefaultGroups
	}

	// Generate machine token
	machineToken, machineHash, err := auth.GenerateToken()
	if err != nil {
		s.logger.Error("generating machine token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	cred, err := s.machines.CreateMachineCredential(&db.MachineCredential{
		TokenHash:   machineHash,
		User:        user,
		MachineID:   req.MachineID,
		AgentName:   req.AgentName,
		AgentType:   req.AgentType,
		Groups:      groups,
		DisplayName: req.DisplayName,
		Description: req.Description,
	})
	if err != nil {
		s.logger.Error("creating machine credential via enrollment", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	// Burn the enrollment token (increment use count)
	if err := s.enrollment.IncrementEnrollmentTokenUse(et.ID); err != nil {
		s.logger.Error("incrementing enrollment token use", "error", err, "token_id", et.ID)
		// Non-fatal — credential was already created
	}

	s.logger.Info("machine enrolled via enrollment token",
		"machine_id", req.MachineID,
		"user", user,
		"enrollment_token_id", et.ID,
		"enrollment_label", et.Label,
	)

	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":     true,
		"token":  machineToken,
		"record": cred,
	})
}
