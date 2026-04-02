package db

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestTaskQueueLifecycleSQLite(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client, err := NewSQLiteClient(filepath.Join(t.TempDir(), "tasks.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	defer client.Close()

	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	task, err := client.TursoClient.CreateTask(&Task{
		Project:      "proj",
		Queue:        "agents",
		Title:        "Implement task queue",
		Status:       TaskStatusQueued,
		Priority:     TaskPriorityHigh,
		Orchestrator: "claude-main",
		Worker:       "codex-worker",
		Metadata:     map[string]string{"epic": "tasks"},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.ID == "" {
		t.Fatal("expected task ID")
	}

	got, err := client.TursoClient.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got == nil || got.Title != task.Title || got.Metadata["epic"] != "tasks" {
		t.Fatalf("unexpected task: %+v", got)
	}

	got.Status = TaskStatusStarted
	if err := client.TursoClient.UpdateTask(got); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	list, err := client.TursoClient.ListTasks(&TaskFilter{Project: "proj", Status: TaskStatusStarted})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(list) != 1 || list[0].ID != task.ID {
		t.Fatalf("unexpected task list: %+v", list)
	}

	event, err := client.TursoClient.CreateTaskEvent(&TaskEvent{
		TaskID:    task.ID,
		EventType: TaskEventCommunication,
		ActorRole: "worker",
		ActorName: "codex-worker",
		Summary:   "worker update",
		Content:   "task queue API is wired",
	})
	if err != nil {
		t.Fatalf("CreateTaskEvent: %v", err)
	}
	if event.ID == "" {
		t.Fatal("expected task event ID")
	}

	events, err := client.TursoClient.ListTaskEvents(task.ID, 10)
	if err != nil {
		t.Fatalf("ListTaskEvents: %v", err)
	}
	if len(events) != 1 || events[0].Summary != "worker update" {
		t.Fatalf("unexpected task events: %+v", events)
	}
}
