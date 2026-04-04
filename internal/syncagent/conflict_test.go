package syncagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDetectConflict(t *testing.T) {
	tests := []struct {
		name         string
		localHash    string
		lastSyncHash string
		remoteHash   string
		want         bool
	}{
		{
			name:         "no changes",
			localHash:    "abc",
			lastSyncHash: "abc",
			remoteHash:   "abc",
			want:         false,
		},
		{
			name:         "only local changed",
			localHash:    "def",
			lastSyncHash: "abc",
			remoteHash:   "abc",
			want:         false,
		},
		{
			name:         "only remote changed",
			localHash:    "abc",
			lastSyncHash: "abc",
			remoteHash:   "xyz",
			want:         false,
		},
		{
			name:         "both changed - conflict",
			localHash:    "def",
			lastSyncHash: "abc",
			remoteHash:   "xyz",
			want:         true,
		},
		{
			name:         "both changed to same value",
			localHash:    "xyz",
			lastSyncHash: "abc",
			remoteHash:   "xyz",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remote := RemoteState{SHA256: tt.remoteHash}
			got := DetectConflict(tt.localHash, tt.lastSyncHash, remote)
			if got != tt.want {
				t.Fatalf("DetectConflict(%q, %q, %q) = %v, want %v",
					tt.localHash, tt.lastSyncHash, tt.remoteHash, got, tt.want)
			}
		})
	}
}

func TestResolveConflictLastWriteWins(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Hour)

	tests := []struct {
		name         string
		strategy     ConflictStrategy
		localModTime time.Time
		remoteTime   time.Time
		want         ConflictResult
	}{
		{
			name:         "local newer wins",
			strategy:     ConflictLastWriteWins,
			localModTime: now,
			remoteTime:   earlier,
			want:         KeepLocal,
		},
		{
			name:         "remote newer wins",
			strategy:     ConflictLastWriteWins,
			localModTime: earlier,
			remoteTime:   now,
			want:         KeepRemote,
		},
		{
			name:         "newest strategy local newer",
			strategy:     ConflictNewest,
			localModTime: now,
			remoteTime:   earlier,
			want:         KeepLocal,
		},
		{
			name:         "newest strategy remote newer",
			strategy:     ConflictNewest,
			localModTime: earlier,
			remoteTime:   now,
			want:         KeepRemote,
		},
		{
			name:         "manual keeps both",
			strategy:     ConflictManual,
			localModTime: now,
			remoteTime:   earlier,
			want:         KeepBoth,
		},
		{
			name:         "same time keeps remote",
			strategy:     ConflictLastWriteWins,
			localModTime: now,
			remoteTime:   now,
			want:         KeepRemote,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remote := RemoteState{UpdatedAt: tt.remoteTime}
			got := ResolveConflict(tt.strategy, tt.localModTime, remote)
			if got != tt.want {
				t.Fatalf("ResolveConflict(%q) = %v, want %v", tt.strategy, got, tt.want)
			}
		})
	}
}

func TestConflictStrategyValidation(t *testing.T) {
	cfg := &Config{
		Server:  ServerConfig{URL: "http://example.com"},
		Sync:    SyncConfig{Mode: "push", ConflictStrategy: "invalid"},
		Privacy: PrivacyConfig{Mode: "allowlist"},
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected validate to reject invalid conflict_strategy")
	}
}

func TestConflictStrategyDefault(t *testing.T) {
	cfg := &Config{
		Server:  ServerConfig{URL: "http://example.com"},
		Privacy: PrivacyConfig{Mode: "allowlist"},
	}
	if err := cfg.setDefaults(); err != nil {
		t.Fatalf("setDefaults: %v", err)
	}
	if cfg.Sync.ConflictStrategy != ConflictLastWriteWins {
		t.Fatalf("default conflict_strategy = %q, want %q", cfg.Sync.ConflictStrategy, ConflictLastWriteWins)
	}
}

func TestSettingsAdapterScan(t *testing.T) {
	dir := t.TempDir()
	settingsFile := filepath.Join(dir, "settings.yaml")

	content := "preferences:\n  default_project: magi\n  ui_theme: dark\n"
	if err := os.WriteFile(settingsFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Machine: MachineConfig{User: "tester", ID: "machine1"},
	}
	agent := AgentConfig{
		Name:         "settings",
		Type:         "settings",
		Enabled:      true,
		SettingsPath: settingsFile,
		Visibility:   "internal",
		Owner:        "tester",
	}

	payloads, err := (settingsAdapter{}).Scan(cfg, agent)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(payloads) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(payloads))
	}

	p := payloads[0]
	if p.Type != "preference" {
		t.Fatalf("type = %q, want preference", p.Type)
	}
	if p.Speaker != "system" {
		t.Fatalf("speaker = %q, want system", p.Speaker)
	}

	// Verify content is valid JSON with preferences.
	var prefs map[string]any
	if err := json.Unmarshal([]byte(p.Content), &prefs); err != nil {
		t.Fatalf("content is not valid JSON: %v", err)
	}
	if prefs["default_project"] != "magi" {
		t.Fatalf("expected default_project=magi, got %v", prefs["default_project"])
	}

	// Check tags.
	wantTags := []string{"settings", "machine:machine1", "type:preference"}
	for _, w := range wantTags {
		found := false
		for _, tag := range p.Tags {
			if tag == w {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected tag %q in %v", w, p.Tags)
		}
	}
}

