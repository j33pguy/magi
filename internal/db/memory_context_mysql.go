package db

import "fmt"

func (c *MySQLClient) LookupMemoryContexts(memoryIDs []string) (map[string]MemoryContextLookup, error) {
	contexts, err := lookupMemoryContexts(c.DB, func(int) string { return "?" }, memoryIDs)
	if err != nil {
		return nil, fmt.Errorf("loading memory contexts: %w", err)
	}
	return contexts, nil
}

func (c *MySQLClient) SaveMemoryContext(record *MemoryContextRecord) error {
	if record == nil || record.MemoryID == "" || record.Empty() {
		return nil
	}

	var repositoryID any
	if repo := record.Repository; repo.CanonicalName != "" {
		id := newHexID()
		_, err := c.DB.Exec(`
			INSERT INTO repositories (id, host, owner, name, canonical_name, display_name, default_branch, is_fork, upstream_canonical_name)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				host = COALESCE(NULLIF(VALUES(host), ''), host),
				owner = COALESCE(NULLIF(VALUES(owner), ''), owner),
				name = COALESCE(NULLIF(VALUES(name), ''), name),
				display_name = COALESCE(NULLIF(VALUES(display_name), ''), display_name),
				default_branch = COALESCE(NULLIF(VALUES(default_branch), ''), default_branch),
				is_fork = VALUES(is_fork),
				upstream_canonical_name = COALESCE(NULLIF(VALUES(upstream_canonical_name), ''), upstream_canonical_name),
				updated_at = CURRENT_TIMESTAMP,
				id = LAST_INSERT_ID(id)
		`, id, repo.Host, repo.Owner, repo.Name, repo.CanonicalName, repo.DisplayName, repo.DefaultBranch, repo.IsFork, repo.UpstreamCanonicalName)
		if err != nil {
			return fmt.Errorf("upserting repository context: %w", err)
		}
		var numericID int64
		if err := c.DB.QueryRow(`SELECT LAST_INSERT_ID()`).Scan(&numericID); err == nil && numericID > 0 {
			repositoryID = fmt.Sprintf("%x", numericID)
		} else {
			repositoryID = id
		}
		// Use canonical lookup so duplicate-key updates return the stable row id.
		if err := c.DB.QueryRow(`SELECT id FROM repositories WHERE canonical_name = ?`, repo.CanonicalName).Scan(&repositoryID); err != nil {
			return fmt.Errorf("loading repository context id: %w", err)
		}
	}

	_, err := c.DB.Exec(`
		INSERT INTO memory_contexts (
			memory_id, repository_id,
			scope_owner, scope_team, scope_workspace, scope_machine, scope_agent, scope_environment,
			provenance_transport, provenance_imported_from, provenance_human_authored, durable_at,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON DUPLICATE KEY UPDATE
			repository_id = COALESCE(VALUES(repository_id), repository_id),
			scope_owner = COALESCE(NULLIF(VALUES(scope_owner), ''), scope_owner),
			scope_team = COALESCE(NULLIF(VALUES(scope_team), ''), scope_team),
			scope_workspace = COALESCE(NULLIF(VALUES(scope_workspace), ''), scope_workspace),
			scope_machine = COALESCE(NULLIF(VALUES(scope_machine), ''), scope_machine),
			scope_agent = COALESCE(NULLIF(VALUES(scope_agent), ''), scope_agent),
			scope_environment = COALESCE(NULLIF(VALUES(scope_environment), ''), scope_environment),
			provenance_transport = COALESCE(NULLIF(VALUES(provenance_transport), ''), provenance_transport),
			provenance_imported_from = COALESCE(NULLIF(VALUES(provenance_imported_from), ''), provenance_imported_from),
			provenance_human_authored = VALUES(provenance_human_authored),
			durable_at = COALESCE(NULLIF(VALUES(durable_at), ''), durable_at),
			updated_at = CURRENT_TIMESTAMP
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
