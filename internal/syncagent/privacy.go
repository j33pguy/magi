package syncagent

import "regexp"

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password)\s*[:=]\s*["']?[A-Za-z0-9_\-\/+=]{8,}`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9_\-\/+=.]{8,}`),
}

func applyPrivacy(content string, cfg PrivacyConfig) string {
	if !cfg.RedactSecrets {
		return content
	}
	redacted := content
	for _, re := range sensitivePatterns {
		redacted = re.ReplaceAllString(redacted, "[REDACTED]")
	}
	return redacted
}
