package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/j33pguy/magi/internal/db"
	"github.com/mark3labs/mcp-go/mcp"
)

// CreateTask creates a task in the separate task queue.
type CreateTask struct {
	Tasks          db.TaskQueueStore
	DefaultProject string
}

func (t *CreateTask) Tool() mcp.Tool {
	return mcp.NewTool("create_task",
		mcp.WithDescription("Create a task in the shared task queue. Use this for work tracking between orchestrators and workers instead of storing tasks as memories."),
		mcp.WithString("title", mcp.Required(), mcp.Description("Short task title")),
		mcp.WithString("project", mcp.Description("Project name/path (auto-detected from PWD if omitted)")),
		mcp.WithString("queue", mcp.Description("Queue name (default: default)")),
		mcp.WithString("summary", mcp.Description("Short summary of the task")),
		mcp.WithString("description", mcp.Description("Detailed task description or plan")),
		mcp.WithString("status", mcp.Description("Initial task status"), mcp.Enum(db.TaskStatusQueued, db.TaskStatusStarted, db.TaskStatusDone, db.TaskStatusFailed, db.TaskStatusBlocked, db.TaskStatusCanceled)),
		mcp.WithString("priority", mcp.Description("Task priority"), mcp.Enum(db.TaskPriorityLow, db.TaskPriorityNormal, db.TaskPriorityHigh, db.TaskPriorityUrgent)),
		mcp.WithString("created_by", mcp.Description("Human or service that created the task")),
		mcp.WithString("orchestrator", mcp.Description("Orchestrator agent/service assigned to the task")),
		mcp.WithString("worker", mcp.Description("Worker agent/service assigned to the task")),
		mcp.WithString("parent_task_id", mcp.Description("Optional parent task ID")),
		mcp.WithString("actor_role", mcp.Description("Role for the initial task event (e.g. orchestrator, worker, system)")),
		mcp.WithString("actor_name", mcp.Description("Display name for the initial task event actor")),
		mcp.WithString("actor_agent", mcp.Description("Agent identifier for the initial task event actor")),
		mcp.WithString("metadata_json", mcp.Description("Optional JSON object with task metadata")),
	)
}

func (t *CreateTask) Handle(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title, err := request.RequireString("title")
	if err != nil {
		return mcp.NewToolResultError("title is required"), nil
	}
	metadata, err := parseTaskMetadataArg(request.GetString("metadata_json", ""))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid metadata_json: %v", err)), nil
	}

	project := request.GetString("project", "")
	if project == "" && t.DefaultProject != "" {
		project = t.DefaultProject
	}
	task := &db.Task{
		Project:      project,
		Queue:        request.GetString("queue", ""),
		Title:        title,
		Summary:      request.GetString("summary", ""),
		Description:  request.GetString("description", ""),
		Status:       request.GetString("status", db.TaskStatusQueued),
		Priority:     request.GetString("priority", db.TaskPriorityNormal),
		CreatedBy:    request.GetString("created_by", ""),
		Orchestrator: request.GetString("orchestrator", ""),
		Worker:       request.GetString("worker", ""),
		ParentTaskID: request.GetString("parent_task_id", ""),
		Metadata:     metadata,
	}
	if !db.ValidTaskStatus(task.Status) {
		return mcp.NewToolResultError(fmt.Sprintf("invalid status %q", task.Status)), nil
	}
	if !db.ValidTaskPriority(task.Priority) {
		return mcp.NewToolResultError(fmt.Sprintf("invalid priority %q", task.Priority)), nil
	}

	created, err := t.Tasks.CreateTask(task)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("creating task: %v", err)), nil
	}
	event := &db.TaskEvent{
		TaskID:     created.ID,
		EventType:  db.TaskEventStatus,
		ActorRole:  request.GetString("actor_role", "orchestrator"),
		ActorName:  request.GetString("actor_name", created.Orchestrator),
		ActorAgent: request.GetString("actor_agent", created.Orchestrator),
		Summary:    "Task created",
		Content:    created.Description,
		Status:     created.Status,
		Source:     "mcp",
		Metadata:   metadata,
	}
	if _, err := t.Tasks.CreateTaskEvent(event); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("recording initial task event: %v", err)), nil
	}
	return marshalTaskResult(created)
}

