package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Compile-time check: MySQLClient must implement Store.
var _ Store = (*MySQLClient)(nil)

// MySQLClient implements Store using a MySQL/MariaDB database.
// Embeddings are stored as BLOBs and vector search is performed in Go
// (MySQL has no native vector type). BM25 search uses FULLTEXT indexes.
type MySQLClient struct {
	DB     *sql.DB
	logger *slog.Logger
}

// NewMySQLClient opens a MySQL connection using the given DSN.
// The DSN should include parseTime=true for proper time scanning.
func NewMySQLClient(dsn string, logger *slog.Logger) (*MySQLClient, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening MySQL database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging MySQL database: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(10 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	return &MySQLClient{
		DB:     db,
		logger: logger,
	}, nil
}

// newHexID generates a 32-character hex string (128-bit random ID),
// matching the format of SQLite's hex(randomblob(16)).
func newHexID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// bytesToFloat32s converts a little-endian byte slice back to []float32.
func bytesToFloat32s(b []byte) []float32 {
	if len(b) == 0 || len(b)%4 != 0 {
		return nil
	}
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}

// --- Core CRUD ---

// SaveMemory inserts a new memory and returns it with the generated ID.
func (c *MySQLClient) SaveMemory(m *Memory) (*Memory, error) {
	now := time.Now().UTC().Format(time.DateTime)
	id := newHexID()

	visibility := m.Visibility
	if visibility == "" {
		visibility = "internal"
	}

	_, err := c.DB.Exec(`
		INSERT INTO memories (id, content, summary, embedding, project, type, visibility, source, source_file, parent_id, chunk_index, speaker, area, sub_area, created_at, updated_at, token_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		id,
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
	)
	if err != nil {
		return nil, fmt.Errorf("inserting memory: %w", err)
	}

	m.ID = id
	m.Visibility = visibility
	m.CreatedAt = now
	m.UpdatedAt = now
	return m, nil
}

// GetMemory retrieves a single memory by ID.
func (c *MySQLClient) GetMemory(id string) (*Memory, error) {
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
func (c *MySQLClient) UpdateMemory(m *Memory) error {
	now := time.Now().UTC().Format(time.DateTime)

	if m.Embedding != nil {
		_, err := c.DB.Exec(`
			UPDATE memories
			SET content = ?, summary = ?, embedding = ?, type = ?, updated_at = ?, token_count = ?
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
func (c *MySQLClient) ArchiveMemory(id string) error {
	now := time.Now().UTC().Format(time.DateTime)
	_, err := c.DB.Exec("UPDATE memories SET archived_at = ? WHERE id = ?", now, id)
	return err
}

// DeleteMemory permanently removes a memory and its tags.
func (c *MySQLClient) DeleteMemory(id string) error {
	_, err := c.DB.Exec("DELETE FROM memories WHERE id = ?", id)
	return err
}

// ListMemories returns memories matching the given filter criteria.
func (c *MySQLClient) ListMemories(filter *MemoryFilter) ([]*Memory, error) {
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

	return c.scanMemories(rows)
}

// CountMemories returns the total number of memories matching the filter.
func (c *MySQLClient) CountMemories(filter *MemoryFilter) (int, error) {
	if filter == nil {
		filter = &MemoryFilter{}
	}
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

	query := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM memories m
		WHERE %s
	`, strings.Join(conditions, " AND "))

	var count int
	if err := c.DB.QueryRow(query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting memories: %w", err)
	}
	return count, nil
}

// --- Search ---

// SearchMemories performs vector similarity search by loading all embeddings
// and computing cosine distance in Go (MySQL has no native vector ops).
func (c *MySQLClient) SearchMemories(embedding []float32, filter *MemoryFilter, topK int) ([]*VectorResult, error) {
	var conditions []string
	var args []any

	conditions = append(conditions, "m.archived_at IS NULL")
	conditions = append(conditions, "m.embedding IS NOT NULL")

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

	query := fmt.Sprintf(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count,
		       m.embedding
		FROM memories m
		WHERE %s
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
		var embBlob []byte

		if err := rows.Scan(
			&m.ID, &m.Content, &summary, &m.Project, &m.Type, &m.Visibility,
			&source, &sourceFile, &parentID, &m.ChunkIndex,
			&m.Speaker, &m.Area, &m.SubArea,
			&m.CreatedAt, &m.UpdatedAt, &archivedAt, &m.TokenCount,
			&embBlob,
		); err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}

		m.Summary = summary.String
		m.Source = source.String
		m.SourceFile = sourceFile.String
		m.ParentID = parentID.String
		m.ArchivedAt = archivedAt.String

		stored := bytesToFloat32s(embBlob)
		if stored == nil {
			continue
		}

		dist := cosineDistance(embedding, stored)
		results = append(results, &VectorResult{Memory: m, Distance: dist})
	}

	// Sort by distance ascending (most similar first).
	sort.Slice(results, func(i, j int) bool {
		return results[i].Distance < results[j].Distance
	})

	if len(results) > topK {
		results = results[:topK]
	}

	// Load tags for each result.
	for _, r := range results {
		tags, err := c.GetTags(r.Memory.ID)
		if err != nil {
			return nil, err
		}
		r.Memory.Tags = tags
	}

	return results, nil
}

// SearchMemoriesBM25 performs full-text keyword search using MySQL FULLTEXT index.
func (c *MySQLClient) SearchMemoriesBM25(query string, filter *MemoryFilter, topK int) ([]*VectorResult, error) {
	var conditions []string
	var args []any

	conditions = append(conditions, "m.archived_at IS NULL")
	conditions = append(conditions, "MATCH(m.content) AGAINST(? IN NATURAL LANGUAGE MODE)")
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

	// Add the query again for scoring in SELECT clause.
	scoreArgs := []any{query}
	allArgs := append(scoreArgs, args...)
	allArgs = append(allArgs, topK)

	q := fmt.Sprintf(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count,
		       MATCH(m.content) AGAINST(? IN NATURAL LANGUAGE MODE) AS score
		FROM memories m
		WHERE %s
		ORDER BY score DESC
		LIMIT ?
	`, strings.Join(conditions, " AND "))

	rows, err := c.DB.Query(q, allArgs...)
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

		// Reuse VectorResult with BM25 score as distance (inverted: lower = better)
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

// HybridSearch runs both vector and BM25 search concurrently then fuses
// results using Reciprocal Rank Fusion (RRF) with k=60.
func (c *MySQLClient) HybridSearch(embedding []float32, query string, filter *MemoryFilter, topK int) ([]*HybridResult, error) {
	if topK <= 0 {
		topK = 10
	}
	fetchK := topK * 3

	var (
		vecResults  []*VectorResult
		bm25Results []*VectorResult
		vecErr      error
		bm25Err     error
		wg          sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		vecResults, vecErr = c.SearchMemories(embedding, filter, fetchK)
	}()
	go func() {
		defer wg.Done()
		bm25Results, bm25Err = c.SearchMemoriesBM25(query, filter, fetchK)
	}()
	wg.Wait()

	if vecErr != nil {
		return nil, fmt.Errorf("vector search: %w", vecErr)
	}
	if bm25Err != nil {
		return nil, fmt.Errorf("BM25 search: %w", bm25Err)
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

	// Sort by RRF score descending.
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

// GetContextMemories returns recent (7 days) memories, optionally filtered by project.
func (c *MySQLClient) GetContextMemories(project string, limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 10
	}

	var conditions []string
	var args []any

	conditions = append(conditions, "m.archived_at IS NULL")
	conditions = append(conditions, "m.created_at > NOW() - INTERVAL 7 DAY")
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

	return c.scanMemories(rows)
}

// FindSimilar returns the single closest non-archived memory by cosine distance.
// Returns nil if no memories exist or the closest distance exceeds maxDistance.
func (c *MySQLClient) FindSimilar(embedding []float32, maxDistance float64) (*VectorResult, error) {
	return c.findSimilarWithProject("", embedding, maxDistance)
}

// FindSimilarInProject is project-scoped similarity lookup.
func (c *MySQLClient) FindSimilarInProject(project string, embedding []float32, maxDistance float64) (*VectorResult, error) {
	return c.findSimilarWithProject(project, embedding, maxDistance)
}

func (c *MySQLClient) findSimilarWithProject(project string, embedding []float32, maxDistance float64) (*VectorResult, error) {
	query := `
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count,
		       m.embedding
		FROM memories m
		WHERE m.archived_at IS NULL AND m.embedding IS NOT NULL`
	args := []any{}
	if project != "" {
		query += " AND m.project = ?"
		args = append(args, project)
	}

	rows, err := c.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("finding similar memory: %w", err)
	}
	defer rows.Close()

	var best *VectorResult
	for rows.Next() {
		m := &Memory{}
		var summary, source, sourceFile, parentID, archivedAt sql.NullString
		var embBlob []byte

		if err := rows.Scan(
			&m.ID, &m.Content, &summary, &m.Project, &m.Type, &m.Visibility,
			&source, &sourceFile, &parentID, &m.ChunkIndex,
			&m.Speaker, &m.Area, &m.SubArea,
			&m.CreatedAt, &m.UpdatedAt, &archivedAt, &m.TokenCount,
			&embBlob,
		); err != nil {
			return nil, fmt.Errorf("scanning similar result: %w", err)
		}

		m.Summary = summary.String
		m.Source = source.String
		m.SourceFile = sourceFile.String
		m.ParentID = parentID.String
		m.ArchivedAt = archivedAt.String

		stored := bytesToFloat32s(embBlob)
		if stored == nil {
			continue
		}

		dist := cosineDistance(embedding, stored)
		if dist > maxDistance {
			continue
		}
		if best == nil || dist < best.Distance {
			best = &VectorResult{Memory: m, Distance: dist}
		}
	}

	if best == nil {
		return nil, nil
	}

	tags, err := c.GetTags(best.Memory.ID)
	if err != nil {
		return nil, err
	}
	best.Memory.Tags = tags

	return best, nil
}

// --- Tags ---

// ExistsWithContentHash returns the memory ID that has the given hash tag, or "" if none.
func (c *MySQLClient) ExistsWithContentHash(hash string) (string, error) {
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
func (c *MySQLClient) GetTags(memoryID string) ([]string, error) {
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

// SetTags replaces all tags for a memory atomically.
func (c *MySQLClient) SetTags(memoryID string, tags []string) error {
	tx, err := c.DB.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec("DELETE FROM memory_tags WHERE memory_id = ?", memoryID); err != nil {
		return fmt.Errorf("clearing tags: %w", err)
	}

	tags = normalizeTags(tags)
	if len(tags) == 0 {
		return tx.Commit()
	}

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

// --- Links ---

// CreateLink creates a directed link between two memories.
func (c *MySQLClient) CreateLink(ctx context.Context, fromID, toID, relation string, weight float64, auto bool) (*MemoryLink, error) {
	now := time.Now().UTC().Format(time.DateTime)
	id := newHexID()
	autoInt := 0
	if auto {
		autoInt = 1
	}

	_, err := c.DB.ExecContext(ctx, `
		INSERT INTO memory_links (id, from_id, to_id, relation, weight, auto, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, fromID, toID, relation, weight, autoInt, now)
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
func (c *MySQLClient) GetLinks(ctx context.Context, memoryID string, direction string) ([]*MemoryLink, error) {
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
func (c *MySQLClient) DeleteLink(ctx context.Context, linkID string) error {
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
func (c *MySQLClient) TraverseGraph(ctx context.Context, startID string, maxDepth int) ([]string, error) {
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

	var result []string
	for id := range visited {
		if id != startID {
			result = append(result, id)
		}
	}
	return result, nil
}

// GetGraphData returns nodes and edges for graph visualization.
func (c *MySQLClient) GetGraphData(ctx context.Context, topN int) ([]*Memory, []*MemoryLink, error) {
	rows, err := c.DB.QueryContext(ctx, `
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source,
		       m.source_file, m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count,
		       COALESCE(lc.cnt, 0) as link_count
		FROM memories m
		LEFT JOIN (
			SELECT memory_id, SUM(cnt) as cnt FROM (
				SELECT from_id as memory_id, COUNT(*) as cnt FROM memory_links GROUP BY from_id
				UNION ALL
				SELECT to_id as memory_id, COUNT(*) as cnt FROM memory_links GROUP BY to_id
			) sub GROUP BY memory_id
		) lc ON lc.memory_id = m.id
		WHERE m.archived_at IS NULL
		ORDER BY link_count DESC, m.created_at DESC
		LIMIT ?
	`, topN)
	if err != nil {
		return nil, nil, fmt.Errorf("getting graph nodes: %w", err)
	}
	defer rows.Close()

	nodeMap := make(map[string]bool)
	var memories []*Memory
	for rows.Next() {
		m := &Memory{}
		var nSummary, nSource, nSourceFile, nParentID, nSpeaker, nArea, nSubArea, archived sql.NullString
		var linkCount int
		if err := rows.Scan(
			&m.ID, &m.Content, &nSummary, &m.Project, &m.Type, &m.Visibility,
			&nSource, &nSourceFile, &nParentID, &m.ChunkIndex, &nSpeaker,
			&nArea, &nSubArea, &m.CreatedAt, &m.UpdatedAt, &archived, &m.TokenCount,
			&linkCount,
		); err != nil {
			return nil, nil, fmt.Errorf("scanning graph node: %w", err)
		}
		m.Summary = nSummary.String
		m.Source = nSource.String
		m.SourceFile = nSourceFile.String
		m.ParentID = nParentID.String
		m.Speaker = nSpeaker.String
		m.Area = nArea.String
		m.SubArea = nSubArea.String
		if archived.Valid {
			m.ArchivedAt = archived.String
		}
		_ = linkCount
		nodeMap[m.ID] = true
		memories = append(memories, m)
	}
	rows.Close()

	if len(memories) == 0 {
		return memories, nil, nil
	}

	linkRows, err := c.DB.QueryContext(ctx, `
		SELECT id, from_id, to_id, relation, weight, auto, created_at
		FROM memory_links
	`)
	if err != nil {
		return nil, nil, fmt.Errorf("getting graph edges: %w", err)
	}
	defer linkRows.Close()

	allLinks, err := scanLinks(linkRows)
	if err != nil {
		return nil, nil, err
	}

	var links []*MemoryLink
	for _, l := range allLinks {
		if nodeMap[l.FromID] && nodeMap[l.ToID] {
			links = append(links, l)
		}
	}

	return memories, links, nil
}

// --- Migrations ---

// Migrate runs all MySQL database migrations. Safe to call on every startup.
func (c *MySQLClient) Migrate() error {
	// Create schema_migrations table.
	_, err := c.DB.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INT NOT NULL PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB
	`)
	if err != nil {
		return fmt.Errorf("creating meta table: %w", err)
	}

	migrations := []struct {
		version int
		sql     string
	}{
		{1, mysqlMigrationV1},
		{2, mysqlMigrationV2},
		{3, mysqlMigrationV3},
		{4, mysqlMigrationV4},
		{5, mysqlMigrationV5},
		{6, mysqlMigrationV6},
		{7, mysqlMigrationV7},
		{8, mysqlMigrationV8},
		{9, mysqlMigrationV9},
	}

	for _, m := range migrations {
		var count int
		if err := c.DB.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", m.version).Scan(&count); err != nil {
			return fmt.Errorf("checking migration %d: %w", m.version, err)
		}
		if count > 0 {
			continue
		}

		// Skip empty migrations (e.g. V3 is a no-op for MySQL).
		if strings.TrimSpace(m.sql) != "" {
			if err := c.execMulti(m.sql); err != nil {
				return fmt.Errorf("running migration %d: %w", m.version, err)
			}
		}

		if _, err := c.DB.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
			return fmt.Errorf("marking migration %d: %w", m.version, err)
		}

		c.logger.Info("Applied migration", "version", m.version)
	}

	return nil
}

// execMulti splits a SQL string into individual statements and executes each one.
func (c *MySQLClient) execMulti(sql string) error {
	statements := splitSQL(sql)
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := c.DB.Exec(stmt); err != nil {
			return fmt.Errorf("executing statement: %w\nSQL: %s", err, stmt)
		}
	}
	return nil
}

// --- Lifecycle ---

// Close shuts down the MySQL database connection.
func (c *MySQLClient) Close() error {
	return c.DB.Close()
}

// --- Internal helpers ---

// scanMemories scans rows into Memory slices (MySQL variant with string timestamps).
func (c *MySQLClient) scanMemories(rows *sql.Rows) ([]*Memory, error) {
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
