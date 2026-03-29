package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
	"crypto/rand"
	"encoding/hex"
)

type Agent struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
	Endpoint     string   `json:"endpoint,omitempty"`
	Status       string   `json:"status"`
	Metadata     string   `json:"metadata,omitempty"`
	LastHeartbeat string  `json:"lastHeartbeat,omitempty"`
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
}

func genID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (c *Client) RegisterAgent(agent *Agent) (*Agent, error) {
	if agent.ID == "" { agent.ID = genID() }
	now := time.Now().UTC().Format(time.DateTime)
	caps, _ := json.Marshal(agent.Capabilities)
	_, err := c.DB.Exec(
		"INSERT OR REPLACE INTO agents (id, name, capabilities, endpoint, status, metadata, last_heartbeat, created_at, updated_at) VALUES (?, ?, ?, ?, 'online', ?, ?, ?, ?)",
		agent.ID, agent.Name, string(caps), agent.Endpoint, agent.Metadata, now, now, now)
	if err != nil { return nil, fmt.Errorf("registering agent: %w", err) }
	agent.Status = "online"
	agent.CreatedAt = now
	agent.UpdatedAt = now
	return agent, nil
}

func (c *Client) HeartbeatAgent(id string) error {
	now := time.Now().UTC().Format(time.DateTime)
	_, err := c.DB.Exec("UPDATE agents SET last_heartbeat = ?, status = 'online', updated_at = ? WHERE id = ?", now, now, id)
	return err
}

func (c *Client) GetAgent(id string) (*Agent, error) {
	a := &Agent{}
	var caps, meta, hb sql.NullString
	err := c.DB.QueryRow("SELECT id, name, capabilities, endpoint, status, metadata, last_heartbeat, created_at, updated_at FROM agents WHERE id = ?", id).
		Scan(&a.ID, &a.Name, &caps, &a.Endpoint, &a.Status, &meta, &hb, &a.CreatedAt, &a.UpdatedAt)
	if err != nil { return nil, err }
	if caps.Valid { json.Unmarshal([]byte(caps.String), &a.Capabilities) }
	a.Metadata = meta.String
	a.LastHeartbeat = hb.String
	return a, nil
}

func (c *Client) ListAgents(status string) ([]*Agent, error) {
	q := "SELECT id, name, capabilities, endpoint, status, metadata, last_heartbeat, created_at, updated_at FROM agents"
	var args []any
	if status != "" { q += " WHERE status = ?"; args = append(args, status) }
	q += " ORDER BY name"
	rows, err := c.DB.Query(q, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	var agents []*Agent
	for rows.Next() {
		a := &Agent{}
		var caps, meta, hb sql.NullString
		rows.Scan(&a.ID, &a.Name, &caps, &a.Endpoint, &a.Status, &meta, &hb, &a.CreatedAt, &a.UpdatedAt)
		if caps.Valid { json.Unmarshal([]byte(caps.String), &a.Capabilities) }
		a.Metadata = meta.String
		a.LastHeartbeat = hb.String
		agents = append(agents, a)
	}
	return agents, nil
}

func (c *Client) DeregisterAgent(id string) error {
	_, err := c.DB.Exec("DELETE FROM agents WHERE id = ?", id)
	return err
}
