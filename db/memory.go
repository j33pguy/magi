package db

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"
)

// Memory represents a stored memory record.
type Memory struct {
	ID         string    `json:"id"`
	Content    string    `json:"content"`
	Summary    string    `json:"summary"`
	Embedding  []float32 `json:"embedding,omitempty"`
	Project    string    `json:"project"`
	Type       string    `json:"type"`
	Source     string    `json:"source"`
	SourceFile string    `json:"sourceFile"`
	ParentID   string    `json:"parentId"`
	ChunkIndex int       `json:"chunkIndex"`
	CreatedAt  string    `json:"createdAt"`
	UpdatedAt  string    `json:"updatedAt"`
	ArchivedAt string    `json:"archivedAt,omitempty"`
	TokenCount int       `json:"tokenCount"`
	Tags       []string  `json:"tags,omitempty"`
}

// MemoryFilter defines search/filter criteria for listing memories.
type MemoryFilter struct {
	Project string
	Type    string
	Tags    []string
	Limit   int
	Offset  int
}

// VectorResult wraps a Memory with its similarity distance.
type VectorResult struct {
	Memory   *Memory `json:"memory"`
	Distance float64 `json:"distance"`
}

// SaveMemory inserts a new memory and returns it with the generated ID.
func (c *Client) SaveMemory(m *Memory) (*Memory, error) {
	now := time.Now().UTC().Format(time.DateTime)

	var id string
	err := c.DB.QueryRow(`
		INSERT INTO memories (content, summary, embedding, project, type, source, source_file, parent_id, chunk_index, created_at, updated_at, token_count)
		VALUES (?, ?, vector32(?), ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`,
		m.Content,
		nullString(m.Summary),
		float32sToBytes(m.Embedding),
		m.Project,
		m.Type,
		nullString(m.Source),
		nullString(m.SourceFile),
		nullString(m.ParentID),
		m.ChunkIndex,
		now,
		now,
		m.TokenCount,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("inserting memory: %w", err)
	}

	m.ID = id
	m.CreatedAt = now
	m.UpdatedAt = now
	return m, nil
}

// GetMemory retrieves a single memory by ID.
func (c *Client) GetMemory(id string) (*Memory, error) {
	m := &Memory{}
	var summary, source, sourceFile, parentID, archivedAt sql.NullString

	err := c.DB.QueryRow(`
		SELECT id, content, summary, project, type, source, source_file, parent_id, chunk_index,
		       created_at, updated_at, archived_at, token_count
		FROM memories WHERE id = ?
	`, id).Scan(
		&m.ID, &m.Content, &summary, &m.Project, &m.Type,
		&source, &sourceFile, &parentID, &m.ChunkIndex,
		&m.CreatedAt, &m.UpdatedAt, &archivedAt, &m.TokenCount,
	)
	if err != nil {
		return nil, fmt.Errorf("getting memory %s: %w", id, err)
	}

	m.Summary = summary.String
	m.Source = source.String
	m.SourceFile = sourceFile.String
	m.ParentID = parentID.String
	m.ArchivedAt = archivedAt.String

	tags, err := c.GetTags(id)
	if err != nil {
		return nil, err
	}
	m.Tags = tags

	return m, nil
}

// UpdateMemory updates a memory's content, metadata, and optionally re-embeds.
func (c *Client) UpdateMemory(m *Memory) error {
	now := time.Now().UTC().Format(time.DateTime)

	if m.Embedding != nil {
		_, err := c.DB.Exec(`
			UPDATE memories
			SET content = ?, summary = ?, embedding = vector32(?), type = ?, updated_at = ?, token_count = ?
			WHERE id = ?
		`,
			m.Content, nullString(m.Summary), float32sToBytes(m.Embedding),
			m.Type, now, m.TokenCount, m.ID,
		)
		return err
	}

	_, err := c.DB.Exec(`
		UPDATE memories
		SET content = ?, summary = ?, type = ?, updated_at = ?
		WHERE id = ?
	`,
		m.Content, nullString(m.Summary), m.Type, now, m.ID,
	)
	return err
}

// ArchiveMemory soft-deletes a memory by setting archived_at.
func (c *Client) ArchiveMemory(id string) error {
	now := time.Now().UTC().Format(time.DateTime)
	_, err := c.DB.Exec("UPDATE memories SET archived_at = ? WHERE id = ?", now, id)
	return err
}

// DeleteMemory permanently removes a memory and its tags.
func (c *Client) DeleteMemory(id string) error {
	_, err := c.DB.Exec("DELETE FROM memories WHERE id = ?", id)
	return err
}

