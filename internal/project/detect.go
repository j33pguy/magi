package project

import (
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
)

// DetectProject returns the canonical project key for a directory.
// It attempts to read the git origin remote and falls back to the
// directory basename when git metadata is unavailable.
func DetectProject(dir string) string {
	remote := gitOrigin(dir)
	if key := parseRemote(remote); key != "" {
		return key
	}

	if dir == "" {
		dir = "."
	}
	return filepath.Base(dir)
}

func gitOrigin(dir string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func parseRemote(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}
	remote = strings.TrimSuffix(remote, "/")
	remote = strings.TrimSuffix(remote, ".git")

	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err == nil && u.Host != "" {
			path := strings.TrimPrefix(u.Path, "/")
			if path == "" {
				return ""
			}
			return u.Host + "/" + path
		}
	}

	if strings.Contains(remote, ":") {
		parts := strings.SplitN(remote, ":", 2)
		if len(parts) == 2 {
			hostPart := parts[0]
			path := parts[1]
			if at := strings.LastIndex(hostPart, "@"); at != -1 {
				hostPart = hostPart[at+1:]
			}
			path = strings.TrimPrefix(path, "/")
			if hostPart != "" && path != "" {
				return hostPart + "/" + path
			}
		}
	}

	if strings.Contains(remote, "/") {
		return remote
	}

	return ""
}
