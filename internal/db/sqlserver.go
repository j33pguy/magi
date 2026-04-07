package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	_ "github.com/microsoft/go-mssqldb" // registers "sqlserver" driver
)

// SQLServerClient implements Store using Microsoft SQL Server.
// Vector search is done Go-side (cosine similarity reranking) since SQL Server
// has no native vector type. Full-text search uses SQL Server FTS catalogs.
type SQLServerClient struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewSQLServerClient opens a SQL Server connection using the given DSN.
// DSN format: sqlserver://user:pass@host:1433?database=magi
func NewSQLServerClient(dsn string, logger *slog.Logger) (*SQLServerClient, error) {
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening SQL Server database: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging SQL Server database: %w", err)
	}

	return &SQLServerClient{db: db, logger: logger}, nil
}

// Migrate runs all SQL Server schema migrations.
func (c *SQLServerClient) Migrate() error {
	return runSQLServerMigrations(c.db)
}

// Close shuts down the SQL Server connection.
func (c *SQLServerClient) Close() error {
	return c.db.Close()
}

// SaveMemory inserts a new memory and returns it with the generated ID.
func (c *SQLServerClient) SaveMemory(m *Memory) (*Memory, error) {
	now := time.Now().UTC().Format(time.DateTime)

	visibility := m.Visibility
	if visibility == "" {
		visibility = "internal"
	}

	var id string
	err := c.db.QueryRow(`
		INSERT INTO memories (content, summary, embedding, project, type, visibility,
			source, source_file, parent_id, chunk_index, speaker, area, sub_area,
			created_at, updated_at, token_count)
		OUTPUT INSERTED.id
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10, @p11, @p12, @p13, @p14, @p15, @p16)
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
	m.Visibility = visibility
	return m, nil
}

// GetMemory retrieves a single memory by ID.
func (c *SQLServerClient) GetMemory(id string) (*Memory, error) {
	m := &Memory{}
	var summary, source, sourceFile, parentID, archivedAt sql.NullString

	err := c.db.QueryRow(`
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
	m.ArchivedAt = archivedAt.String

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
		_, err := c.db.Exec(`
			UPDATE memories
			SET content = @p1, summary = @p2, embedding = @p3, type = @p4, updated_at = @p5, token_count = @p6
			WHERE id = @p7
		`,
			m.Content, nullString(m.Summary), float32sToBytes(m.Embedding),
			m.Type, now, m.TokenCount, m.ID,
		)
		return err
	}

	_, err := c.db.Exec(`
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
	_, err := c.db.Exec("UPDATE memories SET archived_at = @p1 WHERE id = @p2", now, id)
	return err
}

// DeleteMemory permanently removes a memory and its tags.
func (c *SQLServerClient) DeleteMemory(id string) error {
	_, err := c.db.Exec("DELETE FROM memories WHERE id = @p1", id)
	return err
}

// ListMemories returns memories matching the given filter criteria.
func (c *SQLServerClient) ListMemories(filter *MemoryFilter) ([]*Memory, error) {
	var conditions []string
	var args []any

	conditions = append(conditions, "m.archived_at IS NULL")

	sqlserverAppendProjectCondition(filter, &conditions, &args)
	sqlserverAppendTaxonomyConditions(filter, &conditions, &args)
	sqlserverAppendTimeConditions(filter, &conditions, &args)
	if filter.Type != "" {
		args = append(args, filter.Type)
		conditions = append(conditions, fmt.Sprintf("m.type = @p%d", len(args)))
	}
	if len(filter.Tags) > 0 {
		placeholders := make([]string, len(filter.Tags))
		for i, tag := range filter.Tags {
			args = append(args, tag)
			placeholders[i] = fmt.Sprintf("@p%d", len(args))
		}
		conditions = append(conditions, fmt.Sprintf(
			"m.id IN (SELECT memory_id FROM memory_tags WHERE tag IN (%s))",
			strings.Join(placeholders, ","),
		))
	}
	sqlserverAppendVisibilityCondition(filter, &conditions, &args)

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}

	args = append(args, limit)
	limitParam := fmt.Sprintf("@p%d", len(args))
	args = append(args, filter.Offset)
	offsetParam := fmt.Sprintf("@p%d", len(args))

	query := fmt.Sprintf(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count
		FROM memories m
		WHERE %s
		ORDER BY m.created_at DESC
		OFFSET %s ROWS FETCH NEXT %s ROWS ONLY
	`, strings.Join(conditions, " AND "), offsetParam, limitParam)

	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing memories: %w", err)
	}
	defer rows.Close()

	return c.scanMemories(rows)
}

