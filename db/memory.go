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
	// Visibility controls access: "private" (owner only, never via HTTP API),
	// "internal" (default, all Claude instances), "public" (any consumer)
	Visibility string `json:"visibility"`
	Source     string `json:"source"`
	SourceFile string `json:"sourceFile"`
	ParentID   string `json:"parentId"`
	ChunkIndex int    `json:"chunkIndex"`
	// Speaker is who said/wrote this: j33p, gilfoyle, agent, system
	Speaker string `json:"speaker,omitempty"`
	// Area is the top-level domain: work, home, family, homelab, project, meta
	Area string `json:"area,omitempty"`
	// SubArea is a free-form sub-domain (power-platform, proxmox, magi, etc.)
	SubArea   string   `json:"subArea,omitempty"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
	ArchivedAt string  `json:"archivedAt,omitempty"`
	TokenCount int     `json:"tokenCount"`
	Tags       []string `json:"tags,omitempty"`
}

// MemoryFilter defines search/filter criteria for listing memories.
type MemoryFilter struct {
	// Project filters to a single namespace (e.g. "agent:gilfoyle", "crew:shared").
	// Takes precedence over Projects if both are set.
	Project string
	// Projects filters to any of the listed namespaces — useful for agents that
	// want to query their own context AND shared crew memory in one call.
	// Example: []string{"agent:dinesh", "crew:shared"}
	Projects []string
	Type       string
	Tags       []string
	Limit      int
	Offset     int
	// Visibility filters by access level. If empty, defaults to excluding "private"
	// for HTTP API callers. Set to "all" to include private (MCP/internal use only).
	Visibility string
	// Speaker filters by who said/wrote it (j33p, gilfoyle, agent, system).
	Speaker string
	// Area filters by top-level domain (work, home, family, homelab, project, meta).
	Area string
	// SubArea filters by sub-domain (power-platform, proxmox, magi, etc.).
	SubArea string
	// AfterTime filters to memories created after this time.
	AfterTime *time.Time
	// BeforeTime filters to memories created before this time.
	BeforeTime *time.Time
}

// HybridResult wraps a Memory with scores from both retrieval methods.
type HybridResult struct {
	Memory        *Memory `json:"memory"`
	RRFScore      float64 `json:"rrfScore"`                // higher = more relevant
	VecRank       int     `json:"vecRank"`                  // 0 = not in vector results
	BM25Rank      int     `json:"bm25Rank"`                 // 0 = not in BM25 results
	Distance      float64 `json:"distance"`                 // cosine distance (lower = closer)
	Score         float64 `json:"score"`                    // relevance score: 1.0 - distance (higher = more relevant)
	RecencyWeight float64 `json:"recencyWeight,omitempty"`  // exp(-decay * days_old), 0 if recency weighting disabled
	WeightedScore float64 `json:"weightedScore,omitempty"` // score * recencyWeight, 0 if disabled
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
	visibility := m.Visibility
	if visibility == "" {
		visibility = "internal"
	}

	err := c.DB.QueryRow(`
		INSERT INTO memories (content, summary, embedding, project, type, visibility, source, source_file, parent_id, chunk_index, speaker, area, sub_area, created_at, updated_at, token_count)
		VALUES (?, ?, vector32(?), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`,
		m.Content,
		nullString(m.Summary),
		float32sToBytes(m.Embedding),
		m.Project,
		m.Type,
		visibility,
		nullString(m.Source),
		nullString(m.SourceFile),
		nullString(m.ParentID),
		m.ChunkIndex,
		m.Speaker,
		m.Area,
		m.SubArea,
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
		SELECT id, content, summary, project, type, visibility, source, source_file, parent_id, chunk_index,
		       speaker, area, sub_area, created_at, updated_at, archived_at, token_count
		FROM memories WHERE id = ?
	`, id).Scan(
		&m.ID, &m.Content, &summary, &m.Project, &m.Type, &m.Visibility,
		&source, &sourceFile, &parentID, &m.ChunkIndex,
		&m.Speaker, &m.Area, &m.SubArea,
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

	appendProjectCondition(filter, &conditions, &args)
	appendTaxonomyConditions(filter, &conditions, &args)
	appendTimeConditions(filter, &conditions, &args)
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
	appendVisibilityCondition(filter, &conditions, &args)

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}

	query := fmt.Sprintf(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count
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
		appendProjectCondition(filter, &conditions, &args)
		appendTaxonomyConditions(filter, &conditions, &args)
		appendTimeConditions(filter, &conditions, &args)
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
		appendVisibilityCondition(filter, &conditions, &args)
	}

	if topK <= 0 {
		topK = 5
	}
	args = append(args, topK)

	query := fmt.Sprintf(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count,
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
			&m.ID, &m.Content, &summary, &m.Project, &m.Type, &m.Visibility,
			&source, &sourceFile, &parentID, &m.ChunkIndex,
			&m.Speaker, &m.Area, &m.SubArea,
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
			&m.ID, &m.Content, &summary, &m.Project, &m.Type, &m.Visibility,
			&source, &sourceFile, &parentID, &m.ChunkIndex,
			&m.Speaker, &m.Area, &m.SubArea,
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

// appendProjectCondition adds project filtering to conditions/args.
// Handles single Project, multi-Projects slice, or neither.
func appendProjectCondition(filter *MemoryFilter, conditions *[]string, args *[]any) {
	if filter.Project != "" {
		*conditions = append(*conditions, "m.project = ?")
		*args = append(*args, filter.Project)
	} else if len(filter.Projects) > 0 {
		placeholders := make([]string, len(filter.Projects))
		for i, p := range filter.Projects {
			placeholders[i] = "?"
			*args = append(*args, p)
		}
		*conditions = append(*conditions, fmt.Sprintf("m.project IN (%s)", strings.Join(placeholders, ",")))
	}
}

// appendTaxonomyConditions adds speaker/area/sub_area filtering to conditions/args.
func appendTaxonomyConditions(filter *MemoryFilter, conditions *[]string, args *[]any) {
	if filter.Speaker != "" {
		*conditions = append(*conditions, "m.speaker = ?")
		*args = append(*args, filter.Speaker)
	}
	if filter.Area != "" {
		*conditions = append(*conditions, "m.area = ?")
		*args = append(*args, filter.Area)
	}
	if filter.SubArea != "" {
		*conditions = append(*conditions, "m.sub_area = ?")
		*args = append(*args, filter.SubArea)
	}
}

// appendTimeConditions adds after/before time filtering to conditions/args.
func appendTimeConditions(filter *MemoryFilter, conditions *[]string, args *[]any) {
	if filter.AfterTime != nil {
		*conditions = append(*conditions, "m.created_at > ?")
		*args = append(*args, filter.AfterTime.UTC().Format(time.RFC3339))
	}
	if filter.BeforeTime != nil {
		*conditions = append(*conditions, "m.created_at < ?")
		*args = append(*args, filter.BeforeTime.UTC().Format(time.RFC3339))
	}
}

// appendVisibilityCondition adds visibility filtering to conditions/args.
func appendVisibilityCondition(filter *MemoryFilter, conditions *[]string, args *[]any) {
	if filter.Visibility == "all" {
		return
	}
	if filter.Visibility != "" {
		*conditions = append(*conditions, "m.visibility = ?")
		*args = append(*args, filter.Visibility)
	} else {
		*conditions = append(*conditions, "m.visibility != 'private'")
	}
}

// SearchMemoriesBM25 performs full-text keyword search using the FTS5 index.
// Returns memories ranked by BM25 relevance.
func (c *Client) SearchMemoriesBM25(query string, filter *MemoryFilter, topK int) ([]*VectorResult, error) {
	var conditions []string
	var args []any

	conditions = append(conditions, "m.archived_at IS NULL")
	conditions = append(conditions, "m.rowid IN (SELECT rowid FROM memories_fts WHERE memories_fts MATCH ?)")
	args = append(args, query)

	if filter != nil {
		appendProjectCondition(filter, &conditions, &args)
		appendTaxonomyConditions(filter, &conditions, &args)
		appendTimeConditions(filter, &conditions, &args)
		if filter.Type != "" {
			conditions = append(conditions, "m.type = ?")
			args = append(args, filter.Type)
		}
		appendVisibilityCondition(filter, &conditions, &args)
	}

	if topK <= 0 {
		topK = 10
	}
	args = append(args, topK)

	q := fmt.Sprintf(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count,
		       -bm25(memories_fts) AS score
		FROM memories m
		JOIN memories_fts ON memories_fts.rowid = m.rowid
		WHERE %s
		ORDER BY score DESC
		LIMIT ?
	`, strings.Join(conditions, " AND "))

	rows, err := c.DB.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("BM25 search: %w", err)
	}
	defer rows.Close()

	var results []*VectorResult
	for rows.Next() {
		m := &Memory{}
		var summary, source, sourceFile, parentID, archivedAt sql.NullString
		var score float64

		if err := rows.Scan(
			&m.ID, &m.Content, &summary, &m.Project, &m.Type, &m.Visibility,
			&source, &sourceFile, &parentID, &m.ChunkIndex,
			&m.Speaker, &m.Area, &m.SubArea,
			&m.CreatedAt, &m.UpdatedAt, &archivedAt, &m.TokenCount,
			&score,
		); err != nil {
			return nil, fmt.Errorf("scanning BM25 result: %w", err)
		}

		m.Summary = summary.String
		m.Source = source.String
		m.SourceFile = sourceFile.String
		m.ParentID = parentID.String
		m.ArchivedAt = archivedAt.String

		// Reuse VectorResult with BM25 score as distance (inverted: lower = better, so we negate)
		results = append(results, &VectorResult{Memory: m, Distance: -score})
	}

	for _, r := range results {
		tags, err := c.GetTags(r.Memory.ID)
		if err != nil {
			return nil, err
		}
		r.Memory.Tags = tags
	}

	return results, nil
}

// HybridSearch runs both vector and BM25 search then fuses results using
// Reciprocal Rank Fusion (RRF) with k=60 (standard default).
// Returns results ordered by combined RRF score (descending).
func (c *Client) HybridSearch(embedding []float32, query string, filter *MemoryFilter, topK int) ([]*HybridResult, error) {
	if topK <= 0 {
		topK = 10
	}
	fetchK := topK * 3 // over-fetch to have enough for fusion

	vecResults, err := c.SearchMemories(embedding, filter, fetchK)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	bm25Results, err := c.SearchMemoriesBM25(query, filter, fetchK)
	if err != nil {
		return nil, fmt.Errorf("BM25 search: %w", err)
	}

	// Build RRF score map keyed by memory ID.
	const k = 60.0
	type entry struct {
		memory   *Memory
		rrfScore float64
		vecRank  int
		bm25Rank int
		distance float64
	}
	scored := make(map[string]*entry)

	for rank, r := range vecResults {
		e := &entry{memory: r.Memory, vecRank: rank + 1, distance: r.Distance}
		e.rrfScore += 1.0 / (k + float64(rank+1))
		scored[r.Memory.ID] = e
	}

	for rank, r := range bm25Results {
		if e, ok := scored[r.Memory.ID]; ok {
			e.bm25Rank = rank + 1
			e.rrfScore += 1.0 / (k + float64(rank+1))
		} else {
			scored[r.Memory.ID] = &entry{
				memory:   r.Memory,
				bm25Rank: rank + 1,
				rrfScore: 1.0 / (k + float64(rank+1)),
			}
		}
	}

	// Boost incident/lesson results when query contains diagnostic keywords.
	diagnosticBoost := hasDiagnosticKeywords(query)

	// Sort by RRF score descending.
	results := make([]*HybridResult, 0, len(scored))
	for _, e := range scored {
		rrfScore := e.rrfScore
		if diagnosticBoost && (e.memory.Type == "incident" || e.memory.Type == "lesson") {
			rrfScore *= 1.5
		}
		results = append(results, &HybridResult{
			Memory:   e.memory,
			RRFScore: rrfScore,
			VecRank:  e.vecRank,
			BM25Rank: e.bm25Rank,
			Distance: e.distance,
			Score:    1.0 - e.distance,
		})
	}
	// Simple insertion sort — result sets are small (topK*3 max)
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].RRFScore > results[j-1].RRFScore; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
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

// diagnosticKeywords are terms that suggest the user is debugging or investigating
// a past failure. When detected, incident/lesson memories get an RRF score boost.
var diagnosticKeywords = []string{
	"broke", "broken", "error", "fix", "fixed", "why", "cause", "caused",
	"failed", "failing", "failure", "offline", "issue", "bug", "crash",
	"crashed", "down", "outage", "incident", "debug", "troubleshoot",
}

// hasDiagnosticKeywords returns true if the query contains any diagnostic keyword.
func hasDiagnosticKeywords(query string) bool {
	lower := strings.ToLower(query)
	for _, kw := range diagnosticKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// GetContextMemories returns memories for session auto-injection:
// recent (7 days) memories, optionally filtered by project.
func (c *Client) GetContextMemories(project string, limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 10
	}

	var conditions []string
	var args []any

	conditions = append(conditions, "m.archived_at IS NULL")
	conditions = append(conditions, "m.created_at > datetime('now', '-7 days')")
	conditions = append(conditions, "m.visibility != 'private'")

	if project != "" {
		conditions = append(conditions, "m.project = ?")
		args = append(args, project)
	}

	args = append(args, limit)

	query := fmt.Sprintf(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count
		FROM memories m
		WHERE %s
		ORDER BY m.created_at DESC
		LIMIT ?
	`, strings.Join(conditions, " AND "))

	rows, err := c.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("getting context memories: %w", err)
	}
	defer rows.Close()

	return scanMemories(rows)
}

