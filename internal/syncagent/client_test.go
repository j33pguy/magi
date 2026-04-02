package syncagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientRememberUsesSyncEndpoint(t *testing.T) {
	var gotPath string
	var gotAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(ServerConfig{URL: server.URL, Token: "machine-secret"})
	err := client.Remember(context.Background(), Payload{
		Content: "hello",
		Project: "proj",
		Type:    "memory",
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if gotPath != "/sync/memories" {
		t.Fatalf("path = %q want /sync/memories", gotPath)
	}
	if gotAuth != "Bearer machine-secret" {
		t.Fatalf("auth = %q want Bearer machine-secret", gotAuth)
	}
}

func TestClientRememberFallsBackToLegacyEndpoint(t *testing.T) {
	var paths []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/sync/memories" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path == "/remember" {
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(ServerConfig{URL: server.URL, Token: "machine-secret"})
	err := client.Remember(context.Background(), Payload{
		Content: "hello",
		Project: "proj",
		Type:    "memory",
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	got := strings.Join(paths, ",")
	if got != "/sync/memories,/remember" {
		t.Fatalf("paths = %q want %q", got, "/sync/memories,/remember")
	}
}
