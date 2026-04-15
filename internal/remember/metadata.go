package remember

import (
	"path/filepath"
	"strings"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/project"
)

// Scope captures the redesign's intended scoping dimensions.
// These fields are not all persisted yet, but defining the shape now keeps
// future schema work aligned with one domain model instead of ad hoc tags.
type Scope struct {
	Project     string
	Visibility  string
	Owner       string
	Team        string
	Workspace   string
	Machine     string
	Agent       string
	Environment string
}

// Provenance captures who or what produced a memory and how it arrived.
type Provenance struct {
	Source        string
	Transport     string
	ImportedFrom  string
	Machine       string
	Agent         string
	HumanAuthored bool
	DurableAt     string
}

// RepositoryRef is the future first-class repository identity.
// For now we emit repo facets from it so existing clients can filter cheaply.
type RepositoryRef struct {
	Host      string
	Owner     string
	Name      string
	Canonical string
}

func BuildTags(input Input) []string {
	return buildTags(input.Tags, input.Project, input.SourceFile, input.Speaker, input.Area, input.SubArea)
}

func BuildTagsForMemory(mem *db.Memory, base []string) []string {
	if mem == nil {
		return dedupeTags(base)
	}
	return buildTags(base, mem.Project, mem.SourceFile, mem.Speaker, mem.Area, mem.SubArea)
}

func buildTags(base []string, projectName, sourceFile, speaker, area, subArea string) []string {
	tags := append([]string{}, base...)
	if speaker != "" {
		tags = append(tags, "speaker:"+speaker)
	}
	if area != "" {
		tags = append(tags, "area:"+area)
	}
	if subArea != "" {
		tags = append(tags, "sub_area:"+subArea)
	}
	if repo := InferRepository(projectName, sourceFile, tags); repo.Canonical != "" {
		tags = append(tags, "repo:"+repo.Canonical)
	}
	return dedupeTags(tags)
}

func InferRepository(projectName, sourceFile string, tags []string) RepositoryRef {
	for _, tag := range tags {
		if strings.HasPrefix(tag, "repo:") {
			if repo := parseRepository(strings.TrimPrefix(tag, "repo:")); repo.Canonical != "" {
				return repo
			}
		}
	}
	if repo := parseRepository(projectName); repo.Canonical != "" {
		return repo
	}
	if sourceFile != "" {
		dir := sourceFile
		if ext := filepath.Ext(sourceFile); ext != "" {
			dir = filepath.Dir(sourceFile)
		}
		if repo := parseRepository(project.DetectProject(dir)); repo.Canonical != "" {
			return repo
		}
	}
	return RepositoryRef{}
}

func parseRepository(raw string) RepositoryRef {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.TrimPrefix(raw, "repo:")
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimPrefix(raw, "ssh://")
	raw = strings.TrimSuffix(raw, ".git")
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return RepositoryRef{}
	}
	if at := strings.LastIndex(raw, "@"); at != -1 {
		raw = raw[at+1:]
	}
	if colon := strings.Index(raw, ":"); colon != -1 {
		left, right := raw[:colon], raw[colon+1:]
		if strings.Contains(right, "/") && !strings.Contains(left, "/") {
			raw = left + "/" + right
		}
	}

	parts := strings.Split(raw, "/")
	if len(parts) >= 3 {
		return RepositoryRef{Host: parts[len(parts)-3], Owner: parts[len(parts)-2], Name: parts[len(parts)-1], Canonical: parts[len(parts)-2] + "/" + parts[len(parts)-1]}
	}
	if len(parts) == 2 {
		return RepositoryRef{Owner: parts[0], Name: parts[1], Canonical: parts[0] + "/" + parts[1]}
	}
	return RepositoryRef{}
}

func dedupeTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
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
