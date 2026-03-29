package db

const orchestrationSchema = `
CREATE TABLE IF NOT EXISTS agents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    capabilities TEXT DEFAULT '[]',
    endpoint TEXT DEFAULT '',
    status TEXT DEFAULT 'offline',
    metadata TEXT DEFAULT '{}',
    last_heartbeat DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,
    description TEXT NOT NULL,
    status TEXT DEFAULT 'created',
    created_by TEXT DEFAULT '',
    progress_percent INTEGER DEFAULT 0,
    metadata TEXT DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS subtasks (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    description TEXT NOT NULL,
    agent_id TEXT DEFAULT '',
    status TEXT DEFAULT 'pending',
    progress_percent INTEGER DEFAULT 0,
    output TEXT DEFAULT '',
    sort_order INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS progress_log (
    id TEXT PRIMARY KEY,
    subtask_id TEXT NOT NULL,
    percent INTEGER NOT NULL,
    message TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_subtasks_task_id ON subtasks(task_id);
CREATE INDEX IF NOT EXISTS idx_progress_subtask_id ON progress_log(subtask_id);
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
`

// RunOrchestrationMigrations applies the agent registry and task management schema.
func (c *Client) RunOrchestrationMigrations() error {
	stmts := splitSQL(orchestrationSchema)
	for _, stmt := range stmts {
		if stmt == "" {
			continue
		}
		if _, err := c.DB.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}
