package secretstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"
)

type Config struct {
	Mode           string
	Backend        string
	VaultAddr      string
	VaultToken     string
	VaultMount     string
	VaultNamespace string
}

type Manager interface {
	Externalize(ctx context.Context, project, content string) (*ExternalizeResult, error)
	Resolve(ctx context.Context, path, key string) (string, error)
	BackendName() string
}

type ExternalizeResult struct {
	RedactedContent string
	Refs            []Reference
}

type Reference struct {
	Backend string `json:"backend"`
	Path    string `json:"path"`
	Key     string `json:"key"`
}

type backend interface {
	Name() string
	Put(ctx context.Context, path string, data map[string]string) error
	Get(ctx context.Context, path string) (map[string]string, error)
}

type manager struct {
	backend backend
	logger  *slog.Logger
}

type extractedSecret struct {
	Key   string
	Value string
}

var secretKVPattern = regexp.MustCompile(`(?im)\b(api[_-]?key|api[_-]?secret|access[_-]?token|auth[_-]?token|secret[_-]?key|password|passwd|pwd)\b\s*[:=]\s*["']?([^\s"']+)["']?`)

func ConfigFromEnv() Config {
	cfg := Config{
		Mode:           strings.TrimSpace(strings.ToLower(os.Getenv("MAGI_SECRET_MODE"))),
		Backend:        strings.TrimSpace(strings.ToLower(os.Getenv("MAGI_SECRET_BACKEND"))),
		VaultAddr:      strings.TrimSpace(os.Getenv("MAGI_VAULT_ADDR")),
		VaultToken:     strings.TrimSpace(os.Getenv("MAGI_VAULT_TOKEN")),
		VaultMount:     strings.TrimSpace(os.Getenv("MAGI_VAULT_MOUNT")),
		VaultNamespace: strings.TrimSpace(os.Getenv("MAGI_VAULT_NAMESPACE")),
	}
	if cfg.Mode == "" {
		cfg.Mode = "reject"
	}
	if cfg.VaultMount == "" {
		cfg.VaultMount = "secret"
	}
	return cfg
}

func NewFromEnv(logger *slog.Logger) (Manager, error) {
	cfg := ConfigFromEnv()
	if cfg.Mode != "externalize" {
		return nil, nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	switch cfg.Backend {
	case "vault", "vault_kv":
		if cfg.VaultAddr == "" || cfg.VaultToken == "" {
			return nil, fmt.Errorf("vault backend requires MAGI_VAULT_ADDR and MAGI_VAULT_TOKEN")
		}
		return &manager{
			backend: newVaultBackend(cfg),
			logger:  logger,
		}, nil
	case "", "none":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported secret backend %q", cfg.Backend)
	}
}

func (m *manager) BackendName() string {
	if m == nil || m.backend == nil {
		return ""
	}
	return m.backend.Name()
}

func (m *manager) Externalize(ctx context.Context, project, content string) (*ExternalizeResult, error) {
	secrets := extractSecrets(content)
	if len(secrets) == 0 {
		return nil, fmt.Errorf("no extractable secrets found")
	}
	path := buildSecretPath(project)
	payload := make(map[string]string, len(secrets))
	redacted := content
	refs := make([]Reference, 0, len(secrets))
	for _, secret := range secrets {
		payload[secret.Key] = secret.Value
		ref := Reference{Backend: m.backend.Name(), Path: path, Key: secret.Key}
		refs = append(refs, ref)
		redacted = strings.ReplaceAll(redacted, secret.Value, renderReference(ref))
	}
	if err := m.backend.Put(ctx, path, payload); err != nil {
		return nil, err
	}
	return &ExternalizeResult{RedactedContent: redacted, Refs: refs}, nil
}

func (m *manager) Resolve(ctx context.Context, path, key string) (string, error) {
	data, err := m.backend.Get(ctx, path)
	if err != nil {
		return "", err
	}
	value, ok := data[key]
	if !ok {
		return "", fmt.Errorf("secret key %q not found at %q", key, path)
	}
	return value, nil
}

func extractSecrets(content string) []extractedSecret {
	matches := secretKVPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]int{}
	var out []extractedSecret
	for _, match := range matches {
		key := strings.ToLower(match[1])
		value := strings.TrimSpace(match[2])
		if value == "" {
			continue
		}
		seen[key]++
		if seen[key] > 1 {
			key = fmt.Sprintf("%s_%d", key, seen[key])
		}
		out = append(out, extractedSecret{Key: key, Value: value})
	}
	return out
}

func buildSecretPath(project string) string {
	project = sanitizePathPart(project)
	if project == "" {
		project = "default"
	}
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	return fmt.Sprintf("magi/%s/%d-%s", project, time.Now().UTC().Unix(), hex.EncodeToString(buf))
}

func sanitizePathPart(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '/' || r == '-' || r == '_' || r == '.':
			b.WriteRune('/')
		}
	}
	out := strings.Trim(b.String(), "/")
	out = regexp.MustCompile(`/+`).ReplaceAllString(out, "/")
	return out
}

func renderReference(ref Reference) string {
	return fmt.Sprintf("[stored:%s://%s#%s]", ref.Backend, ref.Path, ref.Key)
}
