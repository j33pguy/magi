package api

import (
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/j33pguy/magi/internal/vcs"
)

// SetGitRepo sets the git repository for history/diff endpoints.
func (s *Server) SetGitRepo(repo *vcs.Repo) {
	s.gitRepo = repo
}

func (s *Server) handleMemoryHistory(w http.ResponseWriter, r *http.Request) {
	if s.gitRepo == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "git versioning is not enabled"})
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}

	// Verify memory exists
	if _, err := s.db.GetMemory(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("memory not found: %v", err)})
		return
	}

	relPath := filepath.Join("memories", id+".json")
	commits, err := s.gitRepo.Log(relPath)
	if err != nil {
		s.logger.Error("git log", "error", err, "id", id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("fetching history: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":      id,
		"entries": commits,
	})
}

func (s *Server) handleMemoryDiff(w http.ResponseWriter, r *http.Request) {
	if s.gitRepo == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "git versioning is not enabled"})
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}

	fromCommit := r.URL.Query().Get("from")
	toCommit := r.URL.Query().Get("to")
	if fromCommit == "" || toCommit == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "both 'from' and 'to' query params are required"})
		return
	}

	relPath := filepath.Join("memories", id+".json")
	diff, err := s.gitRepo.Diff(relPath, fromCommit, toCommit)
	if err != nil {
		s.logger.Error("git diff", "error", err, "id", id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("computing diff: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":           id,
		"from_commit":  fromCommit,
		"to_commit":    toCommit,
		"from_content": diff.From,
		"to_content":   diff.To,
		"diff":         diff.Content,
	})
}
