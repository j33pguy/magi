package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/microsoft/go-mssqldb"
)

// Compile-time check: SQLServerClient must implement Store.
var _ Store = (*SQLServerClient)(nil)

// SQLServerClient implements Store using SQL Server / Azure SQL.
// Embeddings are stored as VARBINARY(MAX); vector similarity is computed in Go.
// Full-text search uses SQL Server Full-Text Indexing (FREETEXTTABLE).
type SQLServerClient struct {
	DB     *sql.DB
	logger *slog.Logger
}

// NewSQLServerClient opens a connection to SQL Server using the provided DSN.
// DSN format: sqlserver://user:pass@host?database=dbname
func NewSQLServerClient(dsn string, logger *slog.Logger) (*SQLServerClient, error) {
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening SQL Server database: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(10 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging SQL Server: %w", err)
	}

	return &SQLServerClient{DB: db, logger: logger}, nil
}

// Close shuts down the database connection pool.
func (c *SQLServerClient) Close() error {
	return c.DB.Close()
}

// newHexID and bytesToFloat32s are defined in mysql.go and shared across
// non-SQLite backends.

// ---------------------------------------------------------------------------
// Parameterised query helpers
// ---------------------------------------------------------------------------

// mssqlParamBuilder tracks positional @p1, @p2, ... parameters.
type mssqlParamBuilder struct {
	args []any
}

// add appends a value and returns its @pN placeholder.
func (p *mssqlParamBuilder) add(v any) string {
	p.args = append(p.args, v)
	return fmt.Sprintf("@p%d", len(p.args))
}

// ---------------------------------------------------------------------------
// Filter helpers (SQL Server dialect)
// ---------------------------------------------------------------------------

func mssqlAppendProjectCondition(filter *MemoryFilter, conds *[]string, pb *mssqlParamBuilder) {
	if filter.Project != "" {
		*conds = append(*conds, "m.project = "+pb.add(filter.Project))
	} else if len(filter.Projects) > 0 {
		phs := make([]string, len(filter.Projects))
		for i, p := range filter.Projects {
			phs[i] = pb.add(p)
		}
		*conds = append(*conds, fmt.Sprintf("m.project IN (%s)", strings.Join(phs, ",")))
	}
}

func mssqlAppendTaxonomyConditions(filter *MemoryFilter, conds *[]string, pb *mssqlParamBuilder) {
	if filter.Speaker != "" {
		*conds = append(*conds, "m.speaker = "+pb.add(filter.Speaker))
	}
	if filter.Area != "" {
		*conds = append(*conds, "m.area = "+pb.add(filter.Area))
	}
	if filter.SubArea != "" {
		*conds = append(*conds, "m.sub_area = "+pb.add(filter.SubArea))
	}
}

func mssqlAppendTimeConditions(filter *MemoryFilter, conds *[]string, pb *mssqlParamBuilder) {
	if filter.AfterTime != nil {
		*conds = append(*conds, "m.created_at > "+pb.add(filter.AfterTime.UTC()))
	}
	if filter.BeforeTime != nil {
		*conds = append(*conds, "m.created_at < "+pb.add(filter.BeforeTime.UTC()))
	}
}

func mssqlAppendVisibilityCondition(filter *MemoryFilter, conds *[]string, pb *mssqlParamBuilder) {
	if filter.Visibility == "all" {
		return
	}
	if filter.Visibility != "" {
		*conds = append(*conds, "m.visibility = "+pb.add(filter.Visibility))
	} else {
		*conds = append(*conds, "m.visibility != 'private'")
	}
}

func mssqlAppendTagCondition(filter *MemoryFilter, conds *[]string, pb *mssqlParamBuilder) {
	if len(filter.Tags) == 0 {
		return
	}
	phs := make([]string, len(filter.Tags))
	for i, tag := range filter.Tags {
		phs[i] = pb.add(tag)
	}
	*conds = append(*conds, fmt.Sprintf(
		"m.id IN (SELECT memory_id FROM memory_tags WHERE tag IN (%s))",
		strings.Join(phs, ","),
	))
}

// ---------------------------------------------------------------------------
// Core CRUD
// ---------------------------------------------------------------------------

