package syncagent

import (
	"testing"
)

func TestPrivacyPatterns(t *testing.T) {
	cfg := PrivacyConfig{RedactSecrets: true}

	tests := []struct {
		name  string
		input string
		dirty bool // should contain [REDACTED] after
	}{
		{"hvs token inline", "Root token: " + "hvs" + ".AAAAAAAAAAAAAAAAAAAAAA", true},
		{"hvs token standalone", "used " + "hvs" + ".BBBBBBBBBBBBBBBBBBBBBB" + " to login", true},
		{"vault root yaml", "vault_root_token: " + "hvs" + ".AAAAAAAAAAAAAAAAAAAAAA", true},
		{"bearer token", "Authorization: Bearer AAAAAAAAAAAABBBBBBBBBBBBCCCCCCCCCCCC", true},
		{"X-Vault-Token header", "X-Vault-Token: " + "hvs" + ".CCCCCCCCCCCCCCCCCCCCCC", true},
		{"unseal key yaml", "vault_unseal_key_1: dGhpcyBpcyBhIHRlc3Qga2V5IHRoYXQgaXMgbG9uZyBlbm91Z2g=", true},
		{"secret_id uuid", "secret_id: 00000000-0000-0000-0000-000000000000", true},
		{"github token", "ghp_" + "AAAAAAAAAAAABBBBBBBBBBBBCCCCCCCCCCDDDDDD", true},
		{"safe text", "The vault was unsealed successfully at 3pm", false},
		{"safe mention", "We use HashiCorp Vault for secrets", false},
		{"ssh private key", "-----BEGIN OPENSSH PRIVATE KEY-----\nbase64stuff\n-----END OPENSSH PRIVATE KEY-----", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyPrivacy(tt.input, cfg)
			hasRedacted := result != tt.input
			if tt.dirty && !hasRedacted {
				t.Errorf("expected redaction but got unchanged: %s", result)
			}
			if !tt.dirty && hasRedacted {
				t.Errorf("unexpected redaction: %q -> %q", tt.input, result)
			}
		})
	}
}
