package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// PostgresClient implements Store using a PostgreSQL backend with pgvector.
type PostgresClient struct {
	DB     *sql.DB
	logger *slog.Logger
}

// Compile-time interface check.
var _ Store = (*PostgresClient)(nil)

// NewPostgresClient opens a PostgreSQL connection pool and verifies connectivity.
func NewPostgresClient(connURL string, logger *slog.Logger) (*PostgresClient, error) {
	db, err := sql.Open("pgx", connURL)
	if err != nil {
		return nil, fmt.Errorf("opening postgres: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(10 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	return &PostgresClient{DB: db, logger: logger}, nil
}

// Close shuts down the database connection pool.
func (c *PostgresClient) Close() error {
	return c.DB.Close()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// float32sToPgVector converts a float32 slice to pgvector text format: "[0.1,0.2,...]".
func float32sToPgVector(v []float32) string {
	if v == nil {
		return ""
	}
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%g", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// pgBuilder helps construct WHERE clauses with PostgreSQL numbered placeholders.
type pgBuilder struct {
	conditions []string
	args       []any
	n          int // next placeholder number
}

func (b *pgBuilder) add(cond string, arg any) {
	b.n++
	b.conditions = append(b.conditions, strings.ReplaceAll(cond, "$?", fmt.Sprintf("$%d", b.n)))
	b.args = append(b.args, arg)
}

func (b *pgBuilder) addRaw(cond string) {
	b.conditions = append(b.conditions, cond)
}

func (b *pgBuilder) where() string {
	if len(b.conditions) == 0 {
		return "TRUE"
	}
	return strings.Join(b.conditions, " AND ")
}

// pgNullString mirrors the sqlite nullString helper.
func pgNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// appendPgProjectCondition adds project filtering using numbered placeholders.
func appendPgProjectCondition(filter *MemoryFilter, b *pgBuilder) {
	if filter.Project != "" {
		b.add("m.project = $?", filter.Project)
	} else if len(filter.Projects) > 0 {
		placeholders := make([]string, len(filter.Projects))
		for i, p := range filter.Projects {
			b.n++
			placeholders[i] = fmt.Sprintf("$%d", b.n)
			b.args = append(b.args, p)
		}
		b.conditions = append(b.conditions, fmt.Sprintf("m.project IN (%s)", strings.Join(placeholders, ",")))
	}
}

// appendPgTaxonomyConditions adds speaker/area/sub_area filtering.
func appendPgTaxonomyConditions(filter *MemoryFilter, b *pgBuilder) {
	if filter.Speaker != "" {
		b.add("m.speaker = $?", filter.Speaker)
	}
	if filter.Area != "" {
		b.add("m.area = $?", filter.Area)
	}
	if filter.SubArea != "" {
		b.add("m.sub_area = $?", filter.SubArea)
	}
}

// appendPgTimeConditions adds after/before time filtering.
func appendPgTimeConditions(filter *MemoryFilter, b *pgBuilder) {
	if filter.AfterTime != nil {
		b.add("m.created_at > $?", filter.AfterTime.UTC().Format(time.RFC3339))
	}
	if filter.BeforeTime != nil {
		b.add("m.created_at < $?", filter.BeforeTime.UTC().Format(time.RFC3339))
	}
}

// appendPgVisibilityCondition adds visibility filtering.
func appendPgVisibilityCondition(filter *MemoryFilter, b *pgBuilder) {
	if filter.Visibility == "all" {
		return
	}
	if filter.Visibility != "" {
		b.add("m.visibility = $?", filter.Visibility)
	} else {
		b.addRaw("m.visibility != 'private'")
	}
}

// appendPgTagCondition adds tag IN filtering.
func appendPgTagCondition(filter *MemoryFilter, b *pgBuilder) {
	if len(filter.Tags) == 0 {
		return
	}
	placeholders := make([]string, len(filter.Tags))
	for i, tag := range filter.Tags {
		b.n++
		placeholders[i] = fmt.Sprintf("$%d", b.n)
		b.args = append(b.args, tag)
	}
	b.conditions = append(b.conditions, fmt.Sprintf(
		"m.id IN (SELECT memory_id FROM memory_tags WHERE tag IN (%s))",
		strings.Join(placeholders, ","),
	))
}

// applyPgFilter applies all standard filter conditions.
func applyPgFilter(filter *MemoryFilter, b *pgBuilder) {
	if filter == nil {
		return
	}
	appendPgProjectCondition(filter, b)
	appendPgTaxonomyConditions(filter, b)
	appendPgTimeConditions(filter, b)
	if filter.Type != "" {
		b.add("m.type = $?", filter.Type)
	}
	appendPgTagCondition(filter, b)
	appendPgVisibilityCondition(filter, b)
}

// ---------------------------------------------------------------------------
// Migrations
// ---------------------------------------------------------------------------

// Migrate runs all PostgreSQL migrations. Safe to call on every startup.
func (c *PostgresClient) Migrate() error {
	// Create meta table
	if _, err := c.DB.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
		)
	`); err != nil {
		return fmt.Errorf("creating meta table: %w", err)
	}

	migrations := []struct {
		version int
		sql     string
	}{
		{1, pgMigrationV1},
		{2, pgMigrationV2},
		{3, pgMigrationV3},
		{4, pgMigrationV4},
		{5, pgMigrationV5},
		{6, pgMigrationV6},
		{7, pgMigrationV7},
	}

	for _, m := range migrations {
		var count int
		if err := c.DB.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = $1", m.version).Scan(&count); err != nil {
			return fmt.Errorf("checking migration %d: %w", m.version, err)
		}
		if count > 0 {
			continue
		}

		// PostgreSQL handles multi-statement strings natively.
		if _, err := c.DB.Exec(m.sql); err != nil {
			return fmt.Errorf("running migration %d: %w", m.version, err)
		}

		if _, err := c.DB.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", m.version); err != nil {
			return fmt.Errorf("marking migration %d: %w", m.version, err)
		}

		c.logger.Info("Applied migration", "version", m.version)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Core CRUD
// ---------------------------------------------------------------------------

// SaveMemory inserts a new memory and returns it with the generated ID.
func (c *PostgresClient) SaveMemory(m *Memory) (*Memory, error) {
	now := time.Now().UTC().Format(time.DateTime)

	visibility := m.Visibility
	if visibility == "" {
		visibility = "internal"
	}

	var id string
	err := c.DB.QueryRow(`
		INSERT INTO memories (content, summary, embedding, project, type, visibility,
		                      source, source_file, parent_id, chunk_index,
		                      speaker, area, sub_area, created_at, updated_at, token_count)
		VALUES ($1, $2, $3::vector, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING id
	`,
		m.Content,
		pgNullString(m.Summary),
		float32sToPgVector(m.Embedding),
		m.Project,
		m.Type,
		visibility,
		pgNullString(m.Source),
		pgNullString(m.SourceFile),
		pgNullString(m.ParentID),
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
func (c *PostgresClient) GetMemory(id string) (*Memory, error) {
	m := &Memory{}
	var summary, source, sourceFile, parentID, archivedAt sql.NullString

	err := c.DB.QueryRow(`
		SELECT id, content, summary, project, type, visibility, source, source_file,
		       parent_id, chunk_index, speaker, area, sub_area,
		       created_at, updated_at, archived_at, token_count
		FROM memories WHERE id = $1
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
func (c *PostgresClient) UpdateMemory(m *Memory) error {
	now := time.Now().UTC().Format(time.DateTime)

	if m.Embedding != nil {
		_, err := c.DB.Exec(`
			UPDATE memories
			SET content = $1, summary = $2, embedding = $3::vector, type = $4, updated_at = $5, token_count = $6
			WHERE id = $7
		`,
			m.Content, pgNullString(m.Summary), float32sToPgVector(m.Embedding),
			m.Type, now, m.TokenCount, m.ID,
		)
		return err
	}

	_, err := c.DB.Exec(`
		UPDATE memories
		SET content = $1, summary = $2, type = $3, updated_at = $4
		WHERE id = $5
	`,
		m.Content, pgNullString(m.Summary), m.Type, now, m.ID,
	)
	return err
}

// ArchiveMemory soft-deletes a memory by setting archived_at.
func (c *PostgresClient) ArchiveMemory(id string) error {
	now := time.Now().UTC().Format(time.DateTime)
	_, err := c.DB.Exec("UPDATE memories SET archived_at = $1 WHERE id = $2", now, id)
	return err
}

// DeleteMemory permanently removes a memory and its tags.
func (c *PostgresClient) DeleteMemory(id string) error {
	_, err := c.DB.Exec("DELETE FROM memories WHERE id = $1", id)
	return err
}

// ListMemories returns memories matching the given filter criteria.
func (c *PostgresClient) ListMemories(filter *MemoryFilter) ([]*Memory, error) {
	b := &pgBuilder{}
	b.addRaw("m.archived_at IS NULL")
	applyPgFilter(filter, b)

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	b.add("TRUE ORDER BY m.created_at DESC LIMIT $?", limit)
	// Offset as a separate placeholder — splice it into the query manually.
	b.n++
	offsetPH := fmt.Sprintf("$%d", b.n)
	b.args = append(b.args, filter.Offset)

	query := fmt.Sprintf(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count
		FROM memories m
		WHERE %s
		OFFSET %s
	`, b.where(), offsetPH)

	rows, err := c.DB.Query(query, b.args...)
	if err != nil {
		return nil, fmt.Errorf("listing memories: %w", err)
	}
	defer rows.Close()

	return pgScanMemories(rows)
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

// SearchMemories performs a vector similarity search using pgvector cosine distance.
func (c *PostgresClient) SearchMemories(embedding []float32, filter *MemoryFilter, topK int) ([]*VectorResult, error) {
	b := &pgBuilder{}

	// Embedding is the first arg for the ORDER BY distance calculation.
	b.n++
	embIdx := b.n
	b.args = append(b.args, float32sToPgVector(embedding))

	b.addRaw("m.archived_at IS NULL")
	applyPgFilter(filter, b)

	if topK <= 0 {
		topK = 5
	}
	b.n++
	limitPH := fmt.Sprintf("$%d", b.n)
	b.args = append(b.args, topK)

	query := fmt.Sprintf(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count,
		       m.embedding <=> $%d::vector AS distance
		FROM memories m
		WHERE %s
		ORDER BY distance ASC
		LIMIT %s
	`, embIdx, b.where(), limitPH)

	rows, err := c.DB.Query(query, b.args...)
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

	for _, r := range results {
		tags, err := c.GetTags(r.Memory.ID)
		if err != nil {
			return nil, err
		}
		r.Memory.Tags = tags
	}

	return results, nil
}

// SearchMemoriesBM25 performs full-text keyword search using PostgreSQL tsvector.
func (c *PostgresClient) SearchMemoriesBM25(query string, filter *MemoryFilter, topK int) ([]*VectorResult, error) {
	b := &pgBuilder{}

	b.addRaw("m.archived_at IS NULL")
	b.add("m.search_vector @@ plainto_tsquery('english', $?)", query)

	applyPgFilter(filter, b)

	if topK <= 0 {
		topK = 10
	}

	// We need the query text again for ts_rank; reuse the same placeholder index.
	// plainto_tsquery is used at placeholder $queryPH (which is b.args index 1, placeholder 2).
	queryPH := 2 // the second placeholder added above

	b.n++
	limitPH := fmt.Sprintf("$%d", b.n)
	b.args = append(b.args, topK)

	q := fmt.Sprintf(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count,
		       ts_rank(m.search_vector, plainto_tsquery('english', $%d)) AS score
		FROM memories m
		WHERE %s
		ORDER BY score DESC
		LIMIT %s
	`, queryPH, b.where(), limitPH)

	rows, err := c.DB.Query(q, b.args...)
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

// HybridSearch runs vector and BM25 searches concurrently then fuses results
// using Reciprocal Rank Fusion (RRF) with k=60.
func (c *PostgresClient) HybridSearch(embedding []float32, query string, filter *MemoryFilter, topK int) ([]*HybridResult, error) {
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

// GetContextMemories returns recent (7 day) memories, optionally filtered by project.
func (c *PostgresClient) GetContextMemories(project string, limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 10
	}

	b := &pgBuilder{}
	b.addRaw("m.archived_at IS NULL")
	b.addRaw("m.created_at > NOW() AT TIME ZONE 'UTC' - INTERVAL '7 days'")
	b.addRaw("m.visibility != 'private'")

	if project != "" {
		b.add("m.project = $?", project)
	}

	b.n++
	limitPH := fmt.Sprintf("$%d", b.n)
	b.args = append(b.args, limit)

	query := fmt.Sprintf(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count
		FROM memories m
		WHERE %s
		ORDER BY m.created_at DESC
		LIMIT %s
	`, b.where(), limitPH)

	rows, err := c.DB.Query(query, b.args...)
	if err != nil {
		return nil, fmt.Errorf("getting context memories: %w", err)
	}
	defer rows.Close()

	return pgScanMemories(rows)
}

// FindSimilar returns the single closest non-archived memory by cosine distance.
func (c *PostgresClient) FindSimilar(embedding []float32, maxDistance float64) (*VectorResult, error) {
	var m Memory
	var summary, source, sourceFile, parentID, archivedAt sql.NullString
	var distance float64

	err := c.DB.QueryRow(`
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source, m.source_file,
		       m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count,
		       m.embedding <=> $1::vector AS distance
		FROM memories m
		WHERE m.archived_at IS NULL
		ORDER BY distance ASC
		LIMIT 1
	`, float32sToPgVector(embedding)).Scan(
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

// ---------------------------------------------------------------------------
// Tags
// ---------------------------------------------------------------------------

// ExistsWithContentHash returns the memory ID that has the given hash tag, or "" if none.
func (c *PostgresClient) ExistsWithContentHash(hash string) (string, error) {
	tag := "hash:" + hash
	var id string
	err := c.DB.QueryRow("SELECT memory_id FROM memory_tags WHERE tag = $1 LIMIT 1", tag).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("checking content hash: %w", err)
	}
	return id, nil
}

// GetTags returns all tags for a memory.
func (c *PostgresClient) GetTags(memoryID string) ([]string, error) {
	rows, err := c.DB.Query("SELECT tag FROM memory_tags WHERE memory_id = $1 ORDER BY tag", memoryID)
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
func (c *PostgresClient) SetTags(memoryID string, tags []string) error {
	tx, err := c.DB.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec("DELETE FROM memory_tags WHERE memory_id = $1", memoryID); err != nil {
		return fmt.Errorf("clearing tags: %w", err)
	}

	if len(tags) == 0 {
		return tx.Commit()
	}

	placeholders := make([]string, len(tags))
	args := make([]any, 0, len(tags)*2)
	for i, tag := range tags {
		base := i*2 + 1
		placeholders[i] = fmt.Sprintf("($%d, $%d)", base, base+1)
		args = append(args, memoryID, tag)
	}

	query := fmt.Sprintf("INSERT INTO memory_tags (memory_id, tag) VALUES %s", strings.Join(placeholders, ", "))
	if _, err := tx.Exec(query, args...); err != nil {
		return fmt.Errorf("inserting tags: %w", err)
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Links
// ---------------------------------------------------------------------------

// CreateLink creates a directed link between two memories.
func (c *PostgresClient) CreateLink(ctx context.Context, fromID, toID, relation string, weight float64, auto bool) (*MemoryLink, error) {
	now := time.Now().UTC().Format(time.DateTime)

	var id string
	err := c.DB.QueryRowContext(ctx, `
		INSERT INTO memory_links (from_id, to_id, relation, weight, auto, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, fromID, toID, relation, weight, auto, now).Scan(&id)
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
func (c *PostgresClient) GetLinks(ctx context.Context, memoryID string, direction string) ([]*MemoryLink, error) {
	var query string
	var args []any

	switch direction {
	case "from":
		query = `SELECT id, from_id, to_id, relation, weight, auto, created_at FROM memory_links WHERE from_id = $1 ORDER BY created_at DESC`
		args = []any{memoryID}
	case "to":
		query = `SELECT id, from_id, to_id, relation, weight, auto, created_at FROM memory_links WHERE to_id = $1 ORDER BY created_at DESC`
		args = []any{memoryID}
	default: // "both"
		query = `SELECT id, from_id, to_id, relation, weight, auto, created_at FROM memory_links WHERE from_id = $1 OR to_id = $2 ORDER BY created_at DESC`
		args = []any{memoryID, memoryID}
	}

	rows, err := c.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("getting links: %w", err)
	}
	defer rows.Close()

	return pgScanLinks(rows)
}

// DeleteLink removes a link by ID.
func (c *PostgresClient) DeleteLink(ctx context.Context, linkID string) error {
	result, err := c.DB.ExecContext(ctx, "DELETE FROM memory_links WHERE id = $1", linkID)
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
func (c *PostgresClient) TraverseGraph(ctx context.Context, startID string, maxDepth int) ([]string, error) {
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
func (c *PostgresClient) GetGraphData(ctx context.Context, topN int) ([]*Memory, []*MemoryLink, error) {
	rows, err := c.DB.QueryContext(ctx, `
		SELECT m.id, m.content, m.summary, m.project, m.type, m.visibility, m.source,
		       m.source_file, m.parent_id, m.chunk_index, m.speaker, m.area, m.sub_area,
		       m.created_at, m.updated_at, m.archived_at, m.token_count,
		       COALESCE(lc.cnt, 0) AS link_count
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
		LIMIT $1
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

	allLinks, err := pgScanLinks(linkRows)
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
// Row scanners
// ---------------------------------------------------------------------------

func pgScanMemories(rows *sql.Rows) ([]*Memory, error) {
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

func pgScanLinks(rows *sql.Rows) ([]*MemoryLink, error) {
	var links []*MemoryLink
	for rows.Next() {
		l := &MemoryLink{}
		if err := rows.Scan(&l.ID, &l.FromID, &l.ToID, &l.Relation, &l.Weight, &l.Auto, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning link: %w", err)
		}
		links = append(links, l)
	}
	return links, nil
}
