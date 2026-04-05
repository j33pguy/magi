package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// EnrollmentToken represents a one-time (or multi-use) enrollment token
// that can be exchanged for a machine credential without admin access.
type EnrollmentToken struct {
	ID            string   `json:"id"`
	TokenHash     string   `json:"-"`
	Label         string   `json:"label"`
	MaxUses       int      `json:"max_uses"`
	UseCount      int      `json:"use_count"`
	DefaultUser   string   `json:"default_user,omitempty"`
	DefaultGroups []string `json:"default_groups,omitempty"`
	ExpiresAt     string   `json:"expires_at,omitempty"`
	CreatedAt     string   `json:"created_at"`
	RevokedAt     string   `json:"revoked_at,omitempty"`
}

// IsValid returns true if the token is still usable.
func (t *EnrollmentToken) IsValid() bool {
	if t.RevokedAt != "" {
		return false
	}
	if t.MaxUses > 0 && t.UseCount >= t.MaxUses {
		return false
	}
	if t.ExpiresAt != "" {
		exp, err := time.Parse(time.DateTime, t.ExpiresAt)
		if err == nil && time.Now().UTC().After(exp) {
			return false
		}
	}
	return true
}

func (c *Client) CreateEnrollmentToken(et *EnrollmentToken) (*EnrollmentToken, error) {
	groupsJSON := "[]"
	if len(et.DefaultGroups) > 0 {
		data, _ := json.Marshal(et.DefaultGroups)
		groupsJSON = string(data)
	}

	var expiresAt sql.NullString
	if et.ExpiresAt != "" {
		expiresAt = sql.NullString{String: et.ExpiresAt, Valid: true}
	}

	err := c.DB.QueryRow(`
		INSERT INTO enrollment_tokens (token_hash, label, max_uses, default_user, default_groups, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING id, created_at
	`,
		et.TokenHash,
		et.Label,
		et.MaxUses,
		et.DefaultUser,
		groupsJSON,
		expiresAt,
	).Scan(&et.ID, &et.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert enrollment token: %w", err)
	}
	return et, nil
}

func (c *Client) GetEnrollmentTokenByHash(tokenHash string) (*EnrollmentToken, error) {
	var et EnrollmentToken
	var groupsJSON string
	var expiresAt, revokedAt sql.NullString

	err := c.DB.QueryRow(`
		SELECT id, token_hash, label, max_uses, use_count, default_user, default_groups, expires_at, created_at, revoked_at
		FROM enrollment_tokens
		WHERE token_hash = ?
	`, tokenHash).Scan(
		&et.ID, &et.TokenHash, &et.Label, &et.MaxUses, &et.UseCount,
		&et.DefaultUser, &groupsJSON, &expiresAt, &et.CreatedAt, &revokedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get enrollment token: %w", err)
	}

	if groupsJSON != "" && groupsJSON != "[]" {
		json.Unmarshal([]byte(groupsJSON), &et.DefaultGroups)
	}
	if expiresAt.Valid {
		et.ExpiresAt = expiresAt.String
	}
	if revokedAt.Valid {
		et.RevokedAt = revokedAt.String
	}
	return &et, nil
}

func (c *Client) IncrementEnrollmentTokenUse(id string) error {
	_, err := c.DB.Exec(`UPDATE enrollment_tokens SET use_count = use_count + 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("increment enrollment token use: %w", err)
	}
	return nil
}

func (c *Client) ListEnrollmentTokens() ([]*EnrollmentToken, error) {
	rows, err := c.DB.Query(`
		SELECT id, token_hash, label, max_uses, use_count, default_user, default_groups, expires_at, created_at, revoked_at
		FROM enrollment_tokens
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list enrollment tokens: %w", err)
	}
	defer rows.Close()

	var out []*EnrollmentToken
	for rows.Next() {
		var et EnrollmentToken
		var groupsJSON string
		var expiresAt, revokedAt sql.NullString
		if err := rows.Scan(
			&et.ID, &et.TokenHash, &et.Label, &et.MaxUses, &et.UseCount,
			&et.DefaultUser, &groupsJSON, &expiresAt, &et.CreatedAt, &revokedAt,
		); err != nil {
			return nil, fmt.Errorf("scan enrollment token: %w", err)
		}
		if groupsJSON != "" && groupsJSON != "[]" {
			json.Unmarshal([]byte(groupsJSON), &et.DefaultGroups)
		}
		if expiresAt.Valid {
			et.ExpiresAt = expiresAt.String
		}
		if revokedAt.Valid {
			et.RevokedAt = revokedAt.String
		}
		out = append(out, &et)
	}
	return out, nil
}

func (c *Client) RevokeEnrollmentToken(id string) error {
	_, err := c.DB.Exec(`UPDATE enrollment_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, nowUTCString(), id)
	if err != nil {
		return fmt.Errorf("revoke enrollment token: %w", err)
	}
	return nil
}
