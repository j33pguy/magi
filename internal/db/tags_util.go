package db

import "strings"

// normalizeTags trims, drops empties, and de-duplicates tags while preserving order.
func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return tags
	}
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
