package project

import (
	"path/filepath"
	"testing"
)

func TestParseRemote_HTTPS(t *testing.T) {
	tests := []struct {
		name   string
		remote string
		want   string
	}{
		{"github https", "https://github.com/org/repo.git", "github.com/org/repo"},
		{"github https no git suffix", "https://github.com/org/repo", "github.com/org/repo"},
		{"github https trailing slash", "https://github.com/org/repo/", "github.com/org/repo"},
		{"gitlab https", "https://gitlab.com/group/subgroup/project.git", "gitlab.com/group/subgroup/project"},
		{"custom host", "https://git.example.com/team/project.git", "git.example.com/team/project"},
		{"https host only", "https://github.com/", ""},
		{"https host only no slash", "https://github.com", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRemote(tt.remote)
			if got != tt.want {
				t.Errorf("parseRemote(%q) = %q, want %q", tt.remote, got, tt.want)
			}
		})
	}
}

func TestParseRemote_SSH(t *testing.T) {
	tests := []struct {
		name   string
		remote string
		want   string
	}{
		{"github ssh", "git@github.com:org/repo.git", "github.com/org/repo"},
		{"github ssh no git suffix", "git@github.com:org/repo", "github.com/org/repo"},
		{"gitlab ssh", "git@gitlab.com:group/project.git", "gitlab.com/group/project"},
		{"custom ssh user", "deploy@git.example.com:team/project.git", "git.example.com/team/project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRemote(tt.remote)
			if got != tt.want {
				t.Errorf("parseRemote(%q) = %q, want %q", tt.remote, got, tt.want)
			}
		})
	}
}

func TestParseRemote_SimplePath(t *testing.T) {
	tests := []struct {
		name   string
		remote string
		want   string
	}{
		{"simple path", "org/repo", "org/repo"},
		{"deep path", "org/sub/repo", "org/sub/repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRemote(tt.remote)
			if got != tt.want {
				t.Errorf("parseRemote(%q) = %q, want %q", tt.remote, got, tt.want)
			}
		})
	}
}

func TestParseRemote_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		remote string
		want   string
	}{
		{"empty", "", ""},
		{"whitespace", "   ", ""},
		{"single word", "repo", ""},
		{"trailing slash stripped", "org/repo/", "org/repo"},
		{"git suffix stripped", "org/repo.git", "org/repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRemote(tt.remote)
			if got != tt.want {
				t.Errorf("parseRemote(%q) = %q, want %q", tt.remote, got, tt.want)
			}
		})
	}
}

func TestDetectProject_FallbackToBasename(t *testing.T) {
	// Use a directory that is not a git repo
	dir := t.TempDir()
	got := DetectProject(dir)
	want := filepath.Base(dir)
	if got != want {
		t.Errorf("DetectProject(%q) = %q, want %q", dir, got, want)
	}
}

func TestDetectProject_EmptyDir(t *testing.T) {
	// When run from a git repo, DetectProject("") may find the repo's remote.
	// When run outside a git repo, it falls back to ".".
	// Either way, it should return a non-empty string.
	got := DetectProject("")
	if got == "" {
		t.Error("DetectProject('') should return a non-empty string")
	}
}
