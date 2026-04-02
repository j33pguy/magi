package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	TaskStatusQueued   = "queued"
	TaskStatusStarted  = "started"
	TaskStatusDone     = "done"
	TaskStatusFailed   = "failed"
	TaskStatusBlocked  = "blocked"
	TaskStatusCanceled = "canceled"
)

const (
	TaskPriorityLow    = "low"
	TaskPriorityNormal = "normal"
	TaskPriorityHigh   = "high"
	TaskPriorityUrgent = "urgent"
)

const (
	TaskEventStatus        = "status"
	TaskEventCommunication = "communication"
	TaskEventIssue         = "issue"
	TaskEventLesson        = "lesson"
	TaskEventPitfall       = "pitfall"
	TaskEventSuccess       = "success"
	TaskEventMemoryRef     = "memory_ref"
	TaskEventNote          = "note"
)

// Task is a queue item separate from the main memory stack.
// It tracks shared progress across orchestrators and workers.
type Task struct {
	ID           string            `json:"id"`
	Project      string            `json:"project"`
	Queue        string            `json:"queue"`
	Title        string            `json:"title"`
	Summary      string            `json:"summary,omitempty"`
	Description  string            `json:"description,omitempty"`
	Status       string            `json:"status"`
	Priority     string            `json:"priority"`
	CreatedBy    string            `json:"created_by,omitempty"`
	Orchestrator string            `json:"orchestrator,omitempty"`
	Worker       string            `json:"worker,omitempty"`
	ParentTaskID string            `json:"parent_task_id,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	CreatedAt    string            `json:"created_at"`
	UpdatedAt    string            `json:"updated_at"`
	StartedAt    string            `json:"started_at,omitempty"`
	CompletedAt  string            `json:"completed_at,omitempty"`
	FailedAt     string            `json:"failed_at,omitempty"`
	BlockedAt    string            `json:"blocked_at,omitempty"`
}

// TaskFilter scopes task listing.
type TaskFilter struct {
	Project      string
	Queue        string
	Status       string
	Worker       string
	Orchestrator string
	Limit        int
}

// TaskEvent is an append-only activity record inside a task.
// It can capture status transitions, comms, issues, lessons, pitfalls,
// successes, and references back to memories.
type TaskEvent struct {
	ID           string            `json:"id"`
	TaskID       string            `json:"task_id"`
	EventType    string            `json:"event_type"`
	ActorRole    string            `json:"actor_role,omitempty"`
	ActorName    string            `json:"actor_name,omitempty"`
	ActorUser    string            `json:"actor_user,omitempty"`
	ActorMachine string            `json:"actor_machine,omitempty"`
	ActorAgent   string            `json:"actor_agent,omitempty"`
	Summary      string            `json:"summary,omitempty"`
	Content      string            `json:"content,omitempty"`
	Status       string            `json:"status,omitempty"`
	MemoryID     string            `json:"memory_id,omitempty"`
	Source       string            `json:"source,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	CreatedAt    string            `json:"created_at"`
}

// TaskQueueStore stores tasks separately from memories.
type TaskQueueStore interface {
	CreateTask(task *Task) (*Task, error)
	GetTask(id string) (*Task, error)
	UpdateTask(task *Task) error
	ListTasks(filter *TaskFilter) ([]*Task, error)
	CreateTaskEvent(event *TaskEvent) (*TaskEvent, error)
	ListTaskEvents(taskID string, limit int) ([]*TaskEvent, error)
}

func NormalizeTaskStatus(status string) string {
	status = strings.TrimSpace(strings.ToLower(status))
	switch status {
	case "", TaskStatusQueued:
		return TaskStatusQueued
	case TaskStatusStarted, TaskStatusDone, TaskStatusFailed, TaskStatusBlocked, TaskStatusCanceled:
		return status
	default:
		return status
	}
}

func ValidTaskStatus(status string) bool {
	switch NormalizeTaskStatus(status) {
	case TaskStatusQueued, TaskStatusStarted, TaskStatusDone, TaskStatusFailed, TaskStatusBlocked, TaskStatusCanceled:
		return true
	default:
		return false
	}
}

