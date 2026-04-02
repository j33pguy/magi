package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// MachineCredential represents a machine-scoped credential for sync and worker access.
type MachineCredential struct {
	ID          string   `json:"id"`
	TokenHash   string   `json:"-"`
	User        string   `json:"user"`
	MachineID   string   `json:"machine_id"`
	AgentName   string   `json:"agent_name,omitempty"`
	AgentType   string   `json:"agent_type,omitempty"`
	Groups      []string `json:"groups,omitempty"`
	DisplayName string   `json:"display_name,omitempty"`
	Description string   `json:"description,omitempty"`
	CreatedAt   string   `json:"created_at"`
	LastSeenAt  string   `json:"last_seen_at,omitempty"`
	RevokedAt   string   `json:"revoked_at,omitempty"`
}

func parseMachineCredentialRow(scan func(dest ...any) error) (*MachineCredential, error) {
	var cred MachineCredential
	var groupsJSON string
	var lastSeen, revoked sql.NullString
	if err := scan(
		&cred.ID,
		&cred.TokenHash,
		&cred.User,
		&cred.MachineID,
		&cred.AgentName,
		&cred.AgentType,
		&groupsJSON,
		&cred.DisplayName,
		&cred.Description,
		&cred.CreatedAt,
		&lastSeen,
		&revoked,
	); err != nil {
		return nil, err
	}
	if groupsJSON != "" {
		if err := json.Unmarshal([]byte(groupsJSON), &cred.Groups); err != nil {
			return nil, fmt.Errorf("unmarshal machine groups: %w", err)
		}
	}
	cred.LastSeenAt = lastSeen.String
	cred.RevokedAt = revoked.String
	return &cred, nil
}

func scanMachineCredentialRows(rows *sql.Rows) ([]*MachineCredential, error) {
	var out []*MachineCredential
	for rows.Next() {
		cred, err := parseMachineCredentialRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan machine credential: %w", err)
		}
		out = append(out, cred)
	}
	return out, nil
}

func marshalGroups(groups []string) string {
	if len(groups) == 0 {
		return "[]"
	}
	data, err := json.Marshal(groups)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func nowUTCString() string {
	return time.Now().UTC().Format(time.DateTime)
}

func (c *Client) CreateMachineCredential(cred *MachineCredential) (*MachineCredential, error) {
	err := c.DB.QueryRow(`
		INSERT INTO machine_credentials (token_hash, user_name, machine_id, agent_name, agent_type, groups_json, display_name, description)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id, created_at
	`,
		cred.TokenHash,
		cred.User,
		cred.MachineID,
		cred.AgentName,
		cred.AgentType,
		marshalGroups(cred.Groups),
		cred.DisplayName,
		cred.Description,
	).Scan(&cred.ID, &cred.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert machine credential: %w", err)
	}
	return cred, nil
}

func (c *Client) GetMachineCredentialByTokenHash(tokenHash string) (*MachineCredential, error) {
	row := c.DB.QueryRow(`
		SELECT id, token_hash, user_name, machine_id, agent_name, agent_type, groups_json, display_name, description,
		       created_at, last_seen_at, revoked_at
		FROM machine_credentials
		WHERE token_hash = ? AND revoked_at IS NULL
	`, tokenHash)
	cred, err := parseMachineCredentialRow(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get machine credential by token hash: %w", err)
	}
	return cred, nil
}

func (c *Client) ListMachineCredentials() ([]*MachineCredential, error) {
	rows, err := c.DB.Query(`
		SELECT id, token_hash, user_name, machine_id, agent_name, agent_type, groups_json, display_name, description,
		       created_at, last_seen_at, revoked_at
		FROM machine_credentials
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list machine credentials: %w", err)
	}
	defer rows.Close()
	return scanMachineCredentialRows(rows)
}

func (c *Client) TouchMachineCredential(id string) error {
	_, err := c.DB.Exec(`UPDATE machine_credentials SET last_seen_at = ? WHERE id = ? AND revoked_at IS NULL`, nowUTCString(), id)
	if err != nil {
		return fmt.Errorf("touch machine credential: %w", err)
	}
	return nil
}

func (c *Client) RevokeMachineCredential(id string) error {
	_, err := c.DB.Exec(`UPDATE machine_credentials SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, nowUTCString(), id)
	if err != nil {
		return fmt.Errorf("revoke machine credential: %w", err)
	}
	return nil
}
