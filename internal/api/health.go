package api

import (
	"net/http"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

// version is the current MAGI version.
const version = "0.1.0"

// startTime records when the server started, used for uptime calculation.
var startTime = time.Now()

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"ok":      true,
		"version": version,
		"uptime":  time.Since(startTime).String(),
	}

	// Database status
	memories, err := s.db.ListMemories(&db.MemoryFilter{Limit: 1})
	if err != nil {
		resp["db_status"] = "error"
		resp["db_error"] = err.Error()
		resp["ok"] = false
	} else {
		resp["db_status"] = "ok"
		resp["memory_count"] = len(memories)
		// Get a rough total count via an unfiltered list with high limit
		all, _ := s.db.ListMemories(&db.MemoryFilter{Limit: 100000})
		resp["memory_count"] = len(all)
	}

	// Git repo status
	if s.gitRepo != nil {
		resp["git_status"] = "enabled"
	} else {
		resp["git_status"] = "disabled"
	}

	if !resp["ok"].(bool) {
		writeJSON(w, http.StatusServiceUnavailable, resp)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleReadyz is a readiness probe — returns 200 only when the database is ready.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	_, err := s.db.ListMemories(&db.MemoryFilter{Limit: 1})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"ready": false,
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ready": true})
}

// handleLivez is a liveness probe — returns 200 if the process is alive.
func (s *Server) handleLivez(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"alive": true})
}
