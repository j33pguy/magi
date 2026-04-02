package grpc

import (
	"context"
	"strings"
	"testing"

	"github.com/j33pguy/magi/internal/secretstore"
	pb "github.com/j33pguy/magi/proto/memory/v1"
)

type grpcSecretManager struct{}

func (g *grpcSecretManager) BackendName() string { return "vault" }

func (g *grpcSecretManager) Externalize(_ context.Context, _ string, content string) (*secretstore.ExternalizeResult, error) {
	return &secretstore.ExternalizeResult{
		RedactedContent: strings.ReplaceAll(content, "abc123", "[stored:vault://magi/test#api_key]"),
		Refs: []secretstore.Reference{
			{Backend: "vault", Path: "magi/test", Key: "api_key"},
		},
	}, nil
}

func (g *grpcSecretManager) Resolve(_ context.Context, path, key string) (string, error) {
	return path + "#" + key, nil
}

func TestRememberExternalizesSecretsWithManager(t *testing.T) {
	srv, _ := newTestGRPCServer(t)
	srv.SetSecretManager(&grpcSecretManager{})

	resp, err := srv.Remember(context.Background(), &pb.RememberRequest{
		Content: "api_key=abc123",
		Project: "grpc-secret-proj",
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if !resp.Ok || resp.Id == "" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	mem, err := srv.db.GetMemory(resp.Id)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if strings.Contains(mem.Content, "abc123") {
		t.Fatalf("expected secret to be redacted, got %q", mem.Content)
	}
	if !strings.Contains(mem.Content, "[stored:vault://magi/test#api_key]") {
		t.Fatalf("expected stored ref, got %q", mem.Content)
	}

	tags, err := srv.db.GetTags(resp.Id)
	if err != nil {
		t.Fatalf("GetTags: %v", err)
	}
	tagSet := map[string]bool{}
	for _, tag := range tags {
		tagSet[tag] = true
	}
	if !tagSet["secret_backend:vault"] || !tagSet["secret_ref:magi/test#api_key"] {
		t.Fatalf("expected secret tags, got %v", tags)
	}
}