func NormalizeTaskPriority(priority string) string {
	priority = strings.TrimSpace(strings.ToLower(priority))
	switch priority {
	case "", TaskPriorityNormal:
		return TaskPriorityNormal
	case TaskPriorityLow, TaskPriorityHigh, TaskPriorityUrgent:
		return priority
	default:
		return priority
	}
}

func ValidTaskPriority(priority string) bool {
	switch NormalizeTaskPriority(priority) {
	case TaskPriorityLow, TaskPriorityNormal, TaskPriorityHigh, TaskPriorityUrgent:
		return true
	default:
		return false
	}
}

func NormalizeTaskEventType(eventType string) string {
	eventType = strings.TrimSpace(strings.ToLower(eventType))
	if eventType == "" {
		return TaskEventNote
	}
	return eventType
}

func ValidTaskEventType(eventType string) bool {
	switch NormalizeTaskEventType(eventType) {
	case TaskEventStatus, TaskEventCommunication, TaskEventIssue, TaskEventLesson, TaskEventPitfall, TaskEventSuccess, TaskEventMemoryRef, TaskEventNote:
		return true
	default:
		return false
	}
}

func prepareTaskForWrite(task *Task) {
	if task == nil {
		return
	}
	task.Status = NormalizeTaskStatus(task.Status)
	task.Priority = NormalizeTaskPriority(task.Priority)
	if strings.TrimSpace(task.Queue) == "" {
		task.Queue = "default"
	}
}

func applyTaskTimestamps(task *Task, now string) {
	if task == nil {
		return
	}
	task.UpdatedAt = now
	switch task.Status {
	case TaskStatusStarted:
		if task.StartedAt == "" {
			task.StartedAt = now
		}
	case TaskStatusDone:
		if task.StartedAt == "" {
			task.StartedAt = now
		}
		if task.CompletedAt == "" {
			task.CompletedAt = now
		}
	case TaskStatusFailed:
		if task.StartedAt == "" {
			task.StartedAt = now
		}
		if task.FailedAt == "" {
			task.FailedAt = now
		}
	case TaskStatusBlocked:
		if task.BlockedAt == "" {
			task.BlockedAt = now
		}
	}
}

func marshalTaskMetadata(metadata map[string]string) string {
	if len(metadata) == 0 {
		return "{}"
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func parseTaskMetadata(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var metadata map[string]string
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return nil, fmt.Errorf("unmarshal task metadata: %w", err)
	}
	return metadata, nil
}

func parseTaskRow(scan func(dest ...any) error) (*Task, error) {
	var task Task
	var summary, description, createdBy, orchestrator, worker, parentTaskID sql.NullString
	var metadataJSON string
	var startedAt, completedAt, failedAt, blockedAt sql.NullString
	if err := scan(
		&task.ID,
		&task.Project,
		&task.Queue,
		&task.Title,
		&summary,
		&description,
		&task.Status,
		&task.Priority,
		&createdBy,
		&orchestrator,
		&worker,
		&parentTaskID,
		&metadataJSON,
		&task.CreatedAt,
		&task.UpdatedAt,
		&startedAt,
		&completedAt,
		&failedAt,
		&blockedAt,
	); err != nil {
		return nil, err
	}
	task.Summary = summary.String
	task.Description = description.String
	task.CreatedBy = createdBy.String
	task.Orchestrator = orchestrator.String
	task.Worker = worker.String
	task.ParentTaskID = parentTaskID.String
	task.StartedAt = startedAt.String
	task.CompletedAt = completedAt.String
	task.FailedAt = failedAt.String
	task.BlockedAt = blockedAt.String
	metadata, err := parseTaskMetadata(metadataJSON)
	if err != nil {
		return nil, err
	}
	task.Metadata = metadata
	return &task, nil
}

func scanTaskRows(rows *sql.Rows) ([]*Task, error) {
	var out []*Task
	for rows.Next() {
		task, err := parseTaskRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		out = append(out, task)
	}
	return out, nil
}

func parseTaskEventRow(scan func(dest ...any) error) (*TaskEvent, error) {
	var event TaskEvent
	var actorRole, actorName, actorUser, actorMachine, actorAgent sql.NullString
	var summary, content, status, memoryID, source sql.NullString
	var metadataJSON string
	if err := scan(
		&event.ID,
		&event.TaskID,
		&event.EventType,
		&actorRole,
		&actorName,
		&actorUser,
		&actorMachine,
		&actorAgent,
		&summary,
		&content,
		&status,
		&memoryID,
		&source,
		&metadataJSON,
		&event.CreatedAt,
	); err != nil {
		return nil, err
	}
	event.ActorRole = actorRole.String
	event.ActorName = actorName.String
	event.ActorUser = actorUser.String
	event.ActorMachine = actorMachine.String
	event.ActorAgent = actorAgent.String
	event.Summary = summary.String
	event.Content = content.String
	event.Status = status.String
	event.MemoryID = memoryID.String
	event.Source = source.String
	metadata, err := parseTaskMetadata(metadataJSON)
	if err != nil {
		return nil, err
	}
	event.Metadata = metadata
	return &event, nil
}

func scanTaskEventRows(rows *sql.Rows) ([]*TaskEvent, error) {
	var out []*TaskEvent
	for rows.Next() {
		event, err := parseTaskEventRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan task event: %w", err)
		}
		out = append(out, event)
	}
	return out, nil
}

