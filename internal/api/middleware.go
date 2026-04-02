package api

import (
	"net/http"
	"strings"

	"github.com/j33pguy/magi/internal/auth"
)

// requireAuth wraps a handler with bearer token authentication.
// Uses constant-time comparison to prevent timing attacks.
// When MAGI_API_TOKEN is not set, only read-only (GET) requests are allowed.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") && s.auth != nil {
			provided := authHeader[7:]
			identity, ok := s.auth.ResolveBearer(provided)
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}

			if identity != nil {
				if identity.User != "" {
					r.Header.Set("X-MAGI-Auth-User", identity.User)
				}
				if len(identity.Groups) > 0 {
					r.Header.Set("X-MAGI-Auth-Groups", strings.Join(identity.Groups, ","))
				}
				if identity.MachineID != "" {
					r.Header.Set("X-MAGI-Auth-Machine", identity.MachineID)
				}
				if identity.AgentName != "" {
					r.Header.Set("X-MAGI-Auth-Agent", identity.AgentName)
				}
				r.Header.Set("X-MAGI-Auth-Kind", identity.Kind)
			}

			next(w, r.WithContext(auth.NewContext(r.Context(), identity)))
			return
		}

		if s.auth == nil || !s.auth.Enabled() {
			// No explicit auth configured: read-only mode — block writes.
			if r.Method != http.MethodGet {
				writeJSON(w, http.StatusForbidden, map[string]string{
					"error": "write operations require MAGI_API_TOKEN to be set",
				})
				return
			}
			next(w, r)
			return
		}

		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
}
