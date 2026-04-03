package grpc

import (
	"context"

	"github.com/j33pguy/magi/internal/auth"
	"github.com/j33pguy/magi/internal/db"
	pb "github.com/j33pguy/magi/proto/memory/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SetTaskStore configures the task queue backend.
func (s *Server) SetTaskStore(tasks db.TaskQueueStore) {
	s.tasks = tasks
}

func (s *Server) CreateTask(ctx context.Context, req *pb.CreateTaskRequest) (*pb.CreateTaskResponse, error) {
	if s.tasks == nil {
		return nil, status.Error(codes.Unimplemented, "task queue not available")
	}
	if req.Title == "" {
		return nil, status.Error(codes.InvalidArgument, "title is required")
	}
	if req.Status != "" && !db.ValidTaskStatus(req.Status) {
		return nil, status.Error(codes.InvalidArgument, "invalid task status")
	}
	if req.Priority != "" && !db.ValidTaskPriority(req.Priority) {
		return nil, status.Error(codes.InvalidArgument, "invalid task priority")
	}

	createdBy := req.CreatedBy
	if createdBy == "" {
		if identity, ok := auth.FromContext(ctx); ok {
			createdBy = auth.EffectiveUser(identity)
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
		ParentTaskID: req.ParentTaskId,
		Metadata:     req.Metadata,
	}

	saved, err := s.tasks.CreateTask(task)
	if err != nil {
		s.logger.Error("creating task", "error", err)
		return nil, status.Errorf(codes.Internal, "creating task: %v", err)
	}

	return &pb.CreateTaskResponse{Task: taskToProto(saved)}, nil
}

func (s *Server) ListTasks(_ context.Context, req *pb.ListTasksRequest) (*pb.ListTasksResponse, error) {
	if s.tasks == nil {
		return nil, status.Error(codes.Unimplemented, "task queue not available")
	}

	tasks, err := s.tasks.ListTasks(&db.TaskFilter{
		Project:      req.Project,
		Queue:        req.Queue,
		Status:       req.Status,
		Worker:       req.Worker,
		Orchestrator: req.Orchestrator,
		Limit:        int(req.Limit),
	})
	if err != nil {
		s.logger.Error("listing tasks", "error", err)
		return nil, status.Errorf(codes.Internal, "listing tasks: %v", err)
	}

	return &pb.ListTasksResponse{Tasks: tasksToProto(tasks)}, nil
}

func (s *Server) GetTask(_ context.Context, req *pb.GetTaskRequest) (*pb.GetTaskResponse, error) {
	if s.tasks == nil {
		return nil, status.Error(codes.Unimplemented, "task queue not available")
	}
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	task, err := s.tasks.GetTask(req.Id)
	if err != nil {
		s.logger.Error("getting task", "id", req.Id, "error", err)
		return nil, status.Errorf(codes.Internal, "getting task: %v", err)
	}
	if task == nil {
		return nil, status.Error(codes.NotFound, "task not found")
	}

	return &pb.GetTaskResponse{Task: taskToProto(task)}, nil
}

func (s *Server) UpdateTask(_ context.Context, req *pb.UpdateTaskRequest) (*pb.UpdateTaskResponse, error) {
	if s.tasks == nil {
		return nil, status.Error(codes.Unimplemented, "task queue not available")
	}
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	task, err := s.tasks.GetTask(req.Id)
	if err != nil {
		s.logger.Error("getting task for update", "id", req.Id, "error", err)
		return nil, status.Errorf(codes.Internal, "getting task: %v", err)
	}
	if task == nil {
		return nil, status.Error(codes.NotFound, "task not found")
	}

	if req.Project != "" {
		task.Project = req.Project
	}
	if req.Queue != "" {
		task.Queue = req.Queue
	}
	if req.Title != "" {
		task.Title = req.Title
	}
	if req.Summary != "" {
		task.Summary = req.Summary
	}
	if req.Description != "" {
		task.Description = req.Description
	}
	if req.Status != "" {
		if !db.ValidTaskStatus(req.Status) {
			return nil, status.Error(codes.InvalidArgument, "invalid task status")
		}
		task.Status = req.Status
	}
	if req.Priority != "" {
		if !db.ValidTaskPriority(req.Priority) {
			return nil, status.Error(codes.InvalidArgument, "invalid task priority")
		}
		task.Priority = req.Priority
	}
	if req.CreatedBy != "" {
		task.CreatedBy = req.CreatedBy
	}
	if req.Orchestrator != "" {
		task.Orchestrator = req.Orchestrator
	}
	if req.Worker != "" {
		task.Worker = req.Worker
	}
	if req.ParentTaskId != "" {
		task.ParentTaskID = req.ParentTaskId
	}
	if req.Metadata != nil {
		task.Metadata = req.Metadata
	}

	if err := s.tasks.UpdateTask(task); err != nil {
		s.logger.Error("updating task", "id", req.Id, "error", err)
		return nil, status.Errorf(codes.Internal, "updating task: %v", err)
	}

	return &pb.UpdateTaskResponse{Task: taskToProto(task)}, nil
}

func (s *Server) CreateTaskEvent(_ context.Context, req *pb.CreateTaskEventRequest) (*pb.CreateTaskEventResponse, error) {
	if s.tasks == nil {
		return nil, status.Error(codes.Unimplemented, "task queue not available")
	}
	if req.TaskId == "" {
		return nil, status.Error(codes.InvalidArgument, "task_id is required")
	}
	if !db.ValidTaskEventType(req.EventType) {
		return nil, status.Error(codes.InvalidArgument, "invalid event type")
	}

	task, err := s.tasks.GetTask(req.TaskId)
	if err != nil {
		s.logger.Error("getting task for event", "id", req.TaskId, "error", err)
		return nil, status.Errorf(codes.Internal, "getting task: %v", err)
	}
	if task == nil {
		return nil, status.Error(codes.NotFound, "task not found")
	}

	if req.EventType == db.TaskEventStatus {
		if !db.ValidTaskStatus(req.Status) {
			return nil, status.Error(codes.InvalidArgument, "status events require a valid task status")
		}
		task.Status = req.Status
		if err := s.tasks.UpdateTask(task); err != nil {
			s.logger.Error("updating task status from event", "id", req.TaskId, "error", err)
			return nil, status.Errorf(codes.Internal, "updating task status: %v", err)
		}
	}

	event := &db.TaskEvent{
		TaskID:       req.TaskId,
		EventType:    req.EventType,
		ActorRole:    req.ActorRole,
		ActorName:    req.ActorName,
		ActorUser:    req.ActorUser,
		ActorMachine: req.ActorMachine,
		ActorAgent:   req.ActorAgent,
		Summary:      req.Summary,
		Content:      req.Content,
		Status:       req.Status,
		MemoryID:     req.MemoryId,
		Source:       req.Source,
		Metadata:     req.Metadata,
	}

	saved, err := s.tasks.CreateTaskEvent(event)
	if err != nil {
		s.logger.Error("creating task event", "task_id", req.TaskId, "error", err)
		return nil, status.Errorf(codes.Internal, "creating task event: %v", err)
	}

	return &pb.CreateTaskEventResponse{Event: taskEventToProto(saved)}, nil
}

func (s *Server) ListTaskEvents(_ context.Context, req *pb.ListTaskEventsRequest) (*pb.ListTaskEventsResponse, error) {
	if s.tasks == nil {
		return nil, status.Error(codes.Unimplemented, "task queue not available")
	}
	if req.TaskId == "" {
		return nil, status.Error(codes.InvalidArgument, "task_id is required")
	}

	events, err := s.tasks.ListTaskEvents(req.TaskId, int(req.Limit))
	if err != nil {
		s.logger.Error("listing task events", "task_id", req.TaskId, "error", err)
		return nil, status.Errorf(codes.Internal, "listing task events: %v", err)
	}

	return &pb.ListTaskEventsResponse{Events: taskEventsToProto(events)}, nil
}

// Conversion helpers

func taskToProto(t *db.Task) *pb.Task {
	if t == nil {
		return nil
	}
	return &pb.Task{
		Id:           t.ID,
		Project:      t.Project,
		Queue:        t.Queue,
		Title:        t.Title,
		Summary:      t.Summary,
		Description:  t.Description,
		Status:       t.Status,
		Priority:     t.Priority,
		CreatedBy:    t.CreatedBy,
		Orchestrator: t.Orchestrator,
		Worker:       t.Worker,
		ParentTaskId: t.ParentTaskID,
		Metadata:     t.Metadata,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
		StartedAt:    t.StartedAt,
		CompletedAt:  t.CompletedAt,
		FailedAt:     t.FailedAt,
		BlockedAt:    t.BlockedAt,
	}
}

func tasksToProto(tasks []*db.Task) []*pb.Task {
	out := make([]*pb.Task, len(tasks))
	for i, t := range tasks {
		out[i] = taskToProto(t)
	}
	return out
}

func taskEventToProto(e *db.TaskEvent) *pb.TaskEvent {
	if e == nil {
		return nil
	}
	return &pb.TaskEvent{
		Id:           e.ID,
		TaskId:       e.TaskID,
		EventType:    e.EventType,
		ActorRole:    e.ActorRole,
		ActorName:    e.ActorName,
		ActorUser:    e.ActorUser,
		ActorMachine: e.ActorMachine,
		ActorAgent:   e.ActorAgent,
		Summary:      e.Summary,
		Content:      e.Content,
		Status:       e.Status,
		MemoryId:     e.MemoryID,
		Source:       e.Source,
		Metadata:     e.Metadata,
		CreatedAt:    e.CreatedAt,
	}
}

func taskEventsToProto(events []*db.TaskEvent) []*pb.TaskEvent {
	out := make([]*pb.TaskEvent, len(events))
	for i, e := range events {
		out[i] = taskEventToProto(e)
	}
	return out
}
