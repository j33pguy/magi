package db

import (
	"context"
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
// Avoids BEGIN/COMMIT transactions because Turso embedded-replica Hrana streams
// fail on BeginTx ("connection has reached an invalid state, started with Txn").
// Instead: DELETE then batched INSERT as separate statements with retry on stream expiry.
func (c *Client) SetTags(memoryID string, tags []string) error {
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		err = c.setTagsNoTx(memoryID, tags)
		if err == nil || !isStreamExpired(err) {
			return err
		}
		c.logger.Warn("retrying SetTags after stream expiry", "memoryID", memoryID, "attempt", attempt)
	}
	return err
}

func (c *Client) setTagsNoTx(memoryID string, tags []string) error {
	// Force a fresh Hrana stream by pinging first — the embedded replica connector
	// lets streams expire server-side while keeping stale connections in the pool.
	_ = c.DB.Ping()

	if _, err := c.DB.ExecContext(context.Background(), "DELETE FROM memory_tags WHERE memory_id = ?", memoryID); err != nil {
		return fmt.Errorf("clearing tags: %w", err)
	}

	tags = normalizeTags(tags)
	if len(tags) == 0 {
		return nil
	}

	placeholders := make([]string, len(tags))
	args := make([]any, 0, len(tags)*2)
	for i, tag := range tags {
		placeholders[i] = "(?, ?)"
		args = append(args, memoryID, tag)
	}

	query := fmt.Sprintf("INSERT INTO memory_tags (memory_id, tag) VALUES %s", strings.Join(placeholders, ", "))
	if _, err := c.DB.ExecContext(context.Background(), query, args...); err != nil {
		return fmt.Errorf("inserting tags: %w", err)
	}

	return nil
}
