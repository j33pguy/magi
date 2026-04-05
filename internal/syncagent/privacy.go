package syncagent

import "regexp"

var sensitivePatterns = []*regexp.Regexp{
	// Generic key=value / key: value secrets
	regexp.MustCompile(`(?i)(api[_-]?key|secret[_-]?key|secret[_-]?id|access[_-]?key|private[_-]?key)\s*[:=]\s*["']?[A-Za-z0-9_\-\/+=.]{8,}["']?`),
	regexp.MustCompile(`(?i)(password|passwd|pass)\s*[:=]\s*["']?[^\s"'\n]{4,}["']?`),

	// Bearer tokens
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9_\-\/+=.]{8,}`),

	// HashiCorp Vault tokens (hvs., s., hvb.)
	regexp.MustCompile(`\bhvs\.[A-Za-z0-9]{10,}\b`),
	regexp.MustCompile(`\bhvb\.[A-Za-z0-9]{10,}\b`),
	regexp.MustCompile(`\bs\.[A-Za-z0-9]{20,}\b`),

	// Vault unseal keys (base64, typically 44+ chars)
	regexp.MustCompile(`(?i)unseal[_\s-]*key[s]?\s*[:=\[]\s*["']?[A-Za-z0-9+/=]{40,}["']?`),

	// X-Vault-Token header
	regexp.MustCompile(`(?i)X-Vault-Token\s*[:=]\s*["']?[A-Za-z0-9._\-]{8,}["']?`),

	// SSH private keys
	regexp.MustCompile(`(?s)-----BEGIN[A-Z\s]*PRIVATE KEY-----.*?-----END[A-Z\s]*PRIVATE KEY-----`),

	// AWS-style keys
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`(?i)aws[_-]?secret[_-]?access[_-]?key\s*[:=]\s*["']?[A-Za-z0-9/+=]{30,}["']?`),

	// GitHub tokens
	regexp.MustCompile(`\b(ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{36,}\b`),

	// Generic hex tokens (standalone 32+ char hex strings that look like API keys)
	regexp.MustCompile(`(?i)(token|secret)\s*[:=]\s*["']?[a-f0-9]{32,}["']?`),

	// ansible-vault encrypted blocks
	regexp.MustCompile(`(?s)\$ANSIBLE_VAULT;[0-9.]+;AES256\n[0-9a-f\s]+`),

	// YAML secrets block patterns (vault_unseal_keys, vault_root_token, etc.)
	regexp.MustCompile(`(?i)vault_root_token\s*:\s*["']?[^\s"'\n]{4,}["']?`),
	regexp.MustCompile(`(?i)vault_unseal_key[s_0-9]*\s*:\s*["']?[A-Za-z0-9+/=]{20,}["']?`),
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
