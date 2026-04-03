package auth

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/j33pguy/magi/internal/db"
)

// --- LoadResolverFromEnv ---

func TestLoadResolverFromEnv_NoVars(t *testing.T) {
	t.Setenv("MAGI_API_TOKEN", "")
	t.Setenv("MAGI_MACHINE_TOKENS_JSON", "")
	t.Setenv("MAGI_MACHINE_TOKENS_FILE", "")

	r, err := LoadResolverFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Enabled() {
		t.Error("resolver should not be enabled with no vars")
	}
}

func TestLoadResolverFromEnv_AdminToken(t *testing.T) {
	t.Setenv("MAGI_API_TOKEN", "my-secret")
	t.Setenv("MAGI_MACHINE_TOKENS_JSON", "")
	t.Setenv("MAGI_MACHINE_TOKENS_FILE", "")

	r, err := LoadResolverFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Enabled() {
		t.Error("resolver should be enabled with admin token")
	}
	if r.AdminToken() != "my-secret" {
		t.Errorf("expected admin token 'my-secret', got %q", r.AdminToken())
	}
}

func TestLoadResolverFromEnv_MachineTokensJSON(t *testing.T) {
	tokens := []MachineToken{
		{Token: "tok1", User: "alice", MachineID: "m1", AgentName: "bot1"},
		{Token: "tok2", User: "bob", MachineID: "m2"},
	}
	data, _ := json.Marshal(tokens)

	t.Setenv("MAGI_API_TOKEN", "")
	t.Setenv("MAGI_MACHINE_TOKENS_JSON", string(data))
	t.Setenv("MAGI_MACHINE_TOKENS_FILE", "")

	r, err := LoadResolverFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Enabled() {
		t.Error("resolver should be enabled with machine tokens")
	}
	if len(r.machines) != 2 {
		t.Errorf("expected 2 machines, got %d", len(r.machines))
	}
}

func TestLoadResolverFromEnv_MachineTokensFile(t *testing.T) {
	tokens := []MachineToken{{Token: "file-tok", User: "carol", MachineID: "m3"}}
	data, _ := json.Marshal(tokens)
	path := filepath.Join(t.TempDir(), "tokens.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("writing tokens file: %v", err)
	}

	t.Setenv("MAGI_API_TOKEN", "")
	t.Setenv("MAGI_MACHINE_TOKENS_JSON", "")
	t.Setenv("MAGI_MACHINE_TOKENS_FILE", path)

	r, err := LoadResolverFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Enabled() {
		t.Error("resolver should be enabled with file tokens")
	}
	if len(r.machines) != 1 {
		t.Errorf("expected 1 machine, got %d", len(r.machines))
	}
}

