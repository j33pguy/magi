package remember

import "testing"

func TestBuildEnvelope(t *testing.T) {
	env := BuildEnvelope(Input{
		Project:       "https://github.com/j33pguy/magi.git",
		Visibility:    "internal",
		Owner:         "j33p",
		Workspace:     "magi-lab",
		Machine:       "gilfoyle",
		Agent:         "claude-main",
		Environment:   "prod",
		Source:        "sync-import",
		Transport:     "http",
		ImportedFrom:  "claude-local-memory",
		HumanAuthored: true,
	})

	if env.Scope.Project != "https://github.com/j33pguy/magi.git" {
		t.Fatalf("scope.project = %q", env.Scope.Project)
	}
	if env.Scope.Owner != "j33p" || env.Scope.Machine != "gilfoyle" || env.Scope.Agent != "claude-main" {
		t.Fatalf("unexpected scope: %+v", env.Scope)
	}
	if env.Provenance.Transport != "http" || env.Provenance.ImportedFrom != "claude-local-memory" || !env.Provenance.HumanAuthored {
		t.Fatalf("unexpected provenance: %+v", env.Provenance)
	}
	if env.Repository.Canonical != "j33pguy/magi" || env.Repository.Host != "github.com" {
		t.Fatalf("unexpected repository: %+v", env.Repository)
	}
}

func TestInferRepositoryHandlesGitSSH(t *testing.T) {
	repo := InferRepository("git@github.com:j33pguy/magi.git", "", nil)
	if repo.Host != "github.com" || repo.Owner != "j33pguy" || repo.Name != "magi" || repo.Canonical != "j33pguy/magi" {
		t.Fatalf("unexpected repo: %+v", repo)
	}
}
