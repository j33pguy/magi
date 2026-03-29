package api

import (
	"encoding/json"
	"net/http"

	"github.com/j33pguy/magi/internal/db"
)

func (s *Server) handleRegisterAgent(w http.ResponseWriter, r *http.Request) {
	var agent db.Agent
	if err := json.NewDecoder(r.Body).Decode(&agent); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if agent.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	result, err := s.db.RegisterAgent(&agent)
	if err != nil {
		s.logger.Error("registering agent", "error", err)
		http.Error(w, "registration failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	agents, err := s.db.ListAgents(status)
	if err != nil {
		s.logger.Error("listing agents", "error", err)
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}
	if agents == nil {
		agents = []*db.Agent{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agents)
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent, err := s.db.GetAgent(id)
	if err != nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agent)
}

func (s *Server) handleHeartbeatAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.db.HeartbeatAgent(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleDeregisterAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.db.DeregisterAgent(id); err != nil {
		http.Error(w, "deregister failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