// SaveMemory inserts a new memory and returns it with the generated ID.
func (c *SQLServerClient) SaveMemory(m *Memory) (*Memory, error) {
	now := time.Now().UTC().Format(time.DateTime)
	id := newHexID()

	visibility := m.Visibility
	if visibility == "" {
		visibility = "internal"
	}

	_, err := c.DB.Exec(`
		INSERT INTO memories (id, content, summary, embedding, project, type, visibility,
			source, source_file, parent_id, chunk_index, speaker, area, sub_area,
			created_at, updated_at, token_count)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10, @p11, @p12, @p13, @p14, @p15, @p16, @p17)
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
func (c *SQLServerClient) GetMemory(id string) (*Memory, error) {
	m := &Memory{}
	var summary, source, sourceFile, parentID sql.NullString
	var archivedAt sql.NullTime

	err := c.DB.QueryRow(`
		SELECT id, content, summary, project, type, visibility, source, source_file,
		       parent_id, chunk_index, speaker, area, sub_area,
		       created_at, updated_at, archived_at, token_count
		FROM memories WHERE id = @p1
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
	if archivedAt.Valid {
		m.ArchivedAt = archivedAt.Time.UTC().Format(time.DateTime)
	}

	tags, err := c.GetTags(id)
	if err != nil {
		return nil, err
	}
	m.Tags = tags

	return m, nil
}

// UpdateMemory updates a memory's content, metadata, and optionally re-embeds.
func (c *SQLServerClient) UpdateMemory(m *Memory) error {
	now := time.Now().UTC().Format(time.DateTime)

	if m.Embedding != nil {
		_, err := c.DB.Exec(`
			UPDATE memories
			SET content = @p1, summary = @p2, embedding = @p3, type = @p4, updated_at = @p5, token_count = @p6
			WHERE id = @p7
		`,
			m.Content, nullString(m.Summary), float32sToBytes(m.Embedding),
			m.Type, now, m.TokenCount, m.ID,
		)
		return err
	}

	_, err := c.DB.Exec(`
		UPDATE memories
		SET content = @p1, summary = @p2, type = @p3, updated_at = @p4
		WHERE id = @p5
	`,
		m.Content, nullString(m.Summary), m.Type, now, m.ID,
	)
	return err
}

// ArchiveMemory soft-deletes a memory by setting archived_at.
func (c *SQLServerClient) ArchiveMemory(id string) error {
	now := time.Now().UTC().Format(time.DateTime)
	_, err := c.DB.Exec("UPDATE memories SET archived_at = @p1 WHERE id = @p2", now, id)
	return err
}

// DeleteMemory permanently removes a memory and its tags.
func (c *SQLServerClient) DeleteMemory(id string) error {
	_, err := c.DB.Exec("DELETE FROM memories WHERE id = @p1", id)
	return err
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListMemories returns memories matching the given filter criteria.
func (c *SQLServerClient) ListMemories(filter *MemoryFilter) ([]*Memory, error) {
	pb := &mssqlParamBuilder{}
	var conds []string

	conds = append(conds, "m.archived_at IS NULL")

	mssqlAppendProjectCondition(filter, &conds, pb)
	mssqlAppendTaxonomyConditions(filter, &conds, pb)
	mssqlAppendTimeConditions(filter, &conds, pb)

	if filter.Type != "" {
		conds = append(conds, "m.type = "+pb.add(filter.Type))
	}
	mssqlAppendTagCondition(filter, &conds, pb)
	mssqlAppendVisibilityCondition(filter, &conds, pb)

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}

	offsetParam := pb.add(filter.Offset)
	limitParam := pb.add(limit)

	query := fmt.Sprintf(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count
		FROM memories m
		WHERE %s
		ORDER BY m.created_at DESC
		OFFSET %s ROWS FETCH NEXT %s ROWS ONLY
	`, strings.Join(conds, " AND "), offsetParam, limitParam)

	rows, err := c.DB.Query(query, pb.args...)
	if err != nil {
		return nil, fmt.Errorf("listing memories: %w", err)
	}
	defer rows.Close()

	return c.scanMemories(rows)
}

// ---------------------------------------------------------------------------
// Vector search (computed in Go)
// ---------------------------------------------------------------------------

// SearchMemories performs vector similarity search.
// Since SQL Server has no native vector type, this loads candidate embeddings
// and computes cosine distance in Go, then sorts and returns topK results.
func (c *SQLServerClient) SearchMemories(embedding []float32, filter *MemoryFilter, topK int) ([]*VectorResult, error) {
	pb := &mssqlParamBuilder{}
	var conds []string

	conds = append(conds, "m.archived_at IS NULL")
	conds = append(conds, "m.embedding IS NOT NULL")

	if filter != nil {
		mssqlAppendProjectCondition(filter, &conds, pb)
		mssqlAppendTaxonomyConditions(filter, &conds, pb)
		mssqlAppendTimeConditions(filter, &conds, pb)
		if filter.Type != "" {
			conds = append(conds, "m.type = "+pb.add(filter.Type))
		}
		mssqlAppendTagCondition(filter, &conds, pb)
		mssqlAppendVisibilityCondition(filter, &conds, pb)
	}

	query := fmt.Sprintf(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count, m.embedding
		FROM memories m
		WHERE %s
	`, strings.Join(conds, " AND "))

	rows, err := c.DB.Query(query, pb.args...)
	if err != nil {
		return nil, fmt.Errorf("searching memories: %w", err)
	}
	defer rows.Close()

	if topK <= 0 {
		topK = 5
	}

	var results []*VectorResult
	for rows.Next() {
		m := &Memory{}
		var summary, source, sourceFile, parentID sql.NullString
		var archivedAt sql.NullTime
		var embBytes []byte

		if err := rows.Scan(
			&m.ID, &m.Content, &summary, &m.Project, &m.Type, &m.Visibility,
			&source, &sourceFile, &parentID, &m.ChunkIndex,
			&m.Speaker, &m.Area, &m.SubArea,
			&m.CreatedAt, &m.UpdatedAt, &archivedAt, &m.TokenCount, &embBytes,
		); err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}

		m.Summary = summary.String
		m.Source = source.String
		m.SourceFile = sourceFile.String
		m.ParentID = parentID.String
		if archivedAt.Valid {
			m.ArchivedAt = archivedAt.Time.UTC().Format(time.DateTime)
		}

		rowEmb := bytesToFloat32s(embBytes)
		dist := cosineDistance(embedding, rowEmb)
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

// ---------------------------------------------------------------------------
// BM25 / Full-Text Search
// ---------------------------------------------------------------------------

// SearchMemoriesBM25 performs full-text keyword search using SQL Server Full-Text Indexing.
// Results are ranked by FREETEXTTABLE RANK score.
func (c *SQLServerClient) SearchMemoriesBM25(query string, filter *MemoryFilter, topK int) ([]*VectorResult, error) {
	pb := &mssqlParamBuilder{}
	var conds []string

	conds = append(conds, "m.archived_at IS NULL")

	ftsParam := pb.add(query)

	if filter != nil {
		mssqlAppendProjectCondition(filter, &conds, pb)
		mssqlAppendTaxonomyConditions(filter, &conds, pb)
		mssqlAppendTimeConditions(filter, &conds, pb)
		if filter.Type != "" {
			conds = append(conds, "m.type = "+pb.add(filter.Type))
		}
		mssqlAppendVisibilityCondition(filter, &conds, pb)
	}

	if topK <= 0 {
		topK = 10
	}
	topKParam := pb.add(topK)

	q := fmt.Sprintf(`
		SELECT TOP(%s)
		       m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count,
		       ft.[RANK] AS score
		FROM memories m
		INNER JOIN FREETEXTTABLE(memories, content, %s) AS ft ON m.id = ft.[KEY]
		WHERE %s
		ORDER BY ft.[RANK] DESC
	`, topKParam, ftsParam, strings.Join(conds, " AND "))

	rows, err := c.DB.Query(q, pb.args...)
	if err != nil {
		return nil, fmt.Errorf("BM25 search: %w", err)
	}
	defer rows.Close()

	var results []*VectorResult
	for rows.Next() {
		m := &Memory{}
		var summary, source, sourceFile, parentID sql.NullString
		var archivedAt sql.NullTime
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
		if archivedAt.Valid {
			m.ArchivedAt = archivedAt.Time.UTC().Format(time.DateTime)
		}

		// Use negative score as distance so lower = better matches VectorResult semantics.
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

// ---------------------------------------------------------------------------
// Hybrid search (vector + BM25 with RRF fusion)
// ---------------------------------------------------------------------------

// HybridSearch runs both vector and BM25 search concurrently then fuses
// results using Reciprocal Rank Fusion (RRF) with k=60 (standard default).
func (c *SQLServerClient) HybridSearch(embedding []float32, query string, filter *MemoryFilter, topK int) ([]*HybridResult, error) {
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

	sort.Slice(results, func(i, j int) bool {
		return results[i].RRFScore > results[j].RRFScore
	})

	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

// ---------------------------------------------------------------------------
// Context memories
// ---------------------------------------------------------------------------

// GetContextMemories returns recent (7-day) non-private memories, optionally
// filtered by project, for session auto-injection.
func (c *SQLServerClient) GetContextMemories(project string, limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 10
	}

	pb := &mssqlParamBuilder{}
	var conds []string

	conds = append(conds, "m.archived_at IS NULL")
	conds = append(conds, "m.created_at > DATEADD(day, -7, SYSUTCDATETIME())")
	conds = append(conds, "m.visibility != 'private'")

	if project != "" {
		conds = append(conds, "m.project = "+pb.add(project))
	}

	topParam := pb.add(limit)

	query := fmt.Sprintf(`
		SELECT TOP(%s)
		       m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count
		FROM memories m
		WHERE %s
		ORDER BY m.created_at DESC
	`, topParam, strings.Join(conds, " AND "))

	rows, err := c.DB.Query(query, pb.args...)
	if err != nil {
		return nil, fmt.Errorf("getting context memories: %w", err)
	}
	defer rows.Close()

	return c.scanMemories(rows)
}

// ---------------------------------------------------------------------------
// FindSimilar
// ---------------------------------------------------------------------------

// FindSimilar returns the single closest non-archived memory by cosine distance.
// Returns nil if no memories exist or the closest distance exceeds maxDistance.
func (c *SQLServerClient) FindSimilar(embedding []float32, maxDistance float64) (*VectorResult, error) {
	rows, err := c.DB.Query(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count, m.embedding
		FROM memories m
		WHERE m.archived_at IS NULL AND m.embedding IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("finding similar memory: %w", err)
	}
	defer rows.Close()

	var best *VectorResult
	for rows.Next() {
		m := &Memory{}
		var summary, source, sourceFile, parentID sql.NullString
		var archivedAt sql.NullTime
		var embBytes []byte

		if err := rows.Scan(
			&m.ID, &m.Content, &summary, &m.Project, &m.Type, &m.Visibility,
			&source, &sourceFile, &parentID, &m.ChunkIndex,
			&m.Speaker, &m.Area, &m.SubArea,
			&m.CreatedAt, &m.UpdatedAt, &archivedAt, &m.TokenCount, &embBytes,
		); err != nil {
			return nil, fmt.Errorf("scanning similar result: %w", err)
		}

		m.Summary = summary.String
		m.Source = source.String
		m.SourceFile = sourceFile.String
		m.ParentID = parentID.String
		if archivedAt.Valid {
			m.ArchivedAt = archivedAt.Time.UTC().Format(time.DateTime)
		}

		rowEmb := bytesToFloat32s(embBytes)
		dist := cosineDistance(embedding, rowEmb)

		if dist <= maxDistance && (best == nil || dist < best.Distance) {
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

// ---------------------------------------------------------------------------
// Tags
// ---------------------------------------------------------------------------

// ExistsWithContentHash returns the memory ID that has the given hash tag, or "" if none.
func (c *SQLServerClient) ExistsWithContentHash(hash string) (string, error) {
	tag := "hash:" + hash
	var id string
	err := c.DB.QueryRow("SELECT TOP(1) memory_id FROM memory_tags WHERE tag = @p1", tag).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("checking content hash: %w", err)
	}
	return id, nil
}

// GetTags returns all tags for a memory.
func (c *SQLServerClient) GetTags(memoryID string) ([]string, error) {
	rows, err := c.DB.Query("SELECT tag FROM memory_tags WHERE memory_id = @p1 ORDER BY tag", memoryID)
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

// SetTags replaces all tags for a memory within a transaction.
func (c *SQLServerClient) SetTags(memoryID string, tags []string) error {
	tx, err := c.DB.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec("DELETE FROM memory_tags WHERE memory_id = @p1", memoryID); err != nil {
		return fmt.Errorf("clearing tags: %w", err)
	}

	if len(tags) == 0 {
		return tx.Commit()
	}

	// Batch all tags into a single INSERT with multiple value tuples.
	pb := &mssqlParamBuilder{}
	placeholders := make([]string, len(tags))
	for i, tag := range tags {
		midP := pb.add(memoryID)
		tagP := pb.add(tag)
		placeholders[i] = fmt.Sprintf("(%s, %s)", midP, tagP)
	}

	q := fmt.Sprintf("INSERT INTO memory_tags (memory_id, tag) VALUES %s", strings.Join(placeholders, ", "))
	if _, err := tx.Exec(q, pb.args...); err != nil {
		return fmt.Errorf("inserting tags: %w", err)
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Links
// ---------------------------------------------------------------------------

// CreateLink creates a directed link between two memories.
func (c *SQLServerClient) CreateLink(ctx context.Context, fromID, toID, relation string, weight float64, auto bool) (*MemoryLink, error) {
	now := time.Now().UTC().Format(time.DateTime)
	id := newHexID()

	autoBit := 0
	if auto {
		autoBit = 1
	}

	_, err := c.DB.ExecContext(ctx, `
		INSERT INTO memory_links (id, from_id, to_id, relation, weight, auto, created_at)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7)
	`, id, fromID, toID, relation, weight, autoBit, now)
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
func (c *SQLServerClient) GetLinks(ctx context.Context, memoryID string, direction string) ([]*MemoryLink, error) {
	var query string
	var args []any

	switch direction {
	case "from":
		query = `SELECT id, from_id, to_id, relation, weight, auto, created_at FROM memory_links WHERE from_id = @p1 ORDER BY created_at DESC`
		args = []any{memoryID}
	case "to":
		query = `SELECT id, from_id, to_id, relation, weight, auto, created_at FROM memory_links WHERE to_id = @p1 ORDER BY created_at DESC`
		args = []any{memoryID}
	default: // "both"
		query = `SELECT id, from_id, to_id, relation, weight, auto, created_at FROM memory_links WHERE from_id = @p1 OR to_id = @p2 ORDER BY created_at DESC`
		args = []any{memoryID, memoryID}
	}

	rows, err := c.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("getting links: %w", err)
	}
	defer rows.Close()

	return c.scanLinks(rows)
}

// DeleteLink removes a link by ID.
func (c *SQLServerClient) DeleteLink(ctx context.Context, linkID string) error {
	result, err := c.DB.ExecContext(ctx, "DELETE FROM memory_links WHERE id = @p1", linkID)
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
func (c *SQLServerClient) TraverseGraph(ctx context.Context, startID string, maxDepth int) ([]string, error) {
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
// Limits to topN memories by link count (combined inbound + outbound).
func (c *SQLServerClient) GetGraphData(ctx context.Context, topN int) ([]*Memory, []*MemoryLink, error) {
	rows, err := c.DB.QueryContext(ctx, `
		SELECT TOP(@p1)
		       m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source,
		       m.source_file, m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count,
		       ISNULL(lc.cnt, 0) AS link_count
		FROM memories m
		LEFT JOIN (
			SELECT memory_id, SUM(cnt) AS cnt FROM (
				SELECT from_id AS memory_id, COUNT(*) AS cnt FROM memory_links GROUP BY from_id
				UNION ALL
				SELECT to_id AS memory_id, COUNT(*) AS cnt FROM memory_links GROUP BY to_id
			) sub GROUP BY memory_id
		) lc ON lc.memory_id = m.id
		WHERE m.archived_at IS NULL
		ORDER BY link_count DESC, m.created_at DESC
	`, topN)
	if err != nil {
		return nil, nil, fmt.Errorf("getting graph nodes: %w", err)
	}
	defer rows.Close()

	nodeMap := make(map[string]bool)
	var memories []*Memory
	for rows.Next() {
		m := &Memory{}
		var nSummary, nSource, nSourceFile, nParentID sql.NullString
		var archivedAt sql.NullTime
		var linkCount int
		if err := rows.Scan(
			&m.ID, &m.Content, &nSummary, &m.Project, &m.Type, &m.Visibility,
			&nSource, &nSourceFile, &nParentID, &m.ChunkIndex, &m.Speaker,
			&m.Area, &m.SubArea, &m.CreatedAt, &m.UpdatedAt, &archivedAt, &m.TokenCount,
			&linkCount,
		); err != nil {
			return nil, nil, fmt.Errorf("scanning graph node: %w", err)
		}
		m.Summary = nSummary.String
		m.Source = nSource.String
		m.SourceFile = nSourceFile.String
		m.ParentID = nParentID.String
		if archivedAt.Valid {
			m.ArchivedAt = archivedAt.Time.UTC().Format(time.DateTime)
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

	allLinks, err := c.scanLinks(linkRows)
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

// ---------------------------------------------------------------------------
// Migrations
// ---------------------------------------------------------------------------

// Migrate runs all SQL Server database migrations. Safe to call on every startup.
func (c *SQLServerClient) Migrate() error {
	// Create schema_migrations table if it doesn't exist.
	_, err := c.DB.Exec(`
		IF OBJECT_ID('schema_migrations', 'U') IS NULL
		CREATE TABLE schema_migrations (
			version INT PRIMARY KEY,
			applied_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME()
		)
	`)
	if err != nil {
		return fmt.Errorf("creating schema_migrations table: %w", err)
	}

	migrations := []struct {
		version int
		sql     string
	}{
		{1, mssqlMigrationV1},
		{2, mssqlMigrationV2},
		{3, mssqlMigrationV3},
		{4, mssqlMigrationV4},
		{5, mssqlMigrationV5},
		{6, mssqlMigrationV6},
		{7, mssqlMigrationV7},
	}

	for _, m := range migrations {
		var count int
		if err := c.DB.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = @p1", m.version).Scan(&count); err != nil {
			return fmt.Errorf("checking migration %d: %w", m.version, err)
		}
		if count > 0 {
			continue
		}

		if err := c.execMulti(m.sql); err != nil {
			return fmt.Errorf("running migration %d: %w", m.version, err)
		}

		if _, err := c.DB.Exec("INSERT INTO schema_migrations (version) VALUES (@p1)", m.version); err != nil {
			return fmt.Errorf("marking migration %d: %w", m.version, err)
		}

		c.logger.Info("Applied migration", "version", m.version)
	}

	return nil
}

// execMulti splits a SQL string on semicolons (respecting BEGIN...END blocks)
// and executes each statement individually.
func (c *SQLServerClient) execMulti(sql string) error {
	stmts := splitSQL(sql)
	for _, stmt := range stmts {
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

// ---------------------------------------------------------------------------
// Row scanners
// ---------------------------------------------------------------------------

// scanMemories scans rows into Memory slices, handling SQL Server DATETIME2
// nullable columns with sql.NullTime.
func (c *SQLServerClient) scanMemories(rows *sql.Rows) ([]*Memory, error) {
	var memories []*Memory
	for rows.Next() {
		m := &Memory{}
		var summary, source, sourceFile, parentID sql.NullString
		var archivedAt sql.NullTime

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
		if archivedAt.Valid {
			m.ArchivedAt = archivedAt.Time.UTC().Format(time.DateTime)
		}

		memories = append(memories, m)
	}
	return memories, nil
}

// scanLinks scans link rows, converting BIT auto column to bool.
func (c *SQLServerClient) scanLinks(rows *sql.Rows) ([]*MemoryLink, error) {
	var links []*MemoryLink
	for rows.Next() {
		l := &MemoryLink{}
		var autoBit bool
		if err := rows.Scan(&l.ID, &l.FromID, &l.ToID, &l.Relation, &l.Weight, &autoBit, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning link: %w", err)
		}
		l.Auto = autoBit
		links = append(links, l)
	}
	return links, nil
}
