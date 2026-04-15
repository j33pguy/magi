package db

import "fmt"

func (c *Client) SaveMemoryContext(record *MemoryContextRecord) error {
	if record == nil || record.MemoryID == "" || record.Empty() {
		return nil
	}

	var repositoryID any
	if repo := record.Repository; repo.CanonicalName != "" {
		var id string
		err := c.DB.QueryRow(`
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
		`, repo.Host, repo.Owner, repo.Name, repo.CanonicalName, repo.DisplayName, repo.DefaultBranch, boolToInt(repo.IsFork), repo.UpstreamCanonicalName).Scan(&id)
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
		record.MemoryID, repositoryID,
		record.ScopeOwner, record.ScopeTeam, record.ScopeWorkspace, record.ScopeMachine, record.ScopeAgent, record.ScopeEnvironment,
		record.ProvenanceTransport, record.ProvenanceImportedFrom, boolToInt(record.ProvenanceHumanAuthored), nullString(record.DurableAt),
	)
	if err != nil {
		return fmt.Errorf("upserting memory context: %w", err)
	}
	return nil
}

func (c *Client) LookupMemoryContexts(memoryIDs []string) (map[string]MemoryContextLookup, error) {
	contexts, err := lookupMemoryContexts(c.DB, func(int) string { return "?" }, memoryIDs)
	if err != nil {
		return nil, fmt.Errorf("loading memory contexts: %w", err)
	}
	return contexts, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
