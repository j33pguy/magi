package db

import (
	"database/sql"
	"fmt"
	"strings"
)

func (c *MySQLClient) CreateTask(task *Task) (*Task, error) {
	prepareTaskForWrite(task)
	now := nowUTCString()
	task.ID = newHexID()
	task.CreatedAt = now
	applyTaskTimestamps(task, now)

	_, err := c.DB.Exec(`
		INSERT INTO tasks (id, project, queue_name, title, summary, description, status, priority, created_by, orchestrator, worker, parent_task_id, metadata_json, created_at, updated_at, started_at, completed_at, failed_at, blocked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		task.ID, task.Project, task.Queue, task.Title, task.Summary, task.Description, task.Status, task.Priority,
		task.CreatedBy, task.Orchestrator, task.Worker, nullString(task.ParentTaskID),
		marshalTaskMetadata(task.Metadata), task.CreatedAt, task.UpdatedAt,
		nullString(task.StartedAt), nullString(task.CompletedAt), nullString(task.FailedAt), nullString(task.BlockedAt),
	)
	if err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}
	return task, nil
}

func (c *MySQLClient) GetTask(id string) (*Task, error) {
	row := c.DB.QueryRow(`
		SELECT id, project, queue_name, title, summary, description, status, priority, created_by, orchestrator, worker,
		       parent_task_id, metadata_json, created_at, updated_at, started_at, completed_at, failed_at, blocked_at
		FROM tasks
		WHERE id = ?
	`, id)
	task, err := parseTaskRow(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	return task, nil
}

func (c *MySQLClient) UpdateTask(task *Task) error {
	prepareTaskForWrite(task)
	now := nowUTCString()
	applyTaskTimestamps(task, now)
	_, err := c.DB.Exec(`
		UPDATE tasks
		SET project = ?, queue_name = ?, title = ?, summary = ?, description = ?, status = ?, priority = ?,
		    created_by = ?, orchestrator = ?, worker = ?, parent_task_id = ?, metadata_json = ?, updated_at = ?,
		    started_at = ?, completed_at = ?, failed_at = ?, blocked_at = ?
		WHERE id = ?
	`,
		task.Project, task.Queue, task.Title, task.Summary, task.Description, task.Status, task.Priority,
		task.CreatedBy, task.Orchestrator, task.Worker, nullString(task.ParentTaskID),
		marshalTaskMetadata(task.Metadata), task.UpdatedAt,
		nullString(task.StartedAt), nullString(task.CompletedAt), nullString(task.FailedAt), nullString(task.BlockedAt),
		task.ID,
	)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	return nil
}

func (c *MySQLClient) ListTasks(filter *TaskFilter) ([]*Task, error) {
	if filter == nil {
		filter = &TaskFilter{}
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	conditions := []string{"1=1"}
	args := []any{}
	if filter.Project != "" {
		conditions = append(conditions, "project = ?")
		args = append(args, filter.Project)
	}
	if filter.Queue != "" {
		conditions = append(conditions, "queue_name = ?")
		args = append(args, filter.Queue)
	}
	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, NormalizeTaskStatus(filter.Status))
	}
	if filter.Worker != "" {
		conditions = append(conditions, "worker = ?")
		args = append(args, filter.Worker)
	}
	if filter.Orchestrator != "" {
		conditions = append(conditions, "orchestrator = ?")
		args = append(args, filter.Orchestrator)
	}
	args = append(args, limit)

	rows, err := c.DB.Query(fmt.Sprintf(`
		SELECT id, project, queue_name, title, summary, description, status, priority, created_by, orchestrator, worker,
		       parent_task_id, metadata_json, created_at, updated_at, started_at, completed_at, failed_at, blocked_at
		FROM tasks
		WHERE %s
		ORDER BY CASE status
			WHEN 'queued' THEN 0
			WHEN 'started' THEN 1
			WHEN 'blocked' THEN 2
			WHEN 'failed' THEN 3
			WHEN 'done' THEN 4
			WHEN 'canceled' THEN 5
			ELSE 6
		END, created_at ASC
		LIMIT ?
	`, strings.Join(conditions, " AND ")), args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	return scanTaskRows(rows)
}

func (c *MySQLClient) CreateTaskEvent(event *TaskEvent) (*TaskEvent, error) {
	event.EventType = NormalizeTaskEventType(event.EventType)
	event.ID = newHexID()
	event.CreatedAt = nowUTCString()
	_, err := c.DB.Exec(`
		INSERT INTO task_events (id, task_id, event_type, actor_role, actor_name, actor_user, actor_machine, actor_agent, summary, content, status, memory_id, source, metadata_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		event.ID, event.TaskID, event.EventType, event.ActorRole, event.ActorName, event.ActorUser,
		event.ActorMachine, event.ActorAgent, event.Summary, event.Content,
		event.Status, nullString(event.MemoryID), event.Source, marshalTaskMetadata(event.Metadata), event.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert task event: %w", err)
	}
	return event, nil
}

func (c *MySQLClient) ListTaskEvents(taskID string, limit int) ([]*TaskEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := c.DB.Query(`
		SELECT id, task_id, event_type, actor_role, actor_name, actor_user, actor_machine, actor_agent,
		       summary, content, status, memory_id, source, metadata_json, created_at
		FROM task_events
		WHERE task_id = ?
		ORDER BY created_at ASC
		LIMIT ?
	`, taskID, limit)
	if err != nil {
		return nil, fmt.Errorf("list task events: %w", err)
	}
	defer rows.Close()
	return scanTaskEventRows(rows)
}
