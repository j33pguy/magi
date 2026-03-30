package api

import (
	"net/http"

	"github.com/j33pguy/magi/internal/pipeline"
)

// SetPipeline sets the async write pipeline on the API server.
func (s *Server) SetPipeline(p *pipeline.Writer) {
	s.pipeline = p
}

func (s *Server) handleMemoryStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}

	if s.pipeline == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "async pipeline not enabled"})
		return
	}

	status := s.pipeline.Status(id)
	if status == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no status found for this id"})
		return
	}

	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handlePipelineStats(w http.ResponseWriter, _ *http.Request) {
	if s.pipeline == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "async pipeline not enabled"})
		return
	}

	writeJSON(w, http.StatusOK, s.pipeline.Stats())
}