func TestLoadResolverFromEnv_InvalidJSON(t *testing.T) {
	t.Setenv("MAGI_API_TOKEN", "")
	t.Setenv("MAGI_MACHINE_TOKENS_JSON", "not-json")
	t.Setenv("MAGI_MACHINE_TOKENS_FILE", "")

	_, err := LoadResolverFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadResolverFromEnv_MissingFile(t *testing.T) {
	t.Setenv("MAGI_API_TOKEN", "")
	t.Setenv("MAGI_MACHINE_TOKENS_JSON", "")
	t.Setenv("MAGI_MACHINE_TOKENS_FILE", "/nonexistent/tokens.json")

	_, err := LoadResolverFromEnv()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadResolverFromEnv_WhitespaceJSON(t *testing.T) {
	t.Setenv("MAGI_API_TOKEN", "")
	t.Setenv("MAGI_MACHINE_TOKENS_JSON", "   ")
	t.Setenv("MAGI_MACHINE_TOKENS_FILE", "")

	r, err := LoadResolverFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Enabled() {
		t.Error("resolver should not be enabled with whitespace-only JSON")
	}
}

// --- ResolveBearer ---

func TestResolveBearer_AdminToken(t *testing.T) {
	t.Setenv("MAGI_API_TOKEN", "admin-secret")
	t.Setenv("MAGI_MACHINE_TOKENS_JSON", "")
	t.Setenv("MAGI_MACHINE_TOKENS_FILE", "")

	r, _ := LoadResolverFromEnv()
	identity, ok := r.ResolveBearer("admin-secret")
	if !ok {
		t.Fatal("expected successful resolve")
	}
	if identity.Kind != "admin" {
		t.Errorf("expected kind 'admin', got %q", identity.Kind)
	}
}

func TestResolveBearer_MachineToken(t *testing.T) {
	tokens := []MachineToken{
		{Token: "machine-tok", User: "alice", MachineID: "m1", AgentName: "bot", AgentType: "claude", Groups: []string{"dev", "ops"}},
	}
	data, _ := json.Marshal(tokens)

	t.Setenv("MAGI_API_TOKEN", "")
	t.Setenv("MAGI_MACHINE_TOKENS_JSON", string(data))
	t.Setenv("MAGI_MACHINE_TOKENS_FILE", "")

	r, _ := LoadResolverFromEnv()
	identity, ok := r.ResolveBearer("machine-tok")
	if !ok {
		t.Fatal("expected successful resolve")
	}
	if identity.Kind != "machine" {
		t.Errorf("expected kind 'machine', got %q", identity.Kind)
	}
	if identity.User != "alice" {
		t.Errorf("expected user 'alice', got %q", identity.User)
	}
	if identity.MachineID != "m1" {
		t.Errorf("expected machine_id 'm1', got %q", identity.MachineID)
	}
	if identity.AgentName != "bot" {
		t.Errorf("expected agent_name 'bot', got %q", identity.AgentName)
	}
	if len(identity.Groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(identity.Groups))
	}
}

func TestResolveBearer_InvalidToken(t *testing.T) {
	t.Setenv("MAGI_API_TOKEN", "admin-secret")
	t.Setenv("MAGI_MACHINE_TOKENS_JSON", "")
	t.Setenv("MAGI_MACHINE_TOKENS_FILE", "")

	r, _ := LoadResolverFromEnv()
	_, ok := r.ResolveBearer("wrong-token")
	if ok {
		t.Error("expected resolve to fail for invalid token")
	}
}

func TestResolveBearer_EmptyToken(t *testing.T) {
	t.Setenv("MAGI_API_TOKEN", "admin-secret")
	t.Setenv("MAGI_MACHINE_TOKENS_JSON", "")
	t.Setenv("MAGI_MACHINE_TOKENS_FILE", "")

	r, _ := LoadResolverFromEnv()
	_, ok := r.ResolveBearer("")
	if ok {
		t.Error("expected resolve to fail for empty token")
	}
}

func TestResolveBearer_NilResolver(t *testing.T) {
	var r *Resolver
	_, ok := r.ResolveBearer("any-token")
	if ok {
		t.Error("expected resolve to fail on nil resolver")
	}
}

func TestResolveBearer_SkipsEmptyMachineToken(t *testing.T) {
	tokens := []MachineToken{
		{Token: "", User: "ghost", MachineID: "m0"},
		{Token: "real-tok", User: "alice", MachineID: "m1"},
	}
	data, _ := json.Marshal(tokens)

	t.Setenv("MAGI_API_TOKEN", "")
	t.Setenv("MAGI_MACHINE_TOKENS_JSON", string(data))
	t.Setenv("MAGI_MACHINE_TOKENS_FILE", "")

	r, _ := LoadResolverFromEnv()
	identity, ok := r.ResolveBearer("real-tok")
	if !ok {
		t.Fatal("expected successful resolve")
	}
	if identity.User != "alice" {
		t.Errorf("expected user 'alice', got %q", identity.User)
	}
}

// --- DB-backed credential lookup ---

type mockLookup struct {
	cred    *db.MachineCredential
	err     error
	touched string
}

func (m *mockLookup) GetMachineCredentialByTokenHash(hash string) (*db.MachineCredential, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.cred != nil && m.cred.TokenHash == hash {
		return m.cred, nil
	}
	return nil, nil
}

func (m *mockLookup) TouchMachineCredential(id string) error {
	m.touched = id
	return nil
}

func TestResolveBearer_DBLookup(t *testing.T) {
	token := "db-token-123"
	hash := HashToken(token)

	t.Setenv("MAGI_API_TOKEN", "")
	t.Setenv("MAGI_MACHINE_TOKENS_JSON", "")
	t.Setenv("MAGI_MACHINE_TOKENS_FILE", "")

	r, _ := LoadResolverFromEnv()
	mock := &mockLookup{
		cred: &db.MachineCredential{
			ID:        "cred-1",
			TokenHash: hash,
			User:      "db-user",
			MachineID: "db-machine",
			AgentName: "db-agent",
			AgentType: "claude",
			Groups:    []string{"group1"},
		},
	}
	r.SetMachineLookup(mock)

	identity, ok := r.ResolveBearer(token)
	if !ok {
		t.Fatal("expected DB lookup to succeed")
	}
	if identity.Kind != "machine" {
		t.Errorf("expected kind 'machine', got %q", identity.Kind)
	}
	if identity.User != "db-user" {
		t.Errorf("expected user 'db-user', got %q", identity.User)
	}
	if mock.touched != "cred-1" {
		t.Errorf("expected credential touch on 'cred-1', got %q", mock.touched)
	}
}

func TestResolveBearer_DBLookupNotFound(t *testing.T) {
	t.Setenv("MAGI_API_TOKEN", "")
	t.Setenv("MAGI_MACHINE_TOKENS_JSON", "")
	t.Setenv("MAGI_MACHINE_TOKENS_FILE", "")

	r, _ := LoadResolverFromEnv()
	r.SetMachineLookup(&mockLookup{})

	_, ok := r.ResolveBearer("unknown-token")
	if ok {
		t.Error("expected resolve to fail for unknown DB token")
	}
}

// --- GenerateToken and HashToken ---

func TestGenerateToken(t *testing.T) {
	token, hash, err := GenerateToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Error("token should not be empty")
	}
	if hash == "" {
		t.Error("hash should not be empty")
	}
	if HashToken(token) != hash {
		t.Error("HashToken(token) should equal the returned hash")
	}
}