func TestSettingsAdapterScanMissingFile(t *testing.T) {
	cfg := &Config{Machine: MachineConfig{User: "tester", ID: "machine1"}}
	agent := AgentConfig{
		Type:         "settings",
		Enabled:      true,
		SettingsPath: "/nonexistent/settings.yaml",
		Visibility:   "internal",
	}

	payloads, err := (settingsAdapter{}).Scan(cfg, agent)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if len(payloads) != 0 {
		t.Fatalf("expected 0 payloads for missing file, got %d", len(payloads))
	}
}

func TestMergeSettings(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "settings.yaml")

	// Write local settings.
	local := SettingsFile{
		Preferences: map[string]any{
			"ui_theme":        "dark",
			"default_project": "magi",
		},
	}
	data, _ := yaml.Marshal(&local)
	if err := os.WriteFile(localPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Set local mod time to the past.
	pastTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(localPath, pastTime, pastTime); err != nil {
		t.Fatal(err)
	}

	// Remote has newer settings with an updated key and a new key.
	remotePrefs := map[string]any{
		"ui_theme":      "light",
		"notifications": true,
	}
	remoteTime := time.Now()

	if err := MergeSettings(localPath, remotePrefs, remoteTime); err != nil {
		t.Fatalf("MergeSettings: %v", err)
	}

	// Read back merged file.
	merged, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatal(err)
	}
	var result SettingsFile
	if err := yaml.Unmarshal(merged, &result); err != nil {
		t.Fatal(err)
	}

	// ui_theme should be overwritten by remote (remote is newer).
	if result.Preferences["ui_theme"] != "light" {
		t.Fatalf("ui_theme = %v, want light", result.Preferences["ui_theme"])
	}
	// default_project should be kept (only in local).
	if result.Preferences["default_project"] != "magi" {
		t.Fatalf("default_project = %v, want magi", result.Preferences["default_project"])
	}
	// notifications should be added from remote.
	if result.Preferences["notifications"] != true {
		t.Fatalf("notifications = %v, want true", result.Preferences["notifications"])
	}
}

func TestMergeSettingsNewFile(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "settings.yaml")

	remotePrefs := map[string]any{
		"ui_theme": "dark",
	}

	if err := MergeSettings(localPath, remotePrefs, time.Now()); err != nil {
		t.Fatalf("MergeSettings: %v", err)
	}

	data, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatal(err)
	}
	var result SettingsFile
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.Preferences["ui_theme"] != "dark" {
		t.Fatalf("ui_theme = %v, want dark", result.Preferences["ui_theme"])
	}
}

func TestSettingsAgentValidation(t *testing.T) {
	cfg := &Config{
		Server:  ServerConfig{URL: "http://example.com"},
		Sync:    SyncConfig{Mode: "push", ConflictStrategy: ConflictLastWriteWins},
		Privacy: PrivacyConfig{Mode: "allowlist"},
		Agents: []AgentConfig{
			{
				Type:       "settings",
				Enabled:    true,
				Paths:      []string{"/tmp"},
				Visibility: "internal",
				// Missing SettingsPath — should fail.
			},
		},
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected validate to reject settings agent without settings_path")
	}
}

func TestSettingsCollectPayloads(t *testing.T) {
	dir := t.TempDir()
	settingsFile := filepath.Join(dir, "settings.yaml")
	stateFile := filepath.Join(t.TempDir(), "state.json")

	content := "preferences:\n  default_project: magi\n"
	if err := os.WriteFile(settingsFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Server:  ServerConfig{URL: "http://localhost", Protocol: "http"},
		Machine: MachineConfig{ID: "test", User: "tester"},
		Sync:    SyncConfig{Mode: "push", StateFile: stateFile, ConflictStrategy: ConflictLastWriteWins},
		Privacy: PrivacyConfig{Mode: "allowlist"},
		Agents: []AgentConfig{
			{
				Name:         "settings",
				Type:         "settings",
				Enabled:      true,
				Paths:        []string{dir},
				SettingsPath: settingsFile,
				Visibility:   "internal",
				Owner:        "tester",
			},
		},
	}

	app, err := New(cfg, filepath.Join(t.TempDir(), "config.yaml"), NewLogger())
	if err != nil {
		t.Fatal(err)
	}

	payloads, err := app.collectPayloads()
	if err != nil {
		t.Fatalf("collectPayloads: %v", err)
	}
	if len(payloads) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(payloads))
	}
	if payloads[0].Type != "preference" {
		t.Fatalf("type = %q, want preference", payloads[0].Type)
	}
}
