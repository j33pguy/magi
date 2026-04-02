package syncagent

import (
	"path/filepath"
	"regexp"
	"strings"
)

func shouldInclude(rel string, includes, excludes []string) bool {
	rel = filepath.ToSlash(rel)
	for _, pattern := range excludes {
		if matchGlob(pattern, rel) {
			return false
		}
	}
	if len(includes) == 0 {
		return true
	}
	for _, pattern := range includes {
		if matchGlob(pattern, rel) {
			return true
		}
	}
	return false
}

func matchGlob(pattern, target string) bool {
	pattern = filepath.ToSlash(pattern)
	target = filepath.ToSlash(target)
	re := globToRegexp(pattern)
	return re.MatchString(target)
}

func globToRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i += 2
			} else {
				b.WriteString(`[^/]*`)
				i++
			}
		case '?':
			b.WriteString(".")
			i++
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
			i++
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}
