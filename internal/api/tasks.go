package api

import (
	"encoding/json"
	"net/http"

	"github.com/j33pguy/magi/internal/db"
)

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var task db.Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if task.Description == "" {
		http.Error(w, "description required", http.StatusBadRequest)
		return
	}
	result, err := s.db.CreateTask(&task)
	if err != nil {
		s.logger.Error("creating task", "error", err)
		http.Error(w, "create failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	task, err := s.db.GetTask(id)
	if err != nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	tasks, err := s.db.ListTasks(status)
	if err != nil {
		s.logger.Error("listing tasks", "error", err)
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}
	if tasks == nil {
		tasks = []*db.Task{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

func (s *Server) handleReportProgress(w http.ResponseWriter, r *http.Request) {
	subtaskID := r.PathValue("subtask_id")
	var req struct {
		Percent int    `json:"percent"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := s.db.ReportProgress(subtaskID, req.Percent, req.Message); err != nil {
		s.logger.Error("reporting progress", "error", err)
		http.Error(w, "progress update failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleUpdateSubtask(w http.ResponseWriter, r *http.Request) {
	subtaskID := r.PathValue("subtask_id")
	var req struct {
		Status  string `json:"status"`
		Output  string `json:"output"`
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.AgentID != "" {
		if err := s.db.AssignSubtask(subtaskID, req.AgentID); err != nil {
			s.logger.Error("assigning subtask", "error", err)
			http.Error(w, "assign failed", http.StatusInternalServerError)
			return
		}
	}
	if req.Status != "" {
		if err := s.db.UpdateSubtaskStatus(subtaskID, req.Status, req.Output); err != nil {
			s.logger.Error("updating subtask", "error", err)
			http.Error(w, "update failed", http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
