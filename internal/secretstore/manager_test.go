package secretstore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeBackend struct {
	name string
	data map[string]map[string]string
}

func (f *fakeBackend) Name() string { return f.name }

func (f *fakeBackend) Put(_ context.Context, path string, data map[string]string) error {
	if f.data == nil {
		f.data = map[string]map[string]string{}
	}
	copied := make(map[string]string, len(data))
	for k, v := range data {
		copied[k] = v
	}
	f.data[path] = copied
	return nil
}

func (f *fakeBackend) Get(_ context.Context, path string) (map[string]string, error) {
	return f.data[path], nil
}

func TestManagerExternalizeRedactsAndStores(t *testing.T) {
	backend := &fakeBackend{name: "vault"}
	manager := &manager{backend: backend}

	result, err := manager.Externalize(context.Background(), "Project X", "api_key=abc123\npassword=hunter2")
	if err != nil {
		t.Fatalf("Externalize: %v", err)
	}
	if len(result.Refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(result.Refs))
	}
	if strings.Contains(result.RedactedContent, "abc123") || strings.Contains(result.RedactedContent, "hunter2") {
		t.Fatalf("expected redacted content, got %q", result.RedactedContent)
	}
	if !strings.Contains(result.RedactedContent, "[stored:vault://") {
		t.Fatalf("expected stored refs in redacted content, got %q", result.RedactedContent)
	}

	if len(backend.data) != 1 {
		t.Fatalf("expected 1 stored secret path, got %d", len(backend.data))
	}
	for path, data := range backend.data {
		if !strings.HasPrefix(path, "magi/projectx/") {
			t.Fatalf("unexpected secret path %q", path)
		}
		if data["api_key"] != "abc123" {
			t.Fatalf("api_key = %q want abc123", data["api_key"])
		}
		if data["password"] != "hunter2" {
			t.Fatalf("password = %q want hunter2", data["password"])
		}
	}
}

func TestVaultBackendPutAndGet(t *testing.T) {
	stored := map[string]map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Vault-Token"); got != "token-123" {
			t.Fatalf("X-Vault-Token=%q want token-123", got)
		}
		if got := r.Header.Get("X-Vault-Namespace"); got != "team-a" {
			t.Fatalf("X-Vault-Namespace=%q want team-a", got)
		}

		path := strings.TrimPrefix(r.URL.Path, "/v1/secret/data/")
		switch r.Method {
		case http.MethodPost:
			var payload struct {
				Data map[string]string `json:"data"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode POST body: %v", err)
			}
			stored[path] = payload.Data
			w.WriteHeader(http.StatusNoContent)
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"data": stored[path],
				},
			})
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	backend := newVaultBackend(Config{
		VaultAddr:      server.URL,
		VaultToken:     "token-123",
		VaultMount:     "secret",
		VaultNamespace: "team-a",
	})

	if err := backend.Put(context.Background(), "magi/proj/entry", map[string]string{"api_key": "abc123"}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := backend.Get(context.Background(), "magi/proj/entry")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got["api_key"] != "abc123" {
		t.Fatalf("api_key = %q want abc123", got["api_key"])
	}
}
