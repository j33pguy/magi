package remember

import "strings"

// Envelope is the redesign-oriented domain wrapper around the current memory row.
// It lets new scope, provenance, and repository identity concepts take shape
// without breaking existing remember callers or storage clients.
type Envelope struct {
	Scope      Scope
	Provenance Provenance
	Repository RepositoryRef
}

// BuildEnvelope projects today's remember input into the richer target model.
func BuildEnvelope(input Input) Envelope {
	scope := Scope{
		Project:     input.Project,
		Visibility:  input.Visibility,
		Owner:       input.Owner,
		Team:        input.Team,
		Workspace:   input.Workspace,
		Machine:     input.Machine,
		Agent:       input.Agent,
		Environment: input.Environment,
	}
	provenance := Provenance{
		Source:        input.Source,
		Transport:     input.Transport,
		ImportedFrom:  input.ImportedFrom,
		Machine:       input.Machine,
		Agent:         input.Agent,
		HumanAuthored: input.HumanAuthored,
	}
	return Envelope{
		Scope:      normalizeScope(scope),
		Provenance: provenance,
		Repository: InferRepository(input.Project, "", input.Tags),
	}
}

func normalizeScope(scope Scope) Scope {
	scope.Project = strings.TrimSpace(scope.Project)
	scope.Visibility = strings.TrimSpace(scope.Visibility)
	scope.Owner = strings.TrimSpace(scope.Owner)
	scope.Team = strings.TrimSpace(scope.Team)
	scope.Workspace = strings.TrimSpace(scope.Workspace)
	scope.Machine = strings.TrimSpace(scope.Machine)
	scope.Agent = strings.TrimSpace(scope.Agent)
	scope.Environment = strings.TrimSpace(scope.Environment)
	return scope
}
