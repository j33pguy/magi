package db

import (
	"database/sql"
	"fmt"
	"strings"
)

func (c *PostgresClient) CreateTask(task *Task) (*Task, error) {
	prepareTaskForWrite(task)
	now := nowUTCString()
	task.CreatedAt = now
	applyTaskTimestamps(task, now)

	err := c.DB.QueryRow(`
		INSERT INTO tasks (project, queue_name, title, summary, description, status, priority, created_by, orchestrator, worker, parent_task_id, metadata_json, created_at, updated_at, started_at, completed_at, failed_at, blocked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		RETURNING id
	`,
		task.Project, task.Queue, task.Title, task.Summary, task.Description, task.Status, task.Priority,
		task.CreatedBy, task.Orchestrator, task.Worker, nullString(task.ParentTaskID),
		marshalTaskMetadata(task.Metadata), task.CreatedAt, task.UpdatedAt,
		nullString(task.StartedAt), nullString(task.CompletedAt), nullString(task.FailedAt), nullString(task.BlockedAt),
	).Scan(&task.ID)
	if err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}
	return task, nil
}

func (c *PostgresClient) GetTask(id string) (*Task, error) {
	row := c.DB.QueryRow(`
		SELECT id, project, queue_name, title, summary, description, status, priority, created_by, orchestrator, worker,
		       parent_task_id, metadata_json, created_at, updated_at, started_at, completed_at, failed_at, blocked_at
		FROM tasks
		WHERE id = $1
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

func (c *PostgresClient) UpdateTask(task *Task) error {
	prepareTaskForWrite(task)
	now := nowUTCString()
	applyTaskTimestamps(task, now)
	_, err := c.DB.Exec(`
		UPDATE tasks
		SET project = $1, queue_name = $2, title = $3, summary = $4, description = $5, status = $6, priority = $7,
		    created_by = $8, orchestrator = $9, worker = $10, parent_task_id = $11, metadata_json = $12, updated_at = $13,
		    started_at = $14, completed_at = $15, failed_at = $16, blocked_at = $17
		WHERE id = $18
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

func (c *PostgresClient) ListTasks(filter *TaskFilter) ([]*Task, error) {
	if filter == nil {
		filter = &TaskFilter{}
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	var conditions []string
	var args []any
	add := func(cond string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(cond, len(args)))
	}
	if filter.Project != "" {
		add("project = $%d", filter.Project)
	}
	if filter.Queue != "" {
		add("queue_name = $%d", filter.Queue)
	}
	if filter.Status != "" {
		add("status = $%d", NormalizeTaskStatus(filter.Status))
	}
	if filter.Worker != "" {
		add("worker = $%d", filter.Worker)
	}
	if filter.Orchestrator != "" {
		add("orchestrator = $%d", filter.Orchestrator)
	}
	where := "TRUE"
	if len(conditions) > 0 {
		where = strings.Join(conditions, " AND ")
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
		LIMIT $%d
	`, where, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	return scanTaskRows(rows)
}

func (c *PostgresClient) CreateTaskEvent(event *TaskEvent) (*TaskEvent, error) {
	event.EventType = NormalizeTaskEventType(event.EventType)
	event.CreatedAt = nowUTCString()
	err := c.DB.QueryRow(`
		INSERT INTO task_events (task_id, event_type, actor_role, actor_name, actor_user, actor_machine, actor_agent, summary, content, status, memory_id, source, metadata_json, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id
	`,
		event.TaskID, event.EventType, event.ActorRole, event.ActorName, event.ActorUser,
		event.ActorMachine, event.ActorAgent, event.Summary, event.Content,
		event.Status, nullString(event.MemoryID), event.Source, marshalTaskMetadata(event.Metadata), event.CreatedAt,
	).Scan(&event.ID)
	if err != nil {
		return nil, fmt.Errorf("insert task event: %w", err)
	}
	return event, nil
}

func (c *PostgresClient) ListTaskEvents(taskID string, limit int) ([]*TaskEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := c.DB.Query(`
		SELECT id, task_id, event_type, actor_role, actor_name, actor_user, actor_machine, actor_agent,
		       summary, content, status, memory_id, source, metadata_json, created_at
		FROM task_events
		WHERE task_id = $1
		ORDER BY created_at ASC
		LIMIT $2
	`, taskID, limit)
	if err != nil {
		return nil, fmt.Errorf("list task events: %w", err)
	}
	defer rows.Close()
	return scanTaskEventRows(rows)
}