// ListTasks lists tasks from the separate queue.
type ListTasks struct {
	Tasks          db.TaskQueueStore
	DefaultProject string
}

func (l *ListTasks) Tool() mcp.Tool {
	return mcp.NewTool("list_tasks",
		mcp.WithDescription("List tasks from the shared task queue so agents can see each other's progress."),
		mcp.WithString("project", mcp.Description("Filter by project name")),
		mcp.WithString("queue", mcp.Description("Filter by queue name")),
		mcp.WithString("status", mcp.Description("Filter by status"), mcp.Enum(db.TaskStatusQueued, db.TaskStatusStarted, db.TaskStatusDone, db.TaskStatusFailed, db.TaskStatusBlocked, db.TaskStatusCanceled)),
		mcp.WithString("worker", mcp.Description("Filter by worker name")),
		mcp.WithString("orchestrator", mcp.Description("Filter by orchestrator name")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 25)")),
	)
}

func (l *ListTasks) Handle(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project := request.GetString("project", "")
	if project == "" && l.DefaultProject != "" {
		project = l.DefaultProject
	}
	filter := &db.TaskFilter{
		Project:      project,
		Queue:        request.GetString("queue", ""),
		Status:       request.GetString("status", ""),
		Worker:       request.GetString("worker", ""),
		Orchestrator: request.GetString("orchestrator", ""),
		Limit:        request.GetInt("limit", 25),
	}
	if filter.Status != "" && !db.ValidTaskStatus(filter.Status) {
		return mcp.NewToolResultError(fmt.Sprintf("invalid status %q", filter.Status)), nil
	}
	tasks, err := l.Tasks.ListTasks(filter)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("listing tasks: %v", err)), nil
	}
	if len(tasks) == 0 {
		return mcp.NewToolResultText("No tasks found."), nil
	}
	return marshalTaskResult(tasks)
}

// GetTask fetches a single task by ID.
type GetTask struct {
	Tasks db.TaskQueueStore
}

func (g *GetTask) Tool() mcp.Tool {
	return mcp.NewTool("get_task",
		mcp.WithDescription("Get a single task from the shared task queue."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Task ID")),
	)
}

func (g *GetTask) Handle(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := request.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("id is required"), nil
	}
	task, err := g.Tasks.GetTask(id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("getting task: %v", err)), nil
	}
	if task == nil {
		return mcp.NewToolResultError("task not found"), nil
	}
	return marshalTaskResult(task)
}

// UpdateTask updates an existing task in the separate queue.
type UpdateTask struct {
	Tasks db.TaskQueueStore
}

