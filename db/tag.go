package db

import (
	"fmt"
	"strings"
)

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
// Uses a single batched multi-value INSERT instead of N individual statements
// to minimize round-trips and reduce exposure to Turso Hrana stream expiry.
func (c *Client) SetTags(memoryID string, tags []string) error {
	if _, err := c.DB.Exec("DELETE FROM memory_tags WHERE memory_id = ?", memoryID); err != nil {
		return fmt.Errorf("clearing tags: %w", err)
	}

	if len(tags) == 0 {
		return nil
	}

	// Batch all tags into a single INSERT with multiple value tuples.
	// This is one round-trip instead of N, avoiding stream expiry between statements.
	placeholders := make([]string, len(tags))
	args := make([]any, 0, len(tags)*2)
	for i, tag := range tags {
		placeholders[i] = "(?, ?)"
		args = append(args, memoryID, tag)
	}

	query := fmt.Sprintf("INSERT INTO memory_tags (memory_id, tag) VALUES %s", strings.Join(placeholders, ", "))
	if _, err := c.DB.Exec(query, args...); err != nil {
		return fmt.Errorf("inserting tags: %w", err)
	}

	return nil
}
