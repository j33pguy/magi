package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// MemoryLink represents a directed relationship between two memories.
type MemoryLink struct {
	ID        string  `json:"id"`
	FromID    string  `json:"fromId"`
	ToID      string  `json:"toId"`
	Relation  string  `json:"relation"`
	Weight    float64 `json:"weight"`
	Auto      bool    `json:"auto"`
	CreatedAt string  `json:"createdAt"`
}

// CreateLink creates a directed link between two memories.
func (c *Client) CreateLink(ctx context.Context, fromID, toID, relation string, weight float64, auto bool) (*MemoryLink, error) {
	now := time.Now().UTC().Format(time.DateTime)
	autoInt := 0
	if auto {
		autoInt = 1
	}

	var id string
	err := c.DB.QueryRowContext(ctx, `
		INSERT INTO memory_links (from_id, to_id, relation, weight, auto, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING id
	`, fromID, toID, relation, weight, autoInt, now).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("creating link: %w", err)
	}

	return &MemoryLink{
		ID:        id,
		FromID:    fromID,
		ToID:      toID,
		Relation:  relation,
		Weight:    weight,
		Auto:      auto,
		CreatedAt: now,
	}, nil
}

// GetLinks returns all links from or to the given memory ID.
// direction: "from" (outbound), "to" (inbound), "both" (all).
func (c *Client) GetLinks(ctx context.Context, memoryID string, direction string) ([]*MemoryLink, error) {
	var query string
	var args []any

	switch direction {
	case "from":
		query = `SELECT id, from_id, to_id, relation, weight, auto, created_at FROM memory_links WHERE from_id = ? ORDER BY created_at DESC`
		args = []any{memoryID}
	case "to":
		query = `SELECT id, from_id, to_id, relation, weight, auto, created_at FROM memory_links WHERE to_id = ? ORDER BY created_at DESC`
		args = []any{memoryID}
	default: // "both"
		query = `SELECT id, from_id, to_id, relation, weight, auto, created_at FROM memory_links WHERE from_id = ? OR to_id = ? ORDER BY created_at DESC`
		args = []any{memoryID, memoryID}
	}

	rows, err := c.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("getting links: %w", err)
	}
	defer rows.Close()

	return scanLinks(rows)
}

// DeleteLink removes a link by ID.
func (c *Client) DeleteLink(ctx context.Context, linkID string) error {
	result, err := c.DB.ExecContext(ctx, "DELETE FROM memory_links WHERE id = ?", linkID)
	if err != nil {
		return fmt.Errorf("deleting link: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// TraverseGraph does a BFS from startID up to maxDepth hops, returning all
// reachable memory IDs (excluding the start node).
func (c *Client) TraverseGraph(ctx context.Context, startID string, maxDepth int) ([]string, error) {
	if maxDepth <= 0 {
		maxDepth = 1
	}

	visited := map[string]bool{startID: true}
	frontier := []string{startID}

	for depth := 0; depth < maxDepth && len(frontier) > 0; depth++ {
		var nextFrontier []string
		for _, nodeID := range frontier {
			links, err := c.GetLinks(ctx, nodeID, "both")
			if err != nil {
				return nil, fmt.Errorf("traversing from %s: %w", nodeID, err)
			}
			for _, link := range links {
				neighbor := link.ToID
				if neighbor == nodeID {
					neighbor = link.FromID
				}
				if !visited[neighbor] {
					visited[neighbor] = true
					nextFrontier = append(nextFrontier, neighbor)
				}
			}
		}
		frontier = nextFrontier
	}

	// Collect all visited except start
	var result []string
	for id := range visited {
		if id != startID {
			result = append(result, id)
		}
	}
	return result, nil
}

func scanLinks(rows *sql.Rows) ([]*MemoryLink, error) {
	var links []*MemoryLink
	for rows.Next() {
		l := &MemoryLink{}
		var autoInt int
		if err := rows.Scan(&l.ID, &l.FromID, &l.ToID, &l.Relation, &l.Weight, &autoInt, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning link: %w", err)
		}
		l.Auto = autoInt != 0
		links = append(links, l)
	}
	return links, nil
}