func TestGenerateToken_Uniqueness(t *testing.T) {
	token1, _, _ := GenerateToken()
	token2, _, _ := GenerateToken()
	if token1 == token2 {
		t.Error("two generated tokens should not be equal")
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	h1 := HashToken("test-token")
	h2 := HashToken("test-token")
	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
}

func TestHashToken_DifferentInputs(t *testing.T) {
	h1 := HashToken("token-a")
	h2 := HashToken("token-b")
	if h1 == h2 {
		t.Error("different inputs should produce different hashes")
	}
}

// --- Helper functions ---

func TestIsAdmin(t *testing.T) {
	if IsAdmin(nil) {
		t.Error("nil identity should not be admin")
	}
	if IsAdmin(&Identity{Kind: "machine"}) {
		t.Error("machine identity should not be admin")
	}
	if !IsAdmin(&Identity{Kind: "admin"}) {
		t.Error("admin identity should be admin")
	}
}

func TestEffectiveUser(t *testing.T) {
	tests := []struct {
		name     string
		identity *Identity
		want     string
	}{
		{"nil", nil, ""},
		{"admin no user", &Identity{Kind: "admin"}, "admin"},
		{"admin with user", &Identity{Kind: "admin", User: "root"}, "root"},
		{"machine with user", &Identity{Kind: "machine", User: "alice"}, "alice"},
		{"machine no user", &Identity{Kind: "machine"}, ""},
		{"whitespace user", &Identity{Kind: "machine", User: "  "}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveUser(tt.identity)
			if got != tt.want {
				t.Errorf("EffectiveUser() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOwnerTag(t *testing.T) {
	if tag := OwnerTag(nil); tag != "" {
		t.Errorf("nil identity should produce empty tag, got %q", tag)
	}
	if tag := OwnerTag(&Identity{Kind: "admin"}); tag != "owner:admin" {
		t.Errorf("expected 'owner:admin', got %q", tag)
	}
	if tag := OwnerTag(&Identity{Kind: "machine", User: "alice"}); tag != "owner:alice" {
		t.Errorf("expected 'owner:alice', got %q", tag)
	}
}

func TestCanModifyTags(t *testing.T) {
	admin := &Identity{Kind: "admin"}
	alice := &Identity{Kind: "machine", User: "alice"}
	bob := &Identity{Kind: "machine", User: "bob"}
	noUser := &Identity{Kind: "machine"}

	tags := []string{"owner:alice", "topic:test"}

	if !CanModifyTags(admin, tags) {
		t.Error("admin should be able to modify any tags")
	}
	if !CanModifyTags(alice, tags) {
		t.Error("alice should be able to modify tags with owner:alice")
	}
	if CanModifyTags(bob, tags) {
		t.Error("bob should not be able to modify alice's tags")
	}
	if CanModifyTags(noUser, tags) {
		t.Error("identity with no user should not modify tags")
	}
	if CanModifyTags(nil, tags) {
		t.Error("nil identity should not modify tags")
	}
}

// --- Context ---

func TestContext_RoundTrip(t *testing.T) {
	identity := &Identity{Kind: "admin", User: "test"}
	ctx := NewContext(context.Background(), identity)

	got, ok := FromContext(ctx)
	if !ok {
		t.Fatal("expected identity from context")
	}
	if got.User != "test" {
		t.Errorf("expected user 'test', got %q", got.User)
	}
}

func TestContext_NilIdentity(t *testing.T) {
	ctx := NewContext(context.Background(), nil)
	_, ok := FromContext(ctx)
	if ok {
		t.Error("expected no identity when nil was stored")
	}
}

func TestContext_NilContext(t *testing.T) {
	_, ok := FromContext(nil)
	if ok {
		t.Error("expected no identity from nil context")
	}
}

func TestContext_EmptyContext(t *testing.T) {
	_, ok := FromContext(context.Background())
	if ok {
		t.Error("expected no identity from empty context")
	}
}

// --- Enabled ---

func TestEnabled_NilResolver(t *testing.T) {
	var r *Resolver
	if r.Enabled() {
		t.Error("nil resolver should not be enabled")
	}
}

func TestSetMachineLookup_NilResolver(t *testing.T) {
	var r *Resolver
	r.SetMachineLookup(&mockLookup{}) // should not panic
}

func TestAdminToken_NilResolver(t *testing.T) {
	var r *Resolver
	if r.AdminToken() != "" {
		t.Error("nil resolver should return empty admin token")
	}
}

// --- dedupeStrings ---

func TestDedupeStrings(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  int
	}{
		{"nil", nil, 0},
		{"empty", []string{}, 0},
		{"no dupes", []string{"a", "b", "c"}, 3},
		{"with dupes", []string{"a", "b", "a", "c", "b"}, 3},
		{"whitespace", []string{"a", " a ", "  a  "}, 1},
		{"empty strings", []string{"", "a", "", "b"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupeStrings(tt.input)
			if len(got) != tt.want {
				t.Errorf("dedupeStrings() returned %d items, want %d", len(got), tt.want)
			}
		})
	}
}

// --- ApplyToFilter ---

func TestApplyToFilter_Admin(t *testing.T) {
	identity := &Identity{Kind: "admin"}
	ctx := NewContext(context.Background(), identity)
	filter := &db.MemoryFilter{}
	ApplyToFilter(ctx, filter)
	if filter.EnforceAccess {
		t.Error("admin should not enforce access control")
	}
}

func TestApplyToFilter_Machine(t *testing.T) {
	identity := &Identity{Kind: "machine", User: "alice", Groups: []string{"dev"}}
	ctx := NewContext(context.Background(), identity)
	filter := &db.MemoryFilter{}
	ApplyToFilter(ctx, filter)
	if !filter.EnforceAccess {
		t.Error("machine identity should enforce access control")
	}
	if filter.RequestUser != "alice" {
		t.Errorf("expected request user 'alice', got %q", filter.RequestUser)
	}
	if len(filter.RequestGroups) != 1 || filter.RequestGroups[0] != "dev" {
		t.Errorf("unexpected groups: %v", filter.RequestGroups)
	}
}

func TestApplyToFilter_NoIdentity(t *testing.T) {
	filter := &db.MemoryFilter{}
	ApplyToFilter(context.Background(), filter)
	if filter.EnforceAccess {
		t.Error("no identity should not enforce access control")
	}
}

func TestApplyToFilter_NilFilter(t *testing.T) {
	identity := &Identity{Kind: "machine", User: "alice"}
	ctx := NewContext(context.Background(), identity)
	ApplyToFilter(ctx, nil) // should not panic
}