// CountMemories returns the total number of memories matching the filter.
func (c *SQLServerClient) CountMemories(filter *MemoryFilter) (int, error) {
	if filter == nil {
		filter = &MemoryFilter{}
	}
	var conditions []string
	var args []any

	conditions = append(conditions, "m.archived_at IS NULL")

	sqlserverAppendProjectCondition(filter, &conditions, &args)
	sqlserverAppendTaxonomyConditions(filter, &conditions, &args)
	sqlserverAppendTimeConditions(filter, &conditions, &args)
	if filter.Type != "" {
		args = append(args, filter.Type)
		conditions = append(conditions, fmt.Sprintf("m.type = @p%d", len(args)))
	}
	if len(filter.Tags) > 0 {
		placeholders := make([]string, len(filter.Tags))
		for i, tag := range filter.Tags {
			args = append(args, tag)
			placeholders[i] = fmt.Sprintf("@p%d", len(args))
		}
		conditions = append(conditions, fmt.Sprintf(
			"m.id IN (SELECT memory_id FROM memory_tags WHERE tag IN (%s))",
			strings.Join(placeholders, ","),
		))
	}
	sqlserverAppendVisibilityCondition(filter, &conditions, &args)

	query := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM memories m
		WHERE %s
	`, strings.Join(conditions, " AND "))

	var count int
	if err := c.db.QueryRow(query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting memories: %w", err)
	}
	return count, nil
}

// SearchMemories performs vector similarity search using Go-side cosine reranking.
// Loads candidate memories with embeddings, computes cosine similarity in Go,
// and returns top-K results.
func (c *SQLServerClient) SearchMemories(embedding []float32, filter *MemoryFilter, topK int) ([]*VectorResult, error) {
	var conditions []string
	var args []any

	conditions = append(conditions, "m.archived_at IS NULL")
	conditions = append(conditions, "m.embedding IS NOT NULL")

	if filter != nil {
		sqlserverAppendProjectCondition(filter, &conditions, &args)
		sqlserverAppendTaxonomyConditions(filter, &conditions, &args)
		sqlserverAppendTimeConditions(filter, &conditions, &args)
		if filter.Type != "" {
			args = append(args, filter.Type)
			conditions = append(conditions, fmt.Sprintf("m.type = @p%d", len(args)))
		}
		if len(filter.Tags) > 0 {
			placeholders := make([]string, len(filter.Tags))
			for i, tag := range filter.Tags {
				args = append(args, tag)
				placeholders[i] = fmt.Sprintf("@p%d", len(args))
			}
			conditions = append(conditions, fmt.Sprintf(
				"m.id IN (SELECT memory_id FROM memory_tags WHERE tag IN (%s))",
				strings.Join(placeholders, ","),
			))
		}
		if filter != nil {
			sqlserverAppendVisibilityCondition(filter, &conditions, &args)
		}
	}

	if topK <= 0 {
		topK = 5
	}

	// Fetch candidates — limit to a reasonable pool for Go-side reranking.
	fetchLimit := topK * 20
	if fetchLimit < 200 {
		fetchLimit = 200
	}
	args = append(args, fetchLimit)
	limitParam := fmt.Sprintf("@p%d", len(args))

	query := fmt.Sprintf(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count,
		       m.embedding
		FROM memories m
		WHERE %s
		ORDER BY m.created_at DESC
		OFFSET 0 ROWS FETCH NEXT %s ROWS ONLY
	`, strings.Join(conditions, " AND "), limitParam)

	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("searching memories: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		memory    *Memory
		embedding []float32
	}
	var candidates []candidate

	for rows.Next() {
		m := &Memory{}
		var summary, source, sourceFile, parentID, archivedAt sql.NullString
		var embBytes []byte

		if err := rows.Scan(
			&m.ID, &m.Content, &summary, &m.Project, &m.Type, &m.Visibility,
			&source, &sourceFile, &parentID, &m.ChunkIndex,
			&m.Speaker, &m.Area, &m.SubArea,
			&m.CreatedAt, &m.UpdatedAt, &archivedAt, &m.TokenCount,
			&embBytes,
		); err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}

		m.Summary = summary.String
		m.Source = source.String
		m.SourceFile = sourceFile.String
		m.ParentID = parentID.String
		m.ArchivedAt = archivedAt.String

		emb := bytesToFloat32s(embBytes)
		if len(emb) > 0 {
			candidates = append(candidates, candidate{memory: m, embedding: emb})
		}
	}

	// Compute cosine distance for each candidate and pick top-K.
	type scored struct {
		memory   *Memory
		distance float64
	}
	var results []scored
	for _, cand := range candidates {
		dist := cosineDistance(embedding, cand.embedding)
		results = append(results, scored{memory: cand.memory, distance: dist})
	}

	// Sort by distance ascending (lower = more similar).
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].distance < results[j-1].distance; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
	if len(results) > topK {
		results = results[:topK]
	}

	var out []*VectorResult
	for _, r := range results {
		tags, err := c.GetTags(r.memory.ID)
		if err != nil {
			return nil, err
		}
		r.memory.Tags = tags
		out = append(out, &VectorResult{Memory: r.memory, Distance: r.distance})
	}

	return out, nil
}

