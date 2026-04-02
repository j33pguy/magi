package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/j33pguy/magi/internal/db"
)

type Identity struct {
	Kind      string
	User      string
	MachineID string
	AgentName string
	AgentType string
	Groups    []string
}

type MachineToken struct {
	Token     string   `json:"token"`
	User      string   `json:"user"`
	MachineID string   `json:"machine_id"`
	AgentName string   `json:"agent_name"`
	AgentType string   `json:"agent_type"`
	Groups    []string `json:"groups"`
}

type Resolver struct {
	adminToken string
	machines   []MachineToken
	lookup     MachineLookup
}

type MachineLookup interface {
	GetMachineCredentialByTokenHash(tokenHash string) (*db.MachineCredential, error)
	TouchMachineCredential(id string) error
}

func LoadResolverFromEnv() (*Resolver, error) {
	r := &Resolver{
		adminToken: os.Getenv("MAGI_API_TOKEN"),
	}

	raw := os.Getenv("MAGI_MACHINE_TOKENS_JSON")
	if raw == "" {
		if path := os.Getenv("MAGI_MACHINE_TOKENS_FILE"); path != "" {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("reading machine tokens file: %w", err)
			}
			raw = string(data)
		}
	}
	if strings.TrimSpace(raw) == "" {
		return r, nil
	}

	if err := json.Unmarshal([]byte(raw), &r.machines); err != nil {
		return nil, fmt.Errorf("parsing machine tokens: %w", err)
	}
	return r, nil
}

func (r *Resolver) Enabled() bool {
	return r != nil && (r.adminToken != "" || len(r.machines) > 0)
}

func (r *Resolver) ResolveBearer(token string) (*Identity, bool) {
	if r == nil {
		return nil, false
	}
	if r.adminToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(r.adminToken)) == 1 {
		return &Identity{Kind: "admin"}, true
	}
	for _, m := range r.machines {
		if m.Token == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(m.Token)) == 1 {
			return &Identity{
				Kind:      "machine",
				User:      m.User,
				MachineID: m.MachineID,
				AgentName: m.AgentName,
				AgentType: m.AgentType,
				Groups:    dedupeStrings(m.Groups),
			}, true
		}
	}
	if r.lookup != nil {
		cred, err := r.lookup.GetMachineCredentialByTokenHash(HashToken(token))
		if err == nil && cred != nil {
			_ = r.lookup.TouchMachineCredential(cred.ID)
			return &Identity{
				Kind:      "machine",
				User:      cred.User,
				MachineID: cred.MachineID,
				AgentName: cred.AgentName,
				AgentType: cred.AgentType,
				Groups:    dedupeStrings(cred.Groups),
			}, true
		}
	}
	return nil, false
}

func (r *Resolver) AdminToken() string {
	if r == nil {
		return ""
	}
	return r.adminToken
}

func (r *Resolver) SetMachineLookup(lookup MachineLookup) {
	if r == nil {
		return
	}
	r.lookup = lookup
}

func GenerateToken() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	return token, HashToken(token), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

type contextKey string

const identityContextKey contextKey = "magi_auth_identity"

func NewContext(ctx context.Context, identity *Identity) context.Context {
	if identity == nil {
		return ctx
	}
	return context.WithValue(ctx, identityContextKey, identity)
}

func FromContext(ctx context.Context) (*Identity, bool) {
	if ctx == nil {
		return nil, false
	}
	identity, ok := ctx.Value(identityContextKey).(*Identity)
	return identity, ok && identity != nil
}

func IsAdmin(identity *Identity) bool {
	return identity != nil && identity.Kind == "admin"
}

func EffectiveUser(identity *Identity) string {
	if identity == nil {
		return ""
	}
	if strings.TrimSpace(identity.User) != "" {
		return strings.TrimSpace(identity.User)
	}
	if identity.Kind == "admin" {
		return "admin"
	}
	return ""
}

func OwnerTag(identity *Identity) string {
	user := EffectiveUser(identity)
	if user == "" {
		return ""
	}
	return "owner:" + user
}

func CanModifyTags(identity *Identity, tags []string) bool {
	if IsAdmin(identity) {
		return true
	}
	ownerTag := OwnerTag(identity)
	if ownerTag == "" {
		return false
	}
	for _, tag := range tags {
		if tag == ownerTag {
			return true
		}
	}
	return false
}

func ApplyToFilter(ctx context.Context, filter *db.MemoryFilter) {
	identity, ok := FromContext(ctx)
	if !ok || filter == nil {
		return
	}
	if identity.Kind != "machine" {
		return
	}
	filter.RequestUser = identity.User
	filter.RequestGroups = append([]string{}, identity.Groups...)
	filter.EnforceAccess = true
}
