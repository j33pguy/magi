package db

import (
	"testing"
)

func TestCreateTask(t *testing.T) {
	c := setupOrchestrationDB(t)
	task, err := c.CreateTask(&Task{
		Description: "Research and build adapter",
		CreatedBy:   "orchestrator",
		Subtasks: []*Subtask{
			{Description: "Research API changes", AgentID: "grok"},
			{Description: "Design adapter", AgentID: "claude"},
			{Description: "Implement adapter", AgentID: "codex"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.ID == "" {
		t.Error("expected task ID")
	}
	if task.Status != "created" {
		t.Errorf("expected created, got %s", task.Status)
	}
	if len(task.Subtasks) != 3 {
		t.Errorf("expected 3 subtasks, got %d", len(task.Subtasks))
	}
}

func TestCreateTask_NoSubtasks(t *testing.T) {
	c := setupOrchestrationDB(t)
	task, err := c.CreateTask(&Task{Description: "Simple task"})
	if err != nil {
		t.Fatal(err)
	}
	if task.ID == "" {
		t.Error("expected task ID")
	}
}

func TestGetTask(t *testing.T) {
	c := setupOrchestrationDB(t)
	created, _ := c.CreateTask(&Task{
		Description: "Test task",
		Subtasks: []*Subtask{
			{Description: "Sub 1"},
			{Description: "Sub 2"},
		},
	})
	task, err := c.GetTask(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Description != "Test task" {
		t.Errorf("expected 'Test task', got %s", task.Description)
	}
	if len(task.Subtasks) != 2 {
		t.Errorf("expected 2 subtasks, got %d", len(task.Subtasks))
	}
	if task.Subtasks[0].SortOrder != 0 || task.Subtasks[1].SortOrder != 1 {
		t.Error("subtask sort order incorrect")
	}
}

func TestGetTask_NotFound(t *testing.T) {
	c := setupOrchestrationDB(t)
	_, err := c.GetTask("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestReportProgress(t *testing.T) {
	c := setupOrchestrationDB(t)
	task, _ := c.CreateTask(&Task{
		Description: "Progress test",
		Subtasks:    []*Subtask{{Description: "Work item"}},
	})
	subID := task.Subtasks[0].ID

	err := c.ReportProgress(subID, 50, "Halfway done")
	if err != nil {
		t.Fatal(err)
	}

	updated, _ := c.GetTask(task.ID)
	if updated.Subtasks[0].ProgressPercent != 50 {
		t.Errorf("expected 50%%, got %d%%", updated.Subtasks[0].ProgressPercent)
	}
	if updated.Subtasks[0].Status != "in_progress" {
		t.Errorf("expected in_progress, got %s", updated.Subtasks[0].Status)
	}
}

func TestUpdateSubtaskStatus(t *testing.T) {
	c := setupOrchestrationDB(t)
	task, _ := c.CreateTask(&Task{
		Description: "Status test",
		Subtasks:    []*Subtask{{Description: "Work"}},
	})
	subID := task.Subtasks[0].ID

	err := c.UpdateSubtaskStatus(subID, "complete", "All done")
	if err != nil {
		t.Fatal(err)
	}

	updated, _ := c.GetTask(task.ID)
	if updated.Subtasks[0].Status != "complete" {
		t.Errorf("expected complete, got %s", updated.Subtasks[0].Status)
	}
	if updated.Subtasks[0].Output != "All done" {
		t.Errorf("expected 'All done', got %s", updated.Subtasks[0].Output)
	}
}

func TestAssignSubtask(t *testing.T) {
	c := setupOrchestrationDB(t)
	task, _ := c.CreateTask(&Task{
		Description: "Assign test",
		Subtasks:    []*Subtask{{Description: "Unassigned work"}},
	})
	subID := task.Subtasks[0].ID

	err := c.AssignSubtask(subID, "grok")
	if err != nil {
		t.Fatal(err)
	}

	updated, _ := c.GetTask(task.ID)
	if updated.Subtasks[0].AgentID != "grok" {
		t.Errorf("expected grok, got %s", updated.Subtasks[0].AgentID)
	}
	if updated.Subtasks[0].Status != "assigned" {
		t.Errorf("expected assigned, got %s", updated.Subtasks[0].Status)
	}
}

func TestListTasks(t *testing.T) {
	c := setupOrchestrationDB(t)
	c.CreateTask(&Task{Description: "Task 1"})
	c.CreateTask(&Task{Description: "Task 2"})

	tasks, err := c.ListTasks("")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestListTasks_FilterStatus(t *testing.T) {
	c := setupOrchestrationDB(t)
	c.CreateTask(&Task{Description: "Created task"})
	task2, _ := c.CreateTask(&Task{
		Description: "Completed task",
		Subtasks:    []*Subtask{{Description: "Done"}},
	})
	c.UpdateSubtaskStatus(task2.Subtasks[0].ID, "complete", "")

	created, _ := c.ListTasks("created")
	if len(created) < 1 {
		t.Error("expected at least 1 created task")
	}
}

func TestFullTaskLifecycle(t *testing.T) {
	c := setupOrchestrationDB(t)
	c.RegisterAgent(&Agent{ID: "grok", Name: "Grok", Capabilities: []string{"research"}})
	c.RegisterAgent(&Agent{ID: "claude", Name: "Claude", Capabilities: []string{"architecture"}})

	task, _ := c.CreateTask(&Task{
		Description: "Full lifecycle",
		CreatedBy:   "orchestrator",
		Subtasks: []*Subtask{
			{Description: "Research"},
			{Description: "Design"},
		},
	})

	c.AssignSubtask(task.Subtasks[0].ID, "grok")
	c.ReportProgress(task.Subtasks[0].ID, 50, "Found 3 issues")
	c.ReportProgress(task.Subtasks[0].ID, 100, "Research complete")
	c.UpdateSubtaskStatus(task.Subtasks[0].ID, "complete", "3 breaking changes documented")

	c.AssignSubtask(task.Subtasks[1].ID, "claude")
	c.ReportProgress(task.Subtasks[1].ID, 100, "Adapter pattern designed")
	c.UpdateSubtaskStatus(task.Subtasks[1].ID, "complete", "Architecture doc ready")

	final, _ := c.GetTask(task.ID)
	for _, sub := range final.Subtasks {
		if sub.Status != "complete" {
			t.Errorf("subtask %s not complete: %s", sub.Description, sub.Status)
		}
	}
}
