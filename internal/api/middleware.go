package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// requireAuth wraps a handler with bearer token authentication.
// Uses constant-time comparison to prevent timing attacks.
// When MAGI_API_TOKEN is not set, only read-only (GET) requests are allowed.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token == "" {
			// No token configured: read-only mode — block writes
			if r.Method != http.MethodGet {
				writeJSON(w, http.StatusForbidden, map[string]string{
					"error": "write operations require MAGI_API_TOKEN to be set",
				})
				return
			}
			next(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		provided := auth[7:]
		if subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		next(w, r)
	}
}
