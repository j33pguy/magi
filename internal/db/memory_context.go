package db

// RepositoryRecord captures the additive repository scaffolding introduced by
// migration v10. It is intentionally narrow and optional so current clients can
// start projecting canonical repository identity without changing read models.
type RepositoryRecord struct {
	Host                  string
	Owner                 string
	Name                  string
	CanonicalName         string
	DisplayName           string
	DefaultBranch         string
	IsFork                bool
	UpstreamCanonicalName string
}

// MemoryContextRecord captures redesign-oriented scope and provenance metadata
// for a memory. Persisting it is best-effort and does not replace existing tags.
type MemoryContextRecord struct {
	MemoryID                string
	Repository              RepositoryRecord
	ScopeOwner              string
	ScopeTeam               string
	ScopeWorkspace          string
	ScopeMachine            string
	ScopeAgent              string
	ScopeEnvironment        string
	ProvenanceTransport     string
	ProvenanceImportedFrom  string
	ProvenanceHumanAuthored bool
	DurableAt               string
}

func (mc MemoryContextRecord) Empty() bool {
	return mc.Repository.CanonicalName == "" &&
		mc.ScopeOwner == "" &&
		mc.ScopeTeam == "" &&
		mc.ScopeWorkspace == "" &&
		mc.ScopeMachine == "" &&
		mc.ScopeAgent == "" &&
		mc.ScopeEnvironment == "" &&
		mc.ProvenanceTransport == "" &&
		mc.ProvenanceImportedFrom == "" &&
		!mc.ProvenanceHumanAuthored &&
		mc.DurableAt == ""
}
