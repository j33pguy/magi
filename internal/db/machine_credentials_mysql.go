package db

import (
	"database/sql"
	"fmt"
)

func (c *MySQLClient) CreateMachineCredential(cred *MachineCredential) (*MachineCredential, error) {
	cred.ID = newHexID()
	cred.CreatedAt = nowUTCString()
	_, err := c.DB.Exec(`
		INSERT INTO machine_credentials (id, token_hash, user_name, machine_id, agent_name, agent_type, groups_json, display_name, description, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		cred.ID, cred.TokenHash, cred.User, cred.MachineID, cred.AgentName, cred.AgentType, marshalGroups(cred.Groups), cred.DisplayName, cred.Description, cred.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert machine credential: %w", err)
	}
	return cred, nil
}

func (c *MySQLClient) GetMachineCredentialByTokenHash(tokenHash string) (*MachineCredential, error) {
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

func (c *MySQLClient) ListMachineCredentials() ([]*MachineCredential, error) {
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

func (c *MySQLClient) TouchMachineCredential(id string) error {
	_, err := c.DB.Exec(`UPDATE machine_credentials SET last_seen_at = UTC_TIMESTAMP() WHERE id = ? AND revoked_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("touch machine credential: %w", err)
	}
	return nil
}

func (c *MySQLClient) RevokeMachineCredential(id string) error {
	_, err := c.DB.Exec(`UPDATE machine_credentials SET revoked_at = UTC_TIMESTAMP() WHERE id = ? AND revoked_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("revoke machine credential: %w", err)
	}
	return nil
}
