package db

import (
	"database/sql"
	"fmt"
	"strings"
)

func (c *SQLServerClient) CreateTask(task *Task) (*Task, error) {
	prepareTaskForWrite(task)
	now := nowUTCString()
	task.CreatedAt = now
	applyTaskTimestamps(task, now)

	err := c.db.QueryRow(`
		INSERT INTO tasks (project, queue_name, title, summary, description, status, priority, created_by, orchestrator, worker, parent_task_id, metadata_json, created_at, updated_at, started_at, completed_at, failed_at, blocked_at)
		OUTPUT INSERTED.id
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10, @p11, @p12, @p13, @p14, @p15, @p16, @p17, @p18)
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

func (c *SQLServerClient) GetTask(id string) (*Task, error) {
	row := c.db.QueryRow(`
		SELECT id, project, queue_name, title, summary, description, status, priority, created_by, orchestrator, worker,
		       parent_task_id, metadata_json, created_at, updated_at, started_at, completed_at, failed_at, blocked_at
		FROM tasks
		WHERE id = @p1
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

func (c *SQLServerClient) UpdateTask(task *Task) error {
	prepareTaskForWrite(task)
	now := nowUTCString()
	applyTaskTimestamps(task, now)
	_, err := c.db.Exec(`
		UPDATE tasks
		SET project = @p1, queue_name = @p2, title = @p3, summary = @p4, description = @p5, status = @p6, priority = @p7,
		    created_by = @p8, orchestrator = @p9, worker = @p10, parent_task_id = @p11, metadata_json = @p12, updated_at = @p13,
		    started_at = @p14, completed_at = @p15, failed_at = @p16, blocked_at = @p17
		WHERE id = @p18
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

func (c *SQLServerClient) ListTasks(filter *TaskFilter) ([]*Task, error) {
	if filter == nil {
		filter = &TaskFilter{}
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	conditions := []string{"1=1"}
	args := []any{}
	add := func(cond string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(cond, len(args)))
	}
	if filter.Project != "" {
		add("project = @p%d", filter.Project)
	}
	if filter.Queue != "" {
		add("queue_name = @p%d", filter.Queue)
	}
	if filter.Status != "" {
		add("status = @p%d", NormalizeTaskStatus(filter.Status))
	}
	if filter.Worker != "" {
		add("worker = @p%d", filter.Worker)
	}
	if filter.Orchestrator != "" {
		add("orchestrator = @p%d", filter.Orchestrator)
	}
	args = append(args, limit)
	limitPH := fmt.Sprintf("@p%d", len(args))

	rows, err := c.db.Query(fmt.Sprintf(`
		SELECT TOP (%s)
		       id, project, queue_name, title, summary, description, status, priority, created_by, orchestrator, worker,
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
	`, limitPH, strings.Join(conditions, " AND ")), args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	return scanTaskRows(rows)
}

func (c *SQLServerClient) CreateTaskEvent(event *TaskEvent) (*TaskEvent, error) {
	event.EventType = NormalizeTaskEventType(event.EventType)
	event.CreatedAt = nowUTCString()
	err := c.db.QueryRow(`
		INSERT INTO task_events (task_id, event_type, actor_role, actor_name, actor_user, actor_machine, actor_agent, summary, content, status, memory_id, source, metadata_json, created_at)
		OUTPUT INSERTED.id
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10, @p11, @p12, @p13, @p14)
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

func (c *SQLServerClient) ListTaskEvents(taskID string, limit int) ([]*TaskEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := c.db.Query(`
		SELECT TOP (@p2)
		       id, task_id, event_type, actor_role, actor_name, actor_user, actor_machine, actor_agent,
		       summary, content, status, memory_id, source, metadata_json, created_at
		FROM task_events
		WHERE task_id = @p1
		ORDER BY created_at ASC
	`, taskID, limit)
	if err != nil {
		return nil, fmt.Errorf("list task events: %w", err)
	}
	defer rows.Close()
	return scanTaskEventRows(rows)
}
