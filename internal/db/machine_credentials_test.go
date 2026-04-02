package db

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestMachineCredentialLifecycleSQLite(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client, err := NewSQLiteClient(filepath.Join(t.TempDir(), "machines.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	defer client.Close()

	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cred, err := client.TursoClient.CreateMachineCredential(&MachineCredential{
		TokenHash: "hash-123",
		User:      "UserA",
		MachineID: "MachineA",
		AgentName: "claude-main",
		AgentType: "claude",
		Groups:    []string{"platform", "ops"},
	})
	if err != nil {
		t.Fatalf("CreateMachineCredential: %v", err)
	}
	if cred.ID == "" {
		t.Fatal("expected credential ID")
	}

	got, err := client.TursoClient.GetMachineCredentialByTokenHash("hash-123")
	if err != nil {
		t.Fatalf("GetMachineCredentialByTokenHash: %v", err)
	}
	if got == nil || got.User != "UserA" || got.MachineID != "MachineA" {
		t.Fatalf("unexpected credential: %+v", got)
	}

	if err := client.TursoClient.TouchMachineCredential(cred.ID); err != nil {
		t.Fatalf("TouchMachineCredential: %v", err)
	}

	list, err := client.TursoClient.ListMachineCredentials()
	if err != nil {
		t.Fatalf("ListMachineCredentials: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d credentials, want 1", len(list))
	}
	if list[0].LastSeenAt == "" {
		t.Fatal("expected last_seen_at to be populated after touch")
	}

	if err := client.TursoClient.RevokeMachineCredential(cred.ID); err != nil {
		t.Fatalf("RevokeMachineCredential: %v", err)
	}

	got, err = client.TursoClient.GetMachineCredentialByTokenHash("hash-123")
	if err != nil {
		t.Fatalf("GetMachineCredentialByTokenHash after revoke: %v", err)
	}
	if got != nil {
		t.Fatalf("expected revoked credential to be filtered out, got %+v", got)
	}
}
