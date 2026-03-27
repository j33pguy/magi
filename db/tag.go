package db

import "fmt"

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
// Uses individual statements instead of a transaction to avoid Turso's
// Hrana stream timeout ("stream not found") on reused connections.
func (c *Client) SetTags(memoryID string, tags []string) error {
	if _, err := c.DB.Exec("DELETE FROM memory_tags WHERE memory_id = ?", memoryID); err != nil {
		return fmt.Errorf("clearing tags: %w", err)
	}

	for _, tag := range tags {
		if _, err := c.DB.Exec("INSERT INTO memory_tags (memory_id, tag) VALUES (?, ?)", memoryID, tag); err != nil {
			return fmt.Errorf("inserting tag %q: %w", tag, err)
		}
	}

	return nil
}