// ListMemories returns memories matching the given filter criteria.
func (c *Client) ListMemories(filter *MemoryFilter) ([]*Memory, error) {
	var conditions []string
	var args []any

	conditions = append(conditions, "m.archived_at IS NULL")

	if filter.Project != "" {
		conditions = append(conditions, "m.project = ?")
		args = append(args, filter.Project)
	}
	if filter.Type != "" {
		conditions = append(conditions, "m.type = ?")
		args = append(args, filter.Type)
	}
	if len(filter.Tags) > 0 {
		placeholders := make([]string, len(filter.Tags))
		for i, tag := range filter.Tags {
			placeholders[i] = "?"
			args = append(args, tag)
		}
		conditions = append(conditions, fmt.Sprintf(
			"m.id IN (SELECT memory_id FROM memory_tags WHERE tag IN (%s))",
			strings.Join(placeholders, ","),
		))
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}

	query := fmt.Sprintf(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.created_at, m.updated_at, m.archived_at, m.token_count
		FROM memories m
		WHERE %s
		ORDER BY m.created_at DESC
		LIMIT ? OFFSET ?
	`, strings.Join(conditions, " AND "))

	args = append(args, limit, filter.Offset)

	rows, err := c.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing memories: %w", err)
	}
	defer rows.Close()

	return scanMemories(rows)
}

// SearchMemories performs a vector similarity search against the embedding index.
func (c *Client) SearchMemories(embedding []float32, filter *MemoryFilter, topK int) ([]*VectorResult, error) {
	var conditions []string
	var args []any

	// First arg is the query vector
	args = append(args, float32sToBytes(embedding))

	conditions = append(conditions, "m.archived_at IS NULL")

	if filter != nil {
		if filter.Project != "" {
			conditions = append(conditions, "m.project = ?")
			args = append(args, filter.Project)
		}
		if filter.Type != "" {
			conditions = append(conditions, "m.type = ?")
			args = append(args, filter.Type)
		}
		if len(filter.Tags) > 0 {
			placeholders := make([]string, len(filter.Tags))
			for i, tag := range filter.Tags {
				placeholders[i] = "?"
				args = append(args, tag)
			}
			conditions = append(conditions, fmt.Sprintf(
				"m.id IN (SELECT memory_id FROM memory_tags WHERE tag IN (%s))",
				strings.Join(placeholders, ","),
			))
		}
	}

	if topK <= 0 {
		topK = 5
	}
	args = append(args, topK)

	query := fmt.Sprintf(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.created_at, m.updated_at, m.archived_at, m.token_count,
		       vector_distance_cos(m.embedding, vector32(?)) AS distance
		FROM memories m
		WHERE %s
		ORDER BY distance ASC
		LIMIT ?
	`, strings.Join(conditions, " AND "))

	rows, err := c.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("searching memories: %w", err)
	}
	defer rows.Close()

	var results []*VectorResult
	for rows.Next() {
		m := &Memory{}
		var summary, source, sourceFile, parentID, archivedAt sql.NullString
		var distance float64

		if err := rows.Scan(
			&m.ID, &m.Content, &summary, &m.Project, &m.Type,
			&source, &sourceFile, &parentID, &m.ChunkIndex,
			&m.CreatedAt, &m.UpdatedAt, &archivedAt, &m.TokenCount,
			&distance,
		); err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}

		m.Summary = summary.String
		m.Source = source.String
		m.SourceFile = sourceFile.String
		m.ParentID = parentID.String
		m.ArchivedAt = archivedAt.String

		results = append(results, &VectorResult{Memory: m, Distance: distance})
	}

	// Load tags for each result
	for _, r := range results {
		tags, err := c.GetTags(r.Memory.ID)
		if err != nil {
			return nil, err
		}
		r.Memory.Tags = tags
	}

	return results, nil
}

func scanMemories(rows *sql.Rows) ([]*Memory, error) {
	var memories []*Memory
	for rows.Next() {
		m := &Memory{}
		var summary, source, sourceFile, parentID, archivedAt sql.NullString

		if err := rows.Scan(
			&m.ID, &m.Content, &summary, &m.Project, &m.Type,
			&source, &sourceFile, &parentID, &m.ChunkIndex,
			&m.CreatedAt, &m.UpdatedAt, &archivedAt, &m.TokenCount,
		); err != nil {
			return nil, fmt.Errorf("scanning memory: %w", err)
		}

		m.Summary = summary.String
		m.Source = source.String
		m.SourceFile = sourceFile.String
		m.ParentID = parentID.String
		m.ArchivedAt = archivedAt.String

		memories = append(memories, m)
	}
	return memories, nil
}

// float32sToBytes converts a float32 slice to little-endian bytes for F32_BLOB.
func float32sToBytes(v []float32) []byte {
	if v == nil {
		return nil
	}
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