func (u *UpdateTask) Tool() mcp.Tool {
	return mcp.NewTool("update_task",
		mcp.WithDescription("Update task status, assignment, or details in the shared task queue."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Task ID")),
		mcp.WithString("project", mcp.Description("Project name")),
		mcp.WithString("queue", mcp.Description("Queue name")),
		mcp.WithString("title", mcp.Description("Task title")),
		mcp.WithString("summary", mcp.Description("Task summary")),
		mcp.WithString("description", mcp.Description("Task description")),
		mcp.WithString("status", mcp.Description("Task status"), mcp.Enum(db.TaskStatusQueued, db.TaskStatusStarted, db.TaskStatusDone, db.TaskStatusFailed, db.TaskStatusBlocked, db.TaskStatusCanceled)),
		mcp.WithString("priority", mcp.Description("Task priority"), mcp.Enum(db.TaskPriorityLow, db.TaskPriorityNormal, db.TaskPriorityHigh, db.TaskPriorityUrgent)),
		mcp.WithString("created_by", mcp.Description("Task creator")),
		mcp.WithString("orchestrator", mcp.Description("Orchestrator assignment")),
		mcp.WithString("worker", mcp.Description("Worker assignment")),
		mcp.WithString("parent_task_id", mcp.Description("Parent task ID")),
		mcp.WithString("status_summary", mcp.Description("Optional summary for the generated status event")),
		mcp.WithString("metadata_json", mcp.Description("Optional JSON object with task metadata")),
	)
}

func (u *UpdateTask) Handle(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := request.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("id is required"), nil
	}
	task, err := u.Tasks.GetTask(id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("getting task: %v", err)), nil
	}
	if task == nil {
		return mcp.NewToolResultError("task not found"), nil
	}
	args := request.GetArguments()
	previousStatus := task.Status
	if value, ok := stringArg(args, "project"); ok {
		task.Project = value
	}
	if value, ok := stringArg(args, "queue"); ok {
		task.Queue = value
	}
	if value, ok := stringArg(args, "title"); ok {
		task.Title = value
	}
	if value, ok := stringArg(args, "summary"); ok {
		task.Summary = value
	}
	if value, ok := stringArg(args, "description"); ok {
		task.Description = value
	}
	if value, ok := stringArg(args, "status"); ok {
		if !db.ValidTaskStatus(value) {
			return mcp.NewToolResultError(fmt.Sprintf("invalid status %q", value)), nil
		}
		task.Status = value
	}
	if value, ok := stringArg(args, "priority"); ok {
		if !db.ValidTaskPriority(value) {
			return mcp.NewToolResultError(fmt.Sprintf("invalid priority %q", value)), nil
		}
		task.Priority = value
	}
	if value, ok := stringArg(args, "created_by"); ok {
		task.CreatedBy = value
	}
	if value, ok := stringArg(args, "orchestrator"); ok {
		task.Orchestrator = value
	}
	if value, ok := stringArg(args, "worker"); ok {
		task.Worker = value
	}
	if value, ok := stringArg(args, "parent_task_id"); ok {
		task.ParentTaskID = value
	}
	if rawMetadata, ok := stringArg(args, "metadata_json"); ok {
		metadata, err := parseTaskMetadataArg(rawMetadata)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid metadata_json: %v", err)), nil
		}
		task.Metadata = metadata
	}
	if err := u.Tasks.UpdateTask(task); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("updating task: %v", err)), nil
	}
	if task.Status != previousStatus {
		event := &db.TaskEvent{
			TaskID:     task.ID,
			EventType:  db.TaskEventStatus,
			ActorRole:  "orchestrator",
			ActorName:  task.Orchestrator,
			ActorAgent: task.Orchestrator,
			Summary:    request.GetString("status_summary", "Task status updated"),
			Status:     task.Status,
			Source:     "mcp",
		}
		if _, err := u.Tasks.CreateTaskEvent(event); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("recording status event: %v", err)), nil
		}
	}
	return marshalTaskResult(task)
}

// AddTaskEvent appends structured activity to a task.
type AddTaskEvent struct {
	Tasks db.TaskQueueStore
	DB    db.Store
}

