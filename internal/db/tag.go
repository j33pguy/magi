package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// ExistsWithContentHash returns the memory ID that has the given hash tag, or "" if none.
func (c *Client) ExistsWithContentHash(hash string) (string, error) {
	tag := "hash:" + hash
	var id string
	err := c.DB.QueryRow("SELECT memory_id FROM memory_tags WHERE tag = ? LIMIT 1", tag).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("checking content hash: %w", err)
	}
	return id, nil
}

// GetTags returns all tags for a memory.
func (c *Client) GetTags(memoryID string) ([]string, error) {
	rows, err := c.DB.Query("SELECT tag FROM memory_tags WHERE memory_id = ? ORDER BY tag", memoryID)
	if err != nil {
		return nil, fmt.Errorf("getting tags for %s: %w", memoryID, err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("scanning tag: %w", err)
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

// SetTags replaces all tags for a memory.
// Wraps DELETE + INSERT in a transaction so both use the same Turso Hrana
// stream, preventing expiry between the two operations.
func (c *Client) SetTags(memoryID string, tags []string) error {
	tx, err := c.DB.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec("DELETE FROM memory_tags WHERE memory_id = ?", memoryID); err != nil {
		return fmt.Errorf("clearing tags: %w", err)
	}

	if len(tags) == 0 {
		return tx.Commit()
	}

	// Batch all tags into a single INSERT with multiple value tuples.
	placeholders := make([]string, len(tags))
	args := make([]any, 0, len(tags)*2)
	for i, tag := range tags {
		placeholders[i] = "(?, ?)"
		args = append(args, memoryID, tag)
	}

	query := fmt.Sprintf("INSERT INTO memory_tags (memory_id, tag) VALUES %s", strings.Join(placeholders, ", "))
	if _, err := tx.Exec(query, args...); err != nil {
		return fmt.Errorf("inserting tags: %w", err)
	}

	return tx.Commit()
}
