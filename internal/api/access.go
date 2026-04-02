package api

import (
	"net/http"
	"strings"

	"github.com/j33pguy/magi/internal/db"
)

func applyRequestAccessScope(r *http.Request, filter *db.MemoryFilter) {
	if r == nil || filter == nil {
		return
	}

	user, groups, ok := requestAccessScope(r)
	if !ok {
		return
	}

	filter.RequestUser = user
	filter.RequestGroups = groups
	filter.EnforceAccess = true
}

func splitCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func requestAccessScope(r *http.Request) (string, []string, bool) {
	if r == nil {
		return "", nil, false
	}
	user := strings.TrimSpace(r.Header.Get("X-MAGI-Auth-User"))
	if user == "" {
		user = strings.TrimSpace(r.Header.Get("X-MAGI-User"))
	}
	if user == "" {
		user = strings.TrimSpace(r.URL.Query().Get("user"))
	}
	groupsHeader := r.Header.Get("X-MAGI-Auth-Groups")
	if groupsHeader == "" {
		groupsHeader = r.Header.Get("X-MAGI-Groups")
	}
	if groupsHeader == "" {
		groupsHeader = r.URL.Query().Get("groups")
	}
	groups := splitCSV(groupsHeader)
	return user, groups, user != "" || len(groups) > 0
}

func hasACLTags(tags []string) bool {
	for _, tag := range tags {
		if strings.HasPrefix(tag, "owner:") || strings.HasPrefix(tag, "viewer:") || strings.HasPrefix(tag, "viewer_group:") {
			return true
		}
	}
	return false
}

func canAccessTags(tags []string, user string, groups []string) bool {
	if !hasACLTags(tags) {
		return true
	}
	return matchesACLTags(tags, user, groups)
}

func matchesACLTags(tags []string, user string, groups []string) bool {
	for _, tag := range tags {
		if user != "" && (tag == "owner:"+user || tag == "viewer:"+user) {
			return true
		}
	}
	for _, group := range groups {
		if group == "" {
			continue
		}
		for _, tag := range tags {
			if tag == "viewer_group:"+group {
				return true
			}
		}
	}
	return false
}

func canAccessMemory(memory *db.Memory, tags []string, user string, groups []string) bool {
	if memory == nil {
		return false
	}
	if !hasACLTags(tags) {
		return memory.Visibility != "private"
	}
	return matchesACLTags(tags, user, groups)
}

func memoryAllowedForFilter(memory *db.Memory, tags []string, filter *db.MemoryFilter) bool {
	if memory == nil {
		return false
	}
	switch {
	case filter == nil:
		return memory.Visibility != "private"
	case filter.Visibility == "all":
	case filter.Visibility != "":
		if memory.Visibility != filter.Visibility {
			return false
		}
	default:
		if memory.Visibility == "private" {
			return false
		}
	}

	if !filter.EnforceAccess {
		return true
	}
	if !hasACLTags(tags) {
		return memory.Visibility != "private"
	}
	return matchesACLTags(tags, filter.RequestUser, filter.RequestGroups)
}