func (a *AddTaskEvent) Tool() mcp.Tool {
	return mcp.NewTool("add_task_event",
		mcp.WithDescription("Append comms, issues, lessons, pitfalls, successes, or memory references to a task."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID")),
		mcp.WithString("event_type", mcp.Required(), mcp.Description("Event type"), mcp.Enum(db.TaskEventStatus, db.TaskEventCommunication, db.TaskEventIssue, db.TaskEventLesson, db.TaskEventPitfall, db.TaskEventSuccess, db.TaskEventMemoryRef, db.TaskEventNote)),
		mcp.WithString("actor_role", mcp.Description("Actor role (e.g. orchestrator, worker, system)")),
		mcp.WithString("actor_name", mcp.Description("Display name of the actor")),
		mcp.WithString("actor_user", mcp.Description("Authenticated user or owner")),
		mcp.WithString("actor_machine", mcp.Description("Machine identifier")),
		mcp.WithString("actor_agent", mcp.Description("Agent identifier")),
		mcp.WithString("summary", mcp.Description("Short summary of this event")),
		mcp.WithString("content", mcp.Description("Detailed content or communication text")),
		mcp.WithString("status", mcp.Description("Updated task status for status events"), mcp.Enum(db.TaskStatusQueued, db.TaskStatusStarted, db.TaskStatusDone, db.TaskStatusFailed, db.TaskStatusBlocked, db.TaskStatusCanceled)),
		mcp.WithString("memory_id", mcp.Description("Optional linked memory ID")),
		mcp.WithString("source", mcp.Description("Source system for the event")),
		mcp.WithString("metadata_json", mcp.Description("Optional JSON object with event metadata")),
	)
}

func (a *AddTaskEvent) Handle(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID, err := request.RequireString("task_id")
	if err != nil {
		return mcp.NewToolResultError("task_id is required"), nil
	}
	eventType, err := request.RequireString("event_type")
	if err != nil {
		return mcp.NewToolResultError("event_type is required"), nil
	}
	if !db.ValidTaskEventType(eventType) {
		return mcp.NewToolResultError(fmt.Sprintf("invalid event_type %q", eventType)), nil
	}
	task, err := a.Tasks.GetTask(taskID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("getting task: %v", err)), nil
	}
	if task == nil {
		return mcp.NewToolResultError("task not found"), nil
	}

	status := request.GetString("status", "")
	if status != "" && !db.ValidTaskStatus(status) {
		return mcp.NewToolResultError(fmt.Sprintf("invalid status %q", status)), nil
	}
	if db.NormalizeTaskEventType(eventType) == db.TaskEventStatus && strings.TrimSpace(status) == "" {
		return mcp.NewToolResultError("status is required for status events"), nil
	}

	memoryID := request.GetString("memory_id", "")
	if memoryID != "" && a.DB != nil {
		if _, err := a.DB.GetMemory(memoryID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("memory not found: %v", err)), nil
		}
	}

	metadata, err := parseTaskMetadataArg(request.GetString("metadata_json", ""))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid metadata_json: %v", err)), nil
	}
	event := &db.TaskEvent{
		TaskID:       taskID,
		EventType:    eventType,
		ActorRole:    request.GetString("actor_role", ""),
		ActorName:    request.GetString("actor_name", ""),
		ActorUser:    request.GetString("actor_user", ""),
		ActorMachine: request.GetString("actor_machine", ""),
		ActorAgent:   request.GetString("actor_agent", ""),
		Summary:      request.GetString("summary", ""),
		Content:      request.GetString("content", ""),
		Status:       status,
		MemoryID:     memoryID,
		Source:       request.GetString("source", "mcp"),
		Metadata:     metadata,
	}
	created, err := a.Tasks.CreateTaskEvent(event)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("creating task event: %v", err)), nil
	}
	if created.Status != "" {
		task.Status = created.Status
		if err := a.Tasks.UpdateTask(task); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("updating task status: %v", err)), nil
		}
	}
	return marshalTaskResult(created)
}

// ListTaskEvents lists activity for a task.
type ListTaskEvents struct {
	Tasks db.TaskQueueStore
}

func (l *ListTaskEvents) Tool() mcp.Tool {
	return mcp.NewTool("list_task_events",
		mcp.WithDescription("List the activity log for a task, including comms, issues, lessons, pitfalls, successes, and linked memories."),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 100)")),
	)
}

func (l *ListTaskEvents) Handle(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID, err := request.RequireString("task_id")
	if err != nil {
		return mcp.NewToolResultError("task_id is required"), nil
	}
	events, err := l.Tasks.ListTaskEvents(taskID, request.GetInt("limit", 100))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("listing task events: %v", err)), nil
	}
	if len(events) == 0 {
		return mcp.NewToolResultText("No task events found."), nil
	}
	return marshalTaskResult(events)
}

func parseTaskMetadataArg(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var metadata map[string]string
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return nil, err
	}
	return metadata, nil
}

func marshalTaskResult(value any) (*mcp.CallToolResult, error) {
	output, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(output)), nil
}

func stringArg(args map[string]any, key string) (string, bool) {
	value, ok := args[key]
	if !ok {
		return "", false
	}
	switch typed := value.(type) {
	case string:
		return typed, true
	default:
		return fmt.Sprintf("%v", value), true
	}
}
