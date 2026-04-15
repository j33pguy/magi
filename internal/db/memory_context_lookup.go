package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// MemoryContextLookup carries additive read-path signals for reranking search results.
type MemoryContextLookup struct {
	MemoryID            string
	RepositoryCanonical string
	ScopeWorkspace      string
	ScopeMachine        string
	ScopeAgent          string
}

// MemoryContextReader is an optional store capability for retrieval-time context enrichment.
type MemoryContextReader interface {
	LookupMemoryContexts(memoryIDs []string) (map[string]MemoryContextLookup, error)
}

func scanMemoryContexts(rows *sql.Rows) (map[string]MemoryContextLookup, error) {
	defer rows.Close()

	out := make(map[string]MemoryContextLookup)
	for rows.Next() {
		var ctx MemoryContextLookup
		var repo sql.NullString
		if err := rows.Scan(&ctx.MemoryID, &repo, &ctx.ScopeWorkspace, &ctx.ScopeMachine, &ctx.ScopeAgent); err != nil {
			return nil, err
		}
		ctx.RepositoryCanonical = repo.String
		out[ctx.MemoryID] = ctx
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func buildInClause(count int, placeholder func(int) string) string {
	parts := make([]string, count)
	for i := 0; i < count; i++ {
		parts[i] = placeholder(i)
	}
	return strings.Join(parts, ",")
}

func lookupMemoryContexts(db Queryer, placeholder func(int) string, memoryIDs []string) (map[string]MemoryContextLookup, error) {
	if len(memoryIDs) == 0 {
		return map[string]MemoryContextLookup{}, nil
	}
	args := make([]any, len(memoryIDs))
	for i, id := range memoryIDs {
		args[i] = id
	}
	rows, err := db.Query(fmt.Sprintf(`
		SELECT mc.memory_id, COALESCE(r.canonical_name, ''), mc.scope_workspace, mc.scope_machine, mc.scope_agent
		FROM memory_contexts mc
		LEFT JOIN repositories r ON r.id = mc.repository_id
		WHERE mc.memory_id IN (%s)
	`, buildInClause(len(memoryIDs), placeholder)), args...)
	if err != nil {
		return nil, err
	}
	return scanMemoryContexts(rows)
}

type Queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}