// FindSimilar returns the single closest non-archived memory by cosine distance.
// Returns nil if no memories exist or the closest distance exceeds maxDistance.
func (c *Client) FindSimilar(embedding []float32, maxDistance float64) (*VectorResult, error) {
	var m Memory
	var summary, source, sourceFile, parentID, archivedAt sql.NullString
	var distance float64

	err := c.DB.QueryRow(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count,
		       vector_distance_cos(m.embedding, vector32(?)) AS distance
		FROM memories m
		WHERE m.archived_at IS NULL
		ORDER BY distance ASC
		LIMIT 1
	`, float32sToBytes(embedding)).Scan(
		&m.ID, &m.Content, &summary, &m.Project, &m.Type, &m.Visibility,
		&source, &sourceFile, &parentID, &m.ChunkIndex,
		&m.Speaker, &m.Area, &m.SubArea,
		&m.CreatedAt, &m.UpdatedAt, &archivedAt, &m.TokenCount,
		&distance,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finding similar memory: %w", err)
	}

	if distance > maxDistance {
		return nil, nil
	}

	m.Summary = summary.String
	m.Source = source.String
	m.SourceFile = sourceFile.String
	m.ParentID = parentID.String
	m.ArchivedAt = archivedAt.String

	tags, err := c.GetTags(m.ID)
	if err != nil {
		return nil, err
	}
	m.Tags = tags

	return &VectorResult{Memory: &m, Distance: distance}, nil
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
