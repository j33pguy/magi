package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/j33pguy/magi/internal/auth"
	"github.com/j33pguy/magi/internal/db"
)

type createTaskRequest struct {
	Project      string            `json:"project"`
	Queue        string            `json:"queue"`
	Title        string            `json:"title"`
	Summary      string            `json:"summary"`
	Description  string            `json:"description"`
	Status       string            `json:"status"`
	Priority     string            `json:"priority"`
	CreatedBy    string            `json:"created_by"`
	Orchestrator string            `json:"orchestrator"`
	Worker       string            `json:"worker"`
	ParentTaskID string            `json:"parent_task_id"`
	Metadata     map[string]string `json:"metadata"`
	ActorRole    string            `json:"actor_role"`
	ActorName    string            `json:"actor_name"`
}

type updateTaskRequest struct {
	Project       *string            `json:"project"`
	Queue         *string            `json:"queue"`
	Title         *string            `json:"title"`
	Summary       *string            `json:"summary"`
	Description   *string            `json:"description"`
	Status        *string            `json:"status"`
	Priority      *string            `json:"priority"`
	CreatedBy     *string            `json:"created_by"`
	Orchestrator  *string            `json:"orchestrator"`
	Worker        *string            `json:"worker"`
	ParentTaskID  *string            `json:"parent_task_id"`
	Metadata      *map[string]string `json:"metadata"`
	StatusComment string             `json:"status_comment"`
	ActorRole     string             `json:"actor_role"`
	ActorName     string             `json:"actor_name"`
}

type createTaskEventRequest struct {
	EventType    string            `json:"event_type"`
	ActorRole    string            `json:"actor_role"`
	ActorName    string            `json:"actor_name"`
	ActorUser    string            `json:"actor_user"`
	ActorMachine string            `json:"actor_machine"`
	ActorAgent   string            `json:"actor_agent"`
	Summary      string            `json:"summary"`
	Content      string            `json:"content"`
	Status       string            `json:"status"`
	MemoryID     string            `json:"memory_id"`
	Source       string            `json:"source"`
	Metadata     map[string]string `json:"metadata"`
}

