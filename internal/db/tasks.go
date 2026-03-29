package db

import (
	"database/sql"
	"fmt"
	"time"
)

type Task struct {
	ID              string     `json:"id"`
	Description     string     `json:"description"`
	Status          string     `json:"status"`
	CreatedBy       string     `json:"created_by,omitempty"`
	ProgressPercent int        `json:"progress_percent"`
	Metadata        string     `json:"metadata,omitempty"`
	Subtasks        []*Subtask `json:"subtasks,omitempty"`
	CreatedAt       string     `json:"created_at"`
	UpdatedAt       string     `json:"updated_at"`
}

type Subtask struct {
	ID              string `json:"id"`
	TaskID          string `json:"task_id"`
	Description     string `json:"description"`
	AgentID         string `json:"agent_id,omitempty"`
	Status          string `json:"status"`
	ProgressPercent int    `json:"progress_percent"`
	Output          string `json:"output,omitempty"`
	SortOrder       int    `json:"sort_order"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

func (c *Client) CreateTask(task *Task) (*Task, error) {
	if task.ID == "" { task.ID = genID() }
	now := time.Now().UTC().Format(time.DateTime)
	task.Status = "created"
	task.CreatedAt = now
	task.UpdatedAt = now
	_, err := c.DB.Exec("INSERT INTO tasks (id, description, status, created_by, progress_percent, metadata, created_at, updated_at) VALUES (?, ?, ?, ?, 0, ?, ?, ?)",
		task.ID, task.Description, task.Status, task.CreatedBy, task.Metadata, now, now)
	if err != nil { return nil, fmt.Errorf("creating task: %w", err) }
	for i, sub := range task.Subtasks {
		if sub.ID == "" { sub.ID = genID() }
		sub.TaskID = task.ID
		sub.Status = "pending"
		sub.SortOrder = i
		sub.CreatedAt = now
		sub.UpdatedAt = now
		_, err := c.DB.Exec("INSERT INTO subtasks (id, task_id, description, agent_id, status, sort_order, created_at, updated_at) VALUES (?, ?, ?, ?, 'pending', ?, ?, ?)",
			sub.ID, task.ID, sub.Description, sub.AgentID, i, now, now)
		if err != nil { return nil, fmt.Errorf("creating subtask: %w", err) }
	}
	return task, nil
}

func (c *Client) GetTask(id string) (*Task, error) {
	t := &Task{}
	var meta sql.NullString
	err := c.DB.QueryRow("SELECT id, description, status, created_by, progress_percent, metadata, created_at, updated_at FROM tasks WHERE id = ?", id).
		Scan(&t.ID, &t.Description, &t.Status, &t.CreatedBy, &t.ProgressPercent, &meta, &t.CreatedAt, &t.UpdatedAt)
	if err != nil { return nil, err }
	t.Metadata = meta.String
	rows, err := c.DB.Query("SELECT id, task_id, description, agent_id, status, progress_percent, output, sort_order, created_at, updated_at FROM subtasks WHERE task_id = ? ORDER BY sort_order", id)
	if err != nil { return nil, err }
	defer rows.Close()
	for rows.Next() {
		s := &Subtask{}
		var aid, out sql.NullString
		rows.Scan(&s.ID, &s.TaskID, &s.Description, &aid, &s.Status, &s.ProgressPercent, &out, &s.SortOrder, &s.CreatedAt, &s.UpdatedAt)
		s.AgentID = aid.String
		s.Output = out.String
		t.Subtasks = append(t.Subtasks, s)
	}
	return t, nil
}

func (c *Client) ReportProgress(subtaskID string, percent int, message string) error {
	now := time.Now().UTC().Format(time.DateTime)
	_, err := c.DB.Exec("INSERT INTO progress_log (id, subtask_id, percent, message, created_at) VALUES (?, ?, ?, ?, ?)", genID(), subtaskID, percent, message, now)
	if err != nil { return err }
	_, err = c.DB.Exec("UPDATE subtasks SET progress_percent = ?, status = 'in_progress', updated_at = ? WHERE id = ?", percent, now, subtaskID)
	return err
}

func (c *Client) UpdateSubtaskStatus(subtaskID, status, output string) error {
	now := time.Now().UTC().Format(time.DateTime)
	_, err := c.DB.Exec("UPDATE subtasks SET status = ?, output = ?, updated_at = ? WHERE id = ?", status, output, now, subtaskID)
	return err
}

func (c *Client) AssignSubtask(subtaskID, agentID string) error {
	now := time.Now().UTC().Format(time.DateTime)
	_, err := c.DB.Exec("UPDATE subtasks SET agent_id = ?, status = 'assigned', updated_at = ? WHERE id = ?", agentID, now, subtaskID)
	return err
}

func (c *Client) ListTasks(status string) ([]*Task, error) {
	q := "SELECT id, description, status, created_by, progress_percent, metadata, created_at, updated_at FROM tasks"
	var args []any
	if status != "" { q += " WHERE status = ?"; args = append(args, status) }
	q += " ORDER BY created_at DESC"
	rows, err := c.DB.Query(q, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	var tasks []*Task
	for rows.Next() {
		t := &Task{}
		var meta sql.NullString
		rows.Scan(&t.ID, &t.Description, &t.Status, &t.CreatedBy, &t.ProgressPercent, &meta, &t.CreatedAt, &t.UpdatedAt)
		t.Metadata = meta.String
		tasks = append(tasks, t)
	}
	return tasks, nil
}