// SearchMemoriesBM25 performs full-text keyword search using SQL Server FTS.
func (c *SQLServerClient) SearchMemoriesBM25(query string, filter *MemoryFilter, topK int) ([]*VectorResult, error) {
	var conditions []string
	var args []any

	conditions = append(conditions, "m.archived_at IS NULL")

	// Use FREETEXTTABLE for ranked full-text search.
	args = append(args, query)
	ftsParam := fmt.Sprintf("@p%d", len(args))
	conditions = append(conditions, fmt.Sprintf(
		"m.id IN (SELECT [KEY] FROM FREETEXTTABLE(memories, content, %s))", ftsParam))

	if filter != nil {
		sqlserverAppendProjectCondition(filter, &conditions, &args)
		sqlserverAppendTaxonomyConditions(filter, &conditions, &args)
		sqlserverAppendTimeConditions(filter, &conditions, &args)
		if filter.Type != "" {
			args = append(args, filter.Type)
			conditions = append(conditions, fmt.Sprintf("m.type = @p%d", len(args)))
		}
		sqlserverAppendVisibilityCondition(filter, &conditions, &args)
	}

	if topK <= 0 {
		topK = 10
	}
	args = append(args, topK)
	limitParam := fmt.Sprintf("@p%d", len(args))

	q := fmt.Sprintf(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count,
		       ft.[RANK] AS score
		FROM memories m
		INNER JOIN FREETEXTTABLE(memories, content, %s) ft ON ft.[KEY] = m.id
		WHERE %s
		ORDER BY score DESC
		OFFSET 0 ROWS FETCH NEXT %s ROWS ONLY
	`, ftsParam, strings.Join(conditions, " AND "), limitParam)

	rows, err := c.db.Query(q, args...)
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

		// Use negative rank as distance (lower = better, matching libSQL convention).
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
// Reciprocal Rank Fusion (RRF) with k=60.
func (c *SQLServerClient) HybridSearch(embedding []float32, query string, filter *MemoryFilter, topK int) ([]*HybridResult, error) {
	if topK <= 0 {
		topK = 10
	}
	fetchK := topK * 3

	vecResults, err := c.SearchMemories(embedding, filter, fetchK)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	bm25Results, err := c.SearchMemoriesBM25(query, filter, fetchK)
	if err != nil {
		return nil, fmt.Errorf("BM25 search: %w", err)
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

// GetContextMemories returns recent (7 days) non-private memories.
func (c *SQLServerClient) GetContextMemories(project string, limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 10
	}

	var conditions []string
	var args []any

	conditions = append(conditions, "m.archived_at IS NULL")
	conditions = append(conditions, "m.created_at > DATEADD(day, -7, GETUTCDATE())")
	conditions = append(conditions, "m.visibility != 'private'")

	if project != "" {
		args = append(args, project)
		conditions = append(conditions, fmt.Sprintf("m.project = @p%d", len(args)))
	}

	args = append(args, limit)
	limitParam := fmt.Sprintf("@p%d", len(args))

	query := fmt.Sprintf(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count
		FROM memories m
		WHERE %s
		ORDER BY m.created_at DESC
		OFFSET 0 ROWS FETCH NEXT %s ROWS ONLY
	`, strings.Join(conditions, " AND "), limitParam)

	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("getting context memories: %w", err)
	}
	defer rows.Close()

	return c.scanMemories(rows)
}