type taskActor struct {
	role    string
	name    string
	user    string
	machine string
	agent   string
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	if s.tasks == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "task queue not available"})
		return
	}

	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title is required"})
		return
	}
	if req.Status != "" && !db.ValidTaskStatus(req.Status) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid task status"})
		return
	}
	if req.Priority != "" && !db.ValidTaskPriority(req.Priority) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid task priority"})
		return
	}

	actor := resolveTaskActor(r, req.ActorRole, req.ActorName, "", "", "")
	createdBy := req.CreatedBy
	if createdBy == "" {
		createdBy = actor.user
		if createdBy == "" {
			createdBy = actor.name
		}
	}

	task := &db.Task{
		Project:      req.Project,
		Queue:        req.Queue,
		Title:        req.Title,
		Summary:      req.Summary,
		Description:  req.Description,
		Status:       req.Status,
		Priority:     req.Priority,
		CreatedBy:    createdBy,
		Orchestrator: req.Orchestrator,
		Worker:       req.Worker,
		ParentTaskID: req.ParentTaskID,
		Metadata:     req.Metadata,
	}

	saved, err := s.tasks.CreateTask(task)
	if err != nil {
		s.logger.Error("creating task", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if _, err := s.tasks.CreateTaskEvent(initialTaskStatusEvent(saved, actor)); err != nil {
		s.logger.Warn("creating initial task status event failed", "task_id", saved.ID, "error", err)
	}

	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	if s.tasks == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "task queue not available"})
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	tasks, err := s.tasks.ListTasks(&db.TaskFilter{
		Project:      r.URL.Query().Get("project"),
		Queue:        r.URL.Query().Get("queue"),
		Status:       r.URL.Query().Get("status"),
		Worker:       r.URL.Query().Get("worker"),
		Orchestrator: r.URL.Query().Get("orchestrator"),
		Limit:        limit,
	})
	if err != nil {
		s.logger.Error("listing tasks", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	if s.tasks == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "task queue not available"})
		return
	}
	id := r.PathValue("id")
	task, err := s.tasks.GetTask(id)
	if err != nil {
		s.logger.Error("getting task", "id", id, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if task == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	if s.tasks == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "task queue not available"})
		return
	}

	id := r.PathValue("id")
	task, err := s.tasks.GetTask(id)
	if err != nil {
		s.logger.Error("getting task for update", "id", id, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if task == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}

	var req updateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	oldStatus := task.Status
	applyTaskUpdate(task, &req)
	if req.Status != nil && !db.ValidTaskStatus(*req.Status) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid task status"})
		return
	}
	if req.Priority != nil && !db.ValidTaskPriority(*req.Priority) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid task priority"})
		return
	}
	if task.Title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title is required"})
		return
	}

	if err := s.tasks.UpdateTask(task); err != nil {
		s.logger.Error("updating task", "id", id, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	if task.Status != oldStatus {
		actor := resolveTaskActor(r, req.ActorRole, req.ActorName, "", "", "")
		event := initialTaskStatusEvent(task, actor)
		event.Summary = fmt.Sprintf("Task status %s -> %s", oldStatus, task.Status)
		event.Content = req.StatusComment
		event.Status = task.Status
		if _, err := s.tasks.CreateTaskEvent(event); err != nil {
			s.logger.Warn("creating task status event failed", "task_id", task.ID, "error", err)
		}
	}

	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleCreateTaskEvent(w http.ResponseWriter, r *http.Request) {
	if s.tasks == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "task queue not available"})
		return
	}

	taskID := r.PathValue("id")
	task, err := s.tasks.GetTask(taskID)
	if err != nil {
		s.logger.Error("getting task for event", "id", taskID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if task == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}

	var req createTaskEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !db.ValidTaskEventType(req.EventType) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid task event type"})
		return
	}
	if req.EventType == db.TaskEventStatus {
		if !db.ValidTaskStatus(req.Status) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status events require a valid task status"})
			return
		}
		task.Status = req.Status
		if err := s.tasks.UpdateTask(task); err != nil {
			s.logger.Error("updating task status from event", "id", taskID, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
	}
	if req.MemoryID != "" {
		memory, err := s.db.GetMemory(req.MemoryID)
		if err != nil || memory == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "memory not found for task event"})
			return
		}
	}

	actor := resolveTaskActor(r, req.ActorRole, req.ActorName, req.ActorUser, req.ActorMachine, req.ActorAgent)
	event := &db.TaskEvent{
		TaskID:       taskID,
		EventType:    req.EventType,
		ActorRole:    actor.role,
		ActorName:    actor.name,
		ActorUser:    actor.user,
		ActorMachine: actor.machine,
		ActorAgent:   actor.agent,
		Summary:      req.Summary,
		Content:      req.Content,
		Status:       req.Status,
		MemoryID:     req.MemoryID,
		Source:       req.Source,
		Metadata:     req.Metadata,
	}
	if event.Summary == "" {
		switch event.EventType {
		case db.TaskEventStatus:
			event.Summary = "Task status -> " + event.Status
		case db.TaskEventMemoryRef:
			event.Summary = "Linked memory " + event.MemoryID
		}
	}

	saved, err := s.tasks.CreateTaskEvent(event)
	if err != nil {
		s.logger.Error("creating task event", "task_id", taskID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) handleListTaskEvents(w http.ResponseWriter, r *http.Request) {
	if s.tasks == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "task queue not available"})
		return
	}

	taskID := r.PathValue("id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	events, err := s.tasks.ListTaskEvents(taskID, limit)
	if err != nil {
		s.logger.Error("listing task events", "task_id", taskID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func applyTaskUpdate(task *db.Task, req *updateTaskRequest) {
	if req.Project != nil {
		task.Project = *req.Project
	}
	if req.Queue != nil {
		task.Queue = *req.Queue
	}
	if req.Title != nil {
		task.Title = *req.Title
	}
	if req.Summary != nil {
		task.Summary = *req.Summary
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.Status != nil {
		task.Status = *req.Status
	}
	if req.Priority != nil {
		task.Priority = *req.Priority
	}
	if req.CreatedBy != nil {
		task.CreatedBy = *req.CreatedBy
	}
	if req.Orchestrator != nil {
		task.Orchestrator = *req.Orchestrator
	}
	if req.Worker != nil {
		task.Worker = *req.Worker
	}
	if req.ParentTaskID != nil {
		task.ParentTaskID = *req.ParentTaskID
	}
	if req.Metadata != nil {
		task.Metadata = *req.Metadata
	}
}

func resolveTaskActor(r *http.Request, role, name, user, machine, agent string) taskActor {
	actor := taskActor{
		role:    role,
		name:    name,
		user:    user,
		machine: machine,
		agent:   agent,
	}
	if identity, ok := auth.FromContext(r.Context()); ok {
		if actor.user == "" {
			actor.user = auth.EffectiveUser(identity)
		}
		if actor.machine == "" {
			actor.machine = identity.MachineID
		}
		if actor.agent == "" {
			if identity.AgentName != "" {
				actor.agent = identity.AgentName
			} else {
				actor.agent = identity.AgentType
			}
		}
	}
	if actor.name == "" {
		switch {
		case actor.user != "":
			actor.name = actor.user
		case actor.agent != "":
			actor.name = actor.agent
		case actor.machine != "":
			actor.name = actor.machine
		default:
			actor.name = "system"
		}
	}
	if actor.role == "" {
		switch {
		case actor.agent != "":
			actor.role = "worker"
		case actor.user != "":
			actor.role = "operator"
		default:
			actor.role = "system"
		}
	}
	return actor
}

func initialTaskStatusEvent(task *db.Task, actor taskActor) *db.TaskEvent {
	return &db.TaskEvent{
		TaskID:       task.ID,
		EventType:    db.TaskEventStatus,
		ActorRole:    actor.role,
		ActorName:    actor.name,
		ActorUser:    actor.user,
		ActorMachine: actor.machine,
		ActorAgent:   actor.agent,
		Summary:      "Task status -> " + task.Status,
		Status:       task.Status,
		Source:       "tasks-api",
	}
}
