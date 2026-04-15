package db

import (
	"database/sql"
	"fmt"
)

func (c *SQLServerClient) LookupMemoryContexts(memoryIDs []string) (map[string]MemoryContextLookup, error) {
	if len(memoryIDs) == 0 {
		return map[string]MemoryContextLookup{}, nil
	}
	args := make([]any, len(memoryIDs))
	for i, id := range memoryIDs {
		args[i] = id
	}
	placeholders := buildInClause(len(memoryIDs), func(i int) string { return fmt.Sprintf("@p%d", i+1) })
	rows, err := c.db.Query(fmt.Sprintf(`
		SELECT mc.memory_id, COALESCE(r.canonical_name, ''), mc.scope_workspace, mc.scope_machine, mc.scope_agent
		FROM memory_contexts mc
		LEFT JOIN repositories r ON r.id = mc.repository_id
		WHERE mc.memory_id IN (%s)
	`, placeholders), args...)
	if err != nil {
		return nil, fmt.Errorf("loading memory contexts: %w", err)
	}
	return scanMemoryContexts(rows)
}

func (c *SQLServerClient) SaveMemoryContext(record *MemoryContextRecord) error {
	if record == nil || record.MemoryID == "" || record.Empty() {
		return nil
	}

	var repositoryID any
	if repo := record.Repository; repo.CanonicalName != "" {
		var id string
		err := c.db.QueryRow(`
			MERGE repositories AS target
			USING (SELECT @p1 AS host, @p2 AS owner_name, @p3 AS repo_name, @p4 AS canonical_name, @p5 AS display_name, @p6 AS default_branch, @p7 AS is_fork, @p8 AS upstream_canonical_name) AS source
			ON target.canonical_name = source.canonical_name
			WHEN MATCHED THEN UPDATE SET
				host = CASE WHEN source.host = '' THEN target.host ELSE source.host END,
				owner = CASE WHEN source.owner_name = '' THEN target.owner ELSE source.owner_name END,
				name = CASE WHEN source.repo_name = '' THEN target.name ELSE source.repo_name END,
				display_name = CASE WHEN source.display_name = '' THEN target.display_name ELSE source.display_name END,
				default_branch = CASE WHEN source.default_branch = '' THEN target.default_branch ELSE source.default_branch END,
				is_fork = source.is_fork,
				upstream_canonical_name = CASE WHEN source.upstream_canonical_name = '' THEN target.upstream_canonical_name ELSE source.upstream_canonical_name END,
				updated_at = GETUTCDATE()
			WHEN NOT MATCHED THEN INSERT (host, owner, name, canonical_name, display_name, default_branch, is_fork, upstream_canonical_name)
			VALUES (source.host, source.owner_name, source.repo_name, source.canonical_name, source.display_name, source.default_branch, source.is_fork, source.upstream_canonical_name)
			OUTPUT inserted.id;
		`, repo.Host, repo.Owner, repo.Name, repo.CanonicalName, repo.DisplayName, repo.DefaultBranch, repo.IsFork, repo.UpstreamCanonicalName).Scan(&id)
		if err != nil {
			return fmt.Errorf("upserting repository context: %w", err)
		}
		repositoryID = id
	}

	var existing int
	err := c.db.QueryRow(`SELECT COUNT(*) FROM memory_contexts WHERE memory_id = @p1`, record.MemoryID).Scan(&existing)
	if err != nil {
		return fmt.Errorf("checking memory context: %w", err)
	}
	if existing == 0 {
		_, err = c.db.Exec(`
			INSERT INTO memory_contexts (
				memory_id, repository_id,
				scope_owner, scope_team, scope_workspace, scope_machine, scope_agent, scope_environment,
				provenance_transport, provenance_imported_from, provenance_human_authored, durable_at,
				created_at, updated_at
			) VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10, @p11, NULLIF(@p12, ''), GETUTCDATE(), GETUTCDATE())
		`,
			record.MemoryID, repositoryID,
			record.ScopeOwner, record.ScopeTeam, record.ScopeWorkspace, record.ScopeMachine, record.ScopeAgent, record.ScopeEnvironment,
			record.ProvenanceTransport, record.ProvenanceImportedFrom, record.ProvenanceHumanAuthored, record.DurableAt,
		)
	} else {
		_, err = c.db.Exec(`
			UPDATE memory_contexts SET
				repository_id = COALESCE(@p2, repository_id),
				scope_owner = CASE WHEN @p3 = '' THEN scope_owner ELSE @p3 END,
				scope_team = CASE WHEN @p4 = '' THEN scope_team ELSE @p4 END,
				scope_workspace = CASE WHEN @p5 = '' THEN scope_workspace ELSE @p5 END,
				scope_machine = CASE WHEN @p6 = '' THEN scope_machine ELSE @p6 END,
				scope_agent = CASE WHEN @p7 = '' THEN scope_agent ELSE @p7 END,
				scope_environment = CASE WHEN @p8 = '' THEN scope_environment ELSE @p8 END,
				provenance_transport = CASE WHEN @p9 = '' THEN provenance_transport ELSE @p9 END,
				provenance_imported_from = CASE WHEN @p10 = '' THEN provenance_imported_from ELSE @p10 END,
				provenance_human_authored = @p11,
				durable_at = COALESCE(NULLIF(@p12, ''), durable_at),
				updated_at = GETUTCDATE()
			WHERE memory_id = @p1
		`,
			record.MemoryID, repositoryID,
			record.ScopeOwner, record.ScopeTeam, record.ScopeWorkspace, record.ScopeMachine, record.ScopeAgent, record.ScopeEnvironment,
			record.ProvenanceTransport, record.ProvenanceImportedFrom, record.ProvenanceHumanAuthored, record.DurableAt,
		)
	}
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("upserting memory context: %w", err)
	}
	return nil
}