// FindSimilar returns the single closest non-archived memory by cosine distance.
func (c *SQLServerClient) FindSimilar(embedding []float32, maxDistance float64) (*VectorResult, error) {
	// Fetch candidates with embeddings for Go-side similarity.
	rows, err := c.db.Query(`
		SELECT TOP(200) m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count,
		       m.embedding
		FROM memories m
		WHERE m.archived_at IS NULL AND m.embedding IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("finding similar memory: %w", err)
	}
	defer rows.Close()

	var bestMemory *Memory
	bestDist := math.MaxFloat64

	for rows.Next() {
		m := &Memory{}
		var summary, source, sourceFile, parentID, archivedAt sql.NullString
		var embBytes []byte

		if err := rows.Scan(
			&m.ID, &m.Content, &summary, &m.Project, &m.Type, &m.Visibility,
			&source, &sourceFile, &parentID, &m.ChunkIndex,
			&m.Speaker, &m.Area, &m.SubArea,
			&m.CreatedAt, &m.UpdatedAt, &archivedAt, &m.TokenCount,
			&embBytes,
		); err != nil {
			return nil, fmt.Errorf("scanning similar memory: %w", err)
		}

		m.Summary = summary.String
		m.Source = source.String
		m.SourceFile = sourceFile.String
		m.ParentID = parentID.String
		m.ArchivedAt = archivedAt.String

		emb := bytesToFloat32s(embBytes)
		if len(emb) == 0 {
			continue
		}

		dist := cosineDistance(embedding, emb)
		if dist < bestDist {
			bestDist = dist
			bestMemory = m
		}
	}

	if bestMemory == nil || bestDist > maxDistance {
		return nil, nil
	}

	tags, err := c.GetTags(bestMemory.ID)
	if err != nil {
		return nil, err
	}
	bestMemory.Tags = tags

	return &VectorResult{Memory: bestMemory, Distance: bestDist}, nil
}

// --- Tags ---

// ExistsWithContentHash returns the memory ID that has the given hash tag, or "" if none.
func (c *SQLServerClient) ExistsWithContentHash(hash string) (string, error) {
	tag := "hash:" + hash
	var id string
	err := c.db.QueryRow("SELECT TOP(1) memory_id FROM memory_tags WHERE tag = @p1", tag).Scan(&id)
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
	rows, err := c.db.Query("SELECT tag FROM memory_tags WHERE memory_id = @p1 ORDER BY tag", memoryID)
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
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec("DELETE FROM memory_tags WHERE memory_id = @p1", memoryID); err != nil {
		return fmt.Errorf("clearing tags: %w", err)
	}

	tags = normalizeTags(tags)
	if len(tags) == 0 {
		return tx.Commit()
	}

	// Batch insert all tags.
	placeholders := make([]string, len(tags))
	args := make([]any, 0, len(tags)*2)
	for i, tag := range tags {
		args = append(args, memoryID, tag)
		placeholders[i] = fmt.Sprintf("(@p%d, @p%d)", i*2+1, i*2+2)
	}

	query := fmt.Sprintf("INSERT INTO memory_tags (memory_id, tag) VALUES %s", strings.Join(placeholders, ", "))
	if _, err := tx.Exec(query, args...); err != nil {
		return fmt.Errorf("inserting tags: %w", err)
	}

	return tx.Commit()
}

// --- Links ---

// CreateLink creates a directed link between two memories.
func (c *SQLServerClient) CreateLink(ctx context.Context, fromID, toID, relation string, weight float64, auto bool) (*MemoryLink, error) {
	now := time.Now().UTC().Format(time.DateTime)
	autoVal := false
	if auto {
		autoVal = true
	}

	var id string
	err := c.db.QueryRowContext(ctx, `
		INSERT INTO memory_links (from_id, to_id, relation, weight, auto, created_at)
		OUTPUT INSERTED.id
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6)
	`, fromID, toID, relation, weight, autoVal, now).Scan(&id)
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

	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("getting links: %w", err)
	}
	defer rows.Close()

	return c.scanLinks(rows)
}

// DeleteLink removes a link by ID.
func (c *SQLServerClient) DeleteLink(ctx context.Context, linkID string) error {
	result, err := c.db.ExecContext(ctx, "DELETE FROM memory_links WHERE id = @p1", linkID)
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

// TraverseGraph does a BFS from startID up to maxDepth hops.
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
func (c *SQLServerClient) GetGraphData(ctx context.Context, topN int) ([]*Memory, []*MemoryLink, error) {
	rows, err := c.db.QueryContext(ctx, `
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
		OFFSET 0 ROWS FETCH NEXT @p1 ROWS ONLY
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

	linkRows, err := c.db.QueryContext(ctx, `
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

// --- Helpers ---

func (c *SQLServerClient) scanMemories(rows *sql.Rows) ([]*Memory, error) {
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

func (c *SQLServerClient) scanLinks(rows *sql.Rows) ([]*MemoryLink, error) {
	var links []*MemoryLink
	for rows.Next() {
		l := &MemoryLink{}
		var autoVal bool
		if err := rows.Scan(&l.ID, &l.FromID, &l.ToID, &l.Relation, &l.Weight, &autoVal, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning link: %w", err)
		}
		l.Auto = autoVal
		links = append(links, l)
	}
	return links, nil
}

// --- SQL Server parameter helpers ---
// These mirror the shared helpers in memory.go but use @pN numbered parameters.

func sqlserverAppendProjectCondition(filter *MemoryFilter, conditions *[]string, args *[]any) {
	if filter.Project != "" {
		*args = append(*args, filter.Project)
		*conditions = append(*conditions, fmt.Sprintf("m.project = @p%d", len(*args)))
	} else if len(filter.Projects) > 0 {
		placeholders := make([]string, len(filter.Projects))
		for i, p := range filter.Projects {
			*args = append(*args, p)
			placeholders[i] = fmt.Sprintf("@p%d", len(*args))
		}
		*conditions = append(*conditions, fmt.Sprintf("m.project IN (%s)", strings.Join(placeholders, ",")))
	}
}

func sqlserverAppendTaxonomyConditions(filter *MemoryFilter, conditions *[]string, args *[]any) {
	if filter.Speaker != "" {
		*args = append(*args, filter.Speaker)
		*conditions = append(*conditions, fmt.Sprintf("m.speaker = @p%d", len(*args)))
	}
	if filter.Area != "" {
		*args = append(*args, filter.Area)
		*conditions = append(*conditions, fmt.Sprintf("m.area = @p%d", len(*args)))
	}
	if filter.SubArea != "" {
		*args = append(*args, filter.SubArea)
		*conditions = append(*conditions, fmt.Sprintf("m.sub_area = @p%d", len(*args)))
	}
}

func sqlserverAppendTimeConditions(filter *MemoryFilter, conditions *[]string, args *[]any) {
	if filter.AfterTime != nil {
		*args = append(*args, filter.AfterTime.UTC().Format(time.RFC3339))
		*conditions = append(*conditions, fmt.Sprintf("m.created_at > @p%d", len(*args)))
	}
	if filter.BeforeTime != nil {
		*args = append(*args, filter.BeforeTime.UTC().Format(time.RFC3339))
		*conditions = append(*conditions, fmt.Sprintf("m.created_at < @p%d", len(*args)))
	}
}

func sqlserverAppendVisibilityCondition(filter *MemoryFilter, conditions *[]string, args *[]any) {
	if filter.Visibility == "all" {
	} else if filter.Visibility != "" {
		*args = append(*args, filter.Visibility)
		*conditions = append(*conditions, fmt.Sprintf("m.visibility = @p%d", len(*args)))
	} else {
		*conditions = append(*conditions, "m.visibility != 'private'")
	}
	sqlserverAppendAccessCondition(filter, conditions, args)
}

func sqlserverAppendAccessCondition(filter *MemoryFilter, conditions *[]string, args *[]any) {
	if filter == nil || !filter.EnforceAccess {
		return
	}

	parts := []string{
		"(m.visibility != 'private' AND NOT EXISTS (SELECT 1 FROM memory_tags acl WHERE acl.memory_id = m.id AND (acl.tag LIKE 'owner:%' OR acl.tag LIKE 'viewer:%' OR acl.tag LIKE 'viewer_group:%')))",
	}

	if filter.RequestUser != "" {
		*args = append(*args, "owner:"+filter.RequestUser)
		parts = append(parts, fmt.Sprintf("EXISTS (SELECT 1 FROM memory_tags acl WHERE acl.memory_id = m.id AND acl.tag = @p%d)", len(*args)))
		*args = append(*args, "viewer:"+filter.RequestUser)
		parts = append(parts, fmt.Sprintf("EXISTS (SELECT 1 FROM memory_tags acl WHERE acl.memory_id = m.id AND acl.tag = @p%d)", len(*args)))
	}

	for _, group := range filter.RequestGroups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		*args = append(*args, "viewer_group:"+group)
		parts = append(parts, fmt.Sprintf("EXISTS (SELECT 1 FROM memory_tags acl WHERE acl.memory_id = m.id AND acl.tag = @p%d)", len(*args)))
	}

	*conditions = append(*conditions, "("+strings.Join(parts, " OR ")+")")
}
