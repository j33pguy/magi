package db

import "fmt"

func (c *PostgresClient) LookupMemoryContexts(memoryIDs []string) (map[string]MemoryContextLookup, error) {
	contexts, err := lookupMemoryContexts(c.DB, func(i int) string { return fmt.Sprintf("$%d", i+1) }, memoryIDs)
	if err != nil {
		return nil, fmt.Errorf("loading memory contexts: %w", err)
	}
	return contexts, nil
}

func (c *PostgresClient) SaveMemoryContext(record *MemoryContextRecord) error {
	if record == nil || record.MemoryID == "" || record.Empty() {
		return nil
	}

	var repositoryID any
	if repo := record.Repository; repo.CanonicalName != "" {
		var id string
		err := c.DB.QueryRow(`
			INSERT INTO repositories (host, owner, name, canonical_name, display_name, default_branch, is_fork, upstream_canonical_name)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (canonical_name) DO UPDATE SET
				host = COALESCE(NULLIF(EXCLUDED.host, ''), repositories.host),
				owner = COALESCE(NULLIF(EXCLUDED.owner, ''), repositories.owner),
				name = COALESCE(NULLIF(EXCLUDED.name, ''), repositories.name),
				display_name = COALESCE(NULLIF(EXCLUDED.display_name, ''), repositories.display_name),
				default_branch = COALESCE(NULLIF(EXCLUDED.default_branch, ''), repositories.default_branch),
				is_fork = EXCLUDED.is_fork,
				upstream_canonical_name = COALESCE(NULLIF(EXCLUDED.upstream_canonical_name, ''), repositories.upstream_canonical_name),
				updated_at = NOW() AT TIME ZONE 'UTC'
			RETURNING id
		`, repo.Host, repo.Owner, repo.Name, repo.CanonicalName, repo.DisplayName, repo.DefaultBranch, repo.IsFork, repo.UpstreamCanonicalName).Scan(&id)
		if err != nil {
			return fmt.Errorf("upserting repository context: %w", err)
		}
		repositoryID = id
	}

	_, err := c.DB.Exec(`
		INSERT INTO memory_contexts (
			memory_id, repository_id,
			scope_owner, scope_team, scope_workspace, scope_machine, scope_agent, scope_environment,
			provenance_transport, provenance_imported_from, provenance_human_authored, durable_at,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''), NOW() AT TIME ZONE 'UTC', NOW() AT TIME ZONE 'UTC')
		ON CONFLICT (memory_id) DO UPDATE SET
			repository_id = COALESCE(EXCLUDED.repository_id, memory_contexts.repository_id),
			scope_owner = COALESCE(NULLIF(EXCLUDED.scope_owner, ''), memory_contexts.scope_owner),
			scope_team = COALESCE(NULLIF(EXCLUDED.scope_team, ''), memory_contexts.scope_team),
			scope_workspace = COALESCE(NULLIF(EXCLUDED.scope_workspace, ''), memory_contexts.scope_workspace),
			scope_machine = COALESCE(NULLIF(EXCLUDED.scope_machine, ''), memory_contexts.scope_machine),
			scope_agent = COALESCE(NULLIF(EXCLUDED.scope_agent, ''), memory_contexts.scope_agent),
			scope_environment = COALESCE(NULLIF(EXCLUDED.scope_environment, ''), memory_contexts.scope_environment),
			provenance_transport = COALESCE(NULLIF(EXCLUDED.provenance_transport, ''), memory_contexts.provenance_transport),
			provenance_imported_from = COALESCE(NULLIF(EXCLUDED.provenance_imported_from, ''), memory_contexts.provenance_imported_from),
			provenance_human_authored = EXCLUDED.provenance_human_authored,
			durable_at = COALESCE(EXCLUDED.durable_at, memory_contexts.durable_at),
			updated_at = NOW() AT TIME ZONE 'UTC'
	`,
		record.MemoryID, repositoryID,
		record.ScopeOwner, record.ScopeTeam, record.ScopeWorkspace, record.ScopeMachine, record.ScopeAgent, record.ScopeEnvironment,
		record.ProvenanceTransport, record.ProvenanceImportedFrom, record.ProvenanceHumanAuthored, record.DurableAt,
	)
	if err != nil {
		return fmt.Errorf("upserting memory context: %w", err)
	}
	return nil
}
