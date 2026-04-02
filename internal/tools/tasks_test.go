package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/j33pguy/magi/internal/db"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestTaskQueueToolsLifecycle(t *testing.T) {
	dbClient := newTestDB(t)

	create := &CreateTask{Tasks: dbClient, DefaultProject: "proj"}
	createResult, err := create.Handle(context.Background(), makeRequest(map[string]any{
		"title":        "Implement queue-backed tasks",
		"summary":      "Separate tasks from the main memory stack",
		"worker":       "codex-worker",
		"orchestrator": "claude-orchestrator",
		"priority":     db.TaskPriorityHigh,
		"metadata_json": `{
			"epic":"launch",
			"scope":"tasks"
		}`,
	}))
	if err != nil {
		t.Fatalf("CreateTask.Handle: %v", err)
	}
	if createResult.IsError {
		t.Fatalf("unexpected create error: %v", createResult.Content)
	}

	var created db.Task
	if err := json.Unmarshal([]byte(createResult.Content[0].(mcp.TextContent).Text), &created); err != nil {
		t.Fatalf("unmarshal created task: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected task id")
	}
	if created.Project != "proj" {
		t.Fatalf("project = %q, want proj", created.Project)
	}
	if created.Status != db.TaskStatusQueued {
		t.Fatalf("status = %q, want queued", created.Status)
	}

	get := &GetTask{Tasks: dbClient}
	getResult, err := get.Handle(context.Background(), makeRequest(map[string]any{"id": created.ID}))
	if err != nil {
		t.Fatalf("GetTask.Handle: %v", err)
	}
	if getResult.IsError {
		t.Fatalf("unexpected get error: %v", getResult.Content)
	}

	update := &UpdateTask{Tasks: dbClient}
	updateResult, err := update.Handle(context.Background(), makeRequest(map[string]any{
		"id":             created.ID,
		"status":         db.TaskStatusStarted,
		"status_summary": "Worker picked this up",
		"worker":         "codex-worker-2",
	}))
	if err != nil {
		t.Fatalf("UpdateTask.Handle: %v", err)
	}
	if updateResult.IsError {
		t.Fatalf("unexpected update error: %v", updateResult.Content)
	}

	var updated db.Task
	if err := json.Unmarshal([]byte(updateResult.Content[0].(mcp.TextContent).Text), &updated); err != nil {
		t.Fatalf("unmarshal updated task: %v", err)
	}
	if updated.Status != db.TaskStatusStarted {
		t.Fatalf("updated status = %q, want started", updated.Status)
	}
	if updated.Worker != "codex-worker-2" {
		t.Fatalf("updated worker = %q", updated.Worker)
	}

	linkedMemory := seedTestMemory(t, dbClient, "Remembered pitfall for queue-backed tasks", "proj", "lesson")
	addEvent := &AddTaskEvent{Tasks: dbClient, DB: dbClient}
	eventResult, err := addEvent.Handle(context.Background(), makeRequest(map[string]any{
		"task_id":     created.ID,
		"event_type":  db.TaskEventMemoryRef,
		"summary":     "Linked lesson",
		"memory_id":   linkedMemory.ID,
		"actor_role":  "worker",
		"actor_name":  "codex-worker-2",
		"actor_agent": "codex-worker-2",
	}))
	if err != nil {
		t.Fatalf("AddTaskEvent.Handle memory ref: %v", err)
	}
	if eventResult.IsError {
		t.Fatalf("unexpected event error: %v", eventResult.Content)
	}

	commResult, err := addEvent.Handle(context.Background(), makeRequest(map[string]any{
		"task_id":     created.ID,
		"event_type":  db.TaskEventCommunication,
		"summary":     "Worker update",
		"content":     "Queue storage is in place, moving on to MCP tool wiring.",
		"actor_role":  "worker",
		"actor_name":  "codex-worker-2",
		"actor_agent": "codex-worker-2",
	}))
	if err != nil {
		t.Fatalf("AddTaskEvent.Handle communication: %v", err)
	}
	if commResult.IsError {
		t.Fatalf("unexpected communication error: %v", commResult.Content)
	}

	list := &ListTasks{Tasks: dbClient, DefaultProject: "proj"}
	listResult, err := list.Handle(context.Background(), makeRequest(map[string]any{
		"status": db.TaskStatusStarted,
	}))
	if err != nil {
		t.Fatalf("ListTasks.Handle: %v", err)
	}
	if listResult.IsError {
		t.Fatalf("unexpected list error: %v", listResult.Content)
	}

	var listed []db.Task
	if err := json.Unmarshal([]byte(listResult.Content[0].(mcp.TextContent).Text), &listed); err != nil {
		t.Fatalf("unmarshal listed tasks: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("got %d listed tasks, want 1", len(listed))
	}

	listEvents := &ListTaskEvents{Tasks: dbClient}
	eventsResult, err := listEvents.Handle(context.Background(), makeRequest(map[string]any{
		"task_id": created.ID,
	}))
	if err != nil {
		t.Fatalf("ListTaskEvents.Handle: %v", err)
	}
	if eventsResult.IsError {
		t.Fatalf("unexpected list task events error: %v", eventsResult.Content)
	}

	var events []db.TaskEvent
	if err := json.Unmarshal([]byte(eventsResult.Content[0].(mcp.TextContent).Text), &events); err != nil {
		t.Fatalf("unmarshal task events: %v", err)
	}
	if len(events) < 4 {
		t.Fatalf("got %d task events, want at least 4", len(events))
	}
}

func TestAddTaskEventStatusRequiresStatus(t *testing.T) {
	dbClient := newTestDB(t)
	created, err := dbClient.CreateTask(&db.Task{
		Project:  "proj",
		Title:    "Status event validation",
		Status:   db.TaskStatusQueued,
		Priority: db.TaskPriorityNormal,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	addEvent := &AddTaskEvent{Tasks: dbClient, DB: dbClient}
	result, err := addEvent.Handle(context.Background(), makeRequest(map[string]any{
		"task_id":    created.ID,
		"event_type": db.TaskEventStatus,
		"summary":    "Missing status should fail",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected status event without status to fail")
	}
}
