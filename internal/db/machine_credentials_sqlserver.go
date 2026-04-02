package db

import (
	"database/sql"
	"fmt"
)

func (c *SQLServerClient) CreateMachineCredential(cred *MachineCredential) (*MachineCredential, error) {
	err := c.db.QueryRow(`
		INSERT INTO machine_credentials (token_hash, user_name, machine_id, agent_name, agent_type, groups_json, display_name, description)
		OUTPUT INSERTED.id, INSERTED.created_at
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8)
	`,
		cred.TokenHash, cred.User, cred.MachineID, cred.AgentName, cred.AgentType, marshalGroups(cred.Groups), cred.DisplayName, cred.Description,
	).Scan(&cred.ID, &cred.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert machine credential: %w", err)
	}
	return cred, nil
}

func (c *SQLServerClient) GetMachineCredentialByTokenHash(tokenHash string) (*MachineCredential, error) {
	row := c.db.QueryRow(`
		SELECT id, token_hash, user_name, machine_id, agent_name, agent_type, groups_json, display_name, description,
		       created_at, last_seen_at, revoked_at
		FROM machine_credentials
		WHERE token_hash = @p1 AND revoked_at IS NULL
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

func (c *SQLServerClient) ListMachineCredentials() ([]*MachineCredential, error) {
	rows, err := c.db.Query(`
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

func (c *SQLServerClient) TouchMachineCredential(id string) error {
	_, err := c.db.Exec(`UPDATE machine_credentials SET last_seen_at = GETUTCDATE() WHERE id = @p1 AND revoked_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("touch machine credential: %w", err)
	}
	return nil
}

func (c *SQLServerClient) RevokeMachineCredential(id string) error {
	_, err := c.db.Exec(`UPDATE machine_credentials SET revoked_at = GETUTCDATE() WHERE id = @p1 AND revoked_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("revoke machine credential: %w", err)
	}
	return nil
}