func (c *Client) CreateTask(task *Task) (*Task, error) {
	prepareTaskForWrite(task)
	now := nowUTCString()
	task.CreatedAt = now
	applyTaskTimestamps(task, now)

	err := c.DB.QueryRow(`
		INSERT INTO tasks (project, queue_name, title, summary, description, status, priority, created_by, orchestrator, worker, parent_task_id, metadata_json, created_at, updated_at, started_at, completed_at, failed_at, blocked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`,
		task.Project,
		task.Queue,
		task.Title,
		task.Summary,
		task.Description,
		task.Status,
		task.Priority,
		task.CreatedBy,
		task.Orchestrator,
		task.Worker,
		nullString(task.ParentTaskID),
		marshalTaskMetadata(task.Metadata),
		task.CreatedAt,
		task.UpdatedAt,
		nullString(task.StartedAt),
		nullString(task.CompletedAt),
		nullString(task.FailedAt),
		nullString(task.BlockedAt),
	).Scan(&task.ID)
	if err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}
	return task, nil
}

func (c *Client) GetTask(id string) (*Task, error) {
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

func (c *Client) UpdateTask(task *Task) error {
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
		task.Project,
		task.Queue,
		task.Title,
		task.Summary,
		task.Description,
		task.Status,
		task.Priority,
		task.CreatedBy,
		task.Orchestrator,
		task.Worker,
		nullString(task.ParentTaskID),
		marshalTaskMetadata(task.Metadata),
		task.UpdatedAt,
		nullString(task.StartedAt),
		nullString(task.CompletedAt),
		nullString(task.FailedAt),
		nullString(task.BlockedAt),
		task.ID,
	)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	return nil
}

func (c *Client) ListTasks(filter *TaskFilter) ([]*Task, error) {
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

func (c *Client) CreateTaskEvent(event *TaskEvent) (*TaskEvent, error) {
	event.EventType = NormalizeTaskEventType(event.EventType)
	event.CreatedAt = nowUTCString()
	err := c.DB.QueryRow(`
		INSERT INTO task_events (task_id, event_type, actor_role, actor_name, actor_user, actor_machine, actor_agent, summary, content, status, memory_id, source, metadata_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`,
		event.TaskID,
		event.EventType,
		event.ActorRole,
		event.ActorName,
		event.ActorUser,
		event.ActorMachine,
		event.ActorAgent,
		event.Summary,
		event.Content,
		event.Status,
		nullString(event.MemoryID),
		event.Source,
		marshalTaskMetadata(event.Metadata),
		event.CreatedAt,
	).Scan(&event.ID)
	if err != nil {
		return nil, fmt.Errorf("insert task event: %w", err)
	}
	return event, nil
}

func (c *Client) ListTaskEvents(taskID string, limit int) ([]*TaskEvent, error) {
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
