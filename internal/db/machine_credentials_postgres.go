package db

import (
	"database/sql"
	"fmt"
)

func (c *PostgresClient) CreateMachineCredential(cred *MachineCredential) (*MachineCredential, error) {
	err := c.DB.QueryRow(`
		INSERT INTO machine_credentials (token_hash, user_name, machine_id, agent_name, agent_type, groups_json, display_name, description)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at
	`,
		cred.TokenHash, cred.User, cred.MachineID, cred.AgentName, cred.AgentType, marshalGroups(cred.Groups), cred.DisplayName, cred.Description,
	).Scan(&cred.ID, &cred.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert machine credential: %w", err)
	}
	return cred, nil
}

func (c *PostgresClient) GetMachineCredentialByTokenHash(tokenHash string) (*MachineCredential, error) {
	row := c.DB.QueryRow(`
		SELECT id, token_hash, user_name, machine_id, agent_name, agent_type, groups_json, display_name, description,
		       created_at, last_seen_at, revoked_at
		FROM machine_credentials
		WHERE token_hash = $1 AND revoked_at IS NULL
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

func (c *PostgresClient) ListMachineCredentials() ([]*MachineCredential, error) {
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

func (c *PostgresClient) TouchMachineCredential(id string) error {
	_, err := c.DB.Exec(`UPDATE machine_credentials SET last_seen_at = NOW() AT TIME ZONE 'UTC' WHERE id = $1 AND revoked_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("touch machine credential: %w", err)
	}
	return nil
}

func (c *PostgresClient) RevokeMachineCredential(id string) error {
	_, err := c.DB.Exec(`UPDATE machine_credentials SET revoked_at = NOW() AT TIME ZONE 'UTC' WHERE id = $1 AND revoked_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("revoke machine credential: %w", err)
	}
	return nil
}
