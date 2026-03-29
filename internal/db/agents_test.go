package db

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func setupOrchestrationDB(t *testing.T) *SQLiteClient {
	t.Helper()
	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	client, err := NewSQLiteClient(filepath.Join(tmp, "test.db"), logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.RunOrchestrationMigrations(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func TestRegisterAgent(t *testing.T) {
	c := setupOrchestrationDB(t)
	agent, err := c.RegisterAgent(&Agent{Name: "grok", Capabilities: []string{"research", "x-scraping"}, Endpoint: "http://localhost:9000"})
	if err != nil {
		t.Fatal(err)
	}
	if agent.ID == "" {
		t.Error("expected agent ID")
	}
	if agent.Status != "online" {
		t.Errorf("expected online, got %s", agent.Status)
	}
}

func TestRegisterAgent_CustomID(t *testing.T) {
	c := setupOrchestrationDB(t)
	agent, err := c.RegisterAgent(&Agent{ID: "claude-1", Name: "claude", Capabilities: []string{"architecture"}})
	if err != nil {
		t.Fatal(err)
	}
	if agent.ID != "claude-1" {
		t.Errorf("expected claude-1, got %s", agent.ID)
	}
}

func TestRegisterAgent_Upsert(t *testing.T) {
	c := setupOrchestrationDB(t)
	c.RegisterAgent(&Agent{ID: "test-1", Name: "v1", Capabilities: []string{"a"}})
	agent, err := c.RegisterAgent(&Agent{ID: "test-1", Name: "v2", Capabilities: []string{"a", "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if agent.Name != "v2" {
		t.Errorf("expected v2, got %s", agent.Name)
	}
}

func TestGetAgent(t *testing.T) {
	c := setupOrchestrationDB(t)
	c.RegisterAgent(&Agent{ID: "g1", Name: "grok", Capabilities: []string{"research"}})
	agent, err := c.GetAgent("g1")
	if err != nil {
		t.Fatal(err)
	}
	if agent.Name != "grok" {
		t.Errorf("expected grok, got %s", agent.Name)
	}
	if len(agent.Capabilities) != 1 || agent.Capabilities[0] != "research" {
		t.Errorf("unexpected capabilities: %v", agent.Capabilities)
	}
}

func TestGetAgent_NotFound(t *testing.T) {
	c := setupOrchestrationDB(t)
	_, err := c.GetAgent("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
}

func TestListAgents(t *testing.T) {
	c := setupOrchestrationDB(t)
	c.RegisterAgent(&Agent{ID: "a1", Name: "alpha"})
	c.RegisterAgent(&Agent{ID: "a2", Name: "beta"})
	agents, err := c.ListAgents("")
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}
}

func TestListAgents_FilterStatus(t *testing.T) {
	c := setupOrchestrationDB(t)
	c.RegisterAgent(&Agent{ID: "a1", Name: "online-agent"})
	c.DB.Exec("INSERT INTO agents (id, name, status, created_at, updated_at) VALUES ('a2', 'offline-agent', 'offline', datetime('now'), datetime('now'))")
	agents, err := c.ListAgents("online")
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 {
		t.Errorf("expected 1 online agent, got %d", len(agents))
	}
}

func TestHeartbeatAgent(t *testing.T) {
	c := setupOrchestrationDB(t)
	c.RegisterAgent(&Agent{ID: "h1", Name: "heartbeat-test"})
	err := c.HeartbeatAgent("h1")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeregisterAgent(t *testing.T) {
	c := setupOrchestrationDB(t)
	c.RegisterAgent(&Agent{ID: "d1", Name: "delete-me"})
	err := c.DeregisterAgent("d1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.GetAgent("d1")
	if err == nil {
		t.Error("expected error after deregister")
	}
}
