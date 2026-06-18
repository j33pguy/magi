package db

import (
	"fmt"
	"strings"
	"time"
)

func (c *Client) PersistPreparedMemory(input PersistPreparedMemoryInput) (*PersistPreparedMemoryResult, error) {
	if input.Memory == nil {
		return nil, fmt.Errorf("memory is required")
	}

	ctx, err := buildSQLiteContextInput(input)
	if err != nil {
		return nil, err
	}

	const maxAttempts = 3
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		result, err := c.persistPreparedMemoryTx(ctx)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !isSQLiteBusyErr(err) {
			return nil, err
		}
		time.Sleep(time.Duration(attempt+1) * 25 * time.Millisecond)
	}
	return nil, lastErr
}

type sqliteContextInput struct {
	memory     *Memory
	tags       []string
	ctx        *MemoryContextRecord
	repository *RepositoryRecord
	ctxDurable any
	memoryNow  string
}

func buildSQLiteContextInput(input PersistPreparedMemoryInput) (*sqliteContextInput, error) {
	ctx := &sqliteContextInput{
		memory:    input.Memory,
		tags:      dedupeTags(input.Tags),
		memoryNow: time.Now().UTC().Format(time.DateTime),
	}
	if input.Context != nil {
		copyCtx := *input.Context
		ctx.ctx = &copyCtx
		ctx.ctxDurable = nullString(copyCtx.DurableAt)
		if copyCtx.Repository.CanonicalName != "" {
			repo := copyCtx.Repository
			ctx.repository = &repo
		}
	}
	return ctx, nil
}

func (c *Client) persistPreparedMemoryTx(input *sqliteContextInput) (*PersistPreparedMemoryResult, error) {
	tx, err := c.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin sqlite tx: %w", err)
	}
	defer tx.Rollback()

	memory := *input.memory
	visibility := memory.Visibility
	if visibility == "" {
		visibility = "internal"
	}

	var id string
	err = tx.QueryRow(`
		INSERT INTO memories (content, summary, embedding, project, type, visibility, source, source_file, parent_id, chunk_index, speaker, area, sub_area, created_at, updated_at, token_count)
		VALUES (?, ?, vector32(?), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`,
		memory.Content,
		nullString(memory.Summary),
		float32sToBytes(memory.Embedding),
		memory.Project,
		memory.Type,
		visibility,
		nullString(memory.Source),
		nullString(memory.SourceFile),
		nullString(memory.ParentID),
		memory.ChunkIndex,
		memory.Speaker,
		memory.Area,
		memory.SubArea,
		input.memoryNow,
		input.memoryNow,
		memory.TokenCount,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("inserting memory in tx: %w", err)
	}

	if input.ctx != nil {
		var repositoryID any
		if input.repository != nil {
			var repoID string
			repo := input.repository
			err = tx.QueryRow(`
				INSERT INTO repositories (host, owner, name, canonical_name, display_name, default_branch, is_fork, upstream_canonical_name)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(canonical_name) DO UPDATE SET
					host = COALESCE(NULLIF(excluded.host, ''), repositories.host),
					owner = COALESCE(NULLIF(excluded.owner, ''), repositories.owner),
					name = COALESCE(NULLIF(excluded.name, ''), repositories.name),
					display_name = COALESCE(NULLIF(excluded.display_name, ''), repositories.display_name),
					default_branch = COALESCE(NULLIF(excluded.default_branch, ''), repositories.default_branch),
					is_fork = excluded.is_fork,
					upstream_canonical_name = COALESCE(NULLIF(excluded.upstream_canonical_name, ''), repositories.upstream_canonical_name),
					updated_at = datetime('now')
				RETURNING id
			`, repo.Host, repo.Owner, repo.Name, repo.CanonicalName, repo.DisplayName, repo.DefaultBranch, boolToInt(repo.IsFork), repo.UpstreamCanonicalName).Scan(&repoID)
			if err != nil {
				return nil, fmt.Errorf("upserting repository context in tx: %w", err)
			}
			repositoryID = repoID
		}

		record := input.ctx
		_, err = tx.Exec(`
			INSERT INTO memory_contexts (
				memory_id, repository_id,
				scope_owner, scope_team, scope_workspace, scope_machine, scope_agent, scope_environment,
				provenance_transport, provenance_imported_from, provenance_human_authored, durable_at,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
			ON CONFLICT(memory_id) DO UPDATE SET
				repository_id = COALESCE(excluded.repository_id, memory_contexts.repository_id),
				scope_owner = COALESCE(NULLIF(excluded.scope_owner, ''), memory_contexts.scope_owner),
				scope_team = COALESCE(NULLIF(excluded.scope_team, ''), memory_contexts.scope_team),
				scope_workspace = COALESCE(NULLIF(excluded.scope_workspace, ''), memory_contexts.scope_workspace),
				scope_machine = COALESCE(NULLIF(excluded.scope_machine, ''), memory_contexts.scope_machine),
				scope_agent = COALESCE(NULLIF(excluded.scope_agent, ''), memory_contexts.scope_agent),
				scope_environment = COALESCE(NULLIF(excluded.scope_environment, ''), memory_contexts.scope_environment),
				provenance_transport = COALESCE(NULLIF(excluded.provenance_transport, ''), memory_contexts.provenance_transport),
				provenance_imported_from = COALESCE(NULLIF(excluded.provenance_imported_from, ''), memory_contexts.provenance_imported_from),
				provenance_human_authored = excluded.provenance_human_authored,
				durable_at = COALESCE(NULLIF(excluded.durable_at, ''), memory_contexts.durable_at),
				updated_at = datetime('now')
		`,
			id, repositoryID,
			record.ScopeOwner, record.ScopeTeam, record.ScopeWorkspace, record.ScopeMachine, record.ScopeAgent, record.ScopeEnvironment,
			record.ProvenanceTransport, record.ProvenanceImportedFrom, boolToInt(record.ProvenanceHumanAuthored), input.ctxDurable,
		)
		if err != nil {
			return nil, fmt.Errorf("upserting memory context in tx: %w", err)
		}
	}

	var tagWarning string
	if len(input.tags) > 0 {
		if _, err := tx.Exec(`DELETE FROM memory_tags WHERE memory_id = ?`, id); err != nil {
			tagWarning = fmt.Sprintf("clearing tags in tx: %v", err)
		} else {
			placeholders := make([]string, len(input.tags))
			args := make([]any, 0, len(input.tags)*2)
			for i, tag := range input.tags {
				placeholders[i] = "(?, ?)"
				args = append(args, id, tag)
			}
			query := fmt.Sprintf("INSERT INTO memory_tags (memory_id, tag) VALUES %s", strings.Join(placeholders, ", "))
			if _, err := tx.Exec(query, args...); err != nil {
				tagWarning = fmt.Sprintf("inserting tags in tx: %v", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit sqlite tx: %w", err)
	}

	memory.ID = id
	memory.CreatedAt = input.memoryNow
	memory.UpdatedAt = input.memoryNow
	return &PersistPreparedMemoryResult{Saved: &memory, TagWarning: tagWarning}, nil
}

func dedupeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func isSQLiteBusyErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "sqlite_busy")
}
