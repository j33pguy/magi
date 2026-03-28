package db

import (
	"testing"
	"time"
)

func TestAppendTaxonomyConditions(t *testing.T) {
	tests := []struct {
		name           string
		filter         *MemoryFilter
		wantConditions int
		wantArgs       int
	}{
		{
			name:           "no taxonomy filters",
			filter:         &MemoryFilter{},
			wantConditions: 0,
			wantArgs:       0,
		},
		{
			name:           "speaker only",
			filter:         &MemoryFilter{Speaker: "j33p"},
			wantConditions: 1,
			wantArgs:       1,
		},
		{
			name:           "area only",
			filter:         &MemoryFilter{Area: "work"},
			wantConditions: 1,
			wantArgs:       1,
		},
		{
			name:           "sub_area only",
			filter:         &MemoryFilter{SubArea: "proxmox"},
			wantConditions: 1,
			wantArgs:       1,
		},
		{
			name:           "all taxonomy fields",
			filter:         &MemoryFilter{Speaker: "gilfoyle", Area: "homelab", SubArea: "proxmox"},
			wantConditions: 3,
			wantArgs:       3,
		},
		{
			name:           "taxonomy with other filters",
			filter:         &MemoryFilter{Project: "iac", Type: "memory", Speaker: "j33p", Area: "work"},
			wantConditions: 2, // appendTaxonomyConditions only adds speaker + area
			wantArgs:       2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var conditions []string
			var args []any

			appendTaxonomyConditions(tt.filter, &conditions, &args)

			if len(conditions) != tt.wantConditions {
				t.Errorf("got %d conditions, want %d: %v", len(conditions), tt.wantConditions, conditions)
			}
			if len(args) != tt.wantArgs {
				t.Errorf("got %d args, want %d: %v", len(args), tt.wantArgs, args)
			}
		})
	}
}

func TestAppendTaxonomyConditions_Values(t *testing.T) {
	var conditions []string
	var args []any

	filter := &MemoryFilter{Speaker: "j33p", Area: "homelab", SubArea: "proxmox"}
	appendTaxonomyConditions(filter, &conditions, &args)

	// Verify correct SQL conditions
	expected := []string{"m.speaker = ?", "m.area = ?", "m.sub_area = ?"}
	for i, want := range expected {
		if conditions[i] != want {
			t.Errorf("condition[%d] = %q, want %q", i, conditions[i], want)
		}
	}

	// Verify correct args
	expectedArgs := []any{"j33p", "homelab", "proxmox"}
	for i, want := range expectedArgs {
		if args[i] != want {
			t.Errorf("args[%d] = %v, want %v", i, args[i], want)
		}
	}
}

func TestMemoryStructTaxonomyFields(t *testing.T) {
	m := &Memory{
		Speaker: "j33p",
		Area:    "work",
		SubArea: "power-platform",
	}

	if m.Speaker != "j33p" {
		t.Errorf("Speaker = %q, want %q", m.Speaker, "j33p")
	}
	if m.Area != "work" {
		t.Errorf("Area = %q, want %q", m.Area, "work")
	}
	if m.SubArea != "power-platform" {
		t.Errorf("SubArea = %q, want %q", m.SubArea, "power-platform")
	}
}

func TestAppendTimeConditions(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-7 * 24 * time.Hour)

	tests := []struct {
		name           string
		filter         *MemoryFilter
		wantConditions int
		wantArgs       int
	}{
		{
			name:           "no time filters",
			filter:         &MemoryFilter{},
			wantConditions: 0,
			wantArgs:       0,
		},
		{
			name:           "after only",
			filter:         &MemoryFilter{AfterTime: &past},
			wantConditions: 1,
			wantArgs:       1,
		},
		{
			name:           "before only",
			filter:         &MemoryFilter{BeforeTime: &now},
			wantConditions: 1,
			wantArgs:       1,
		},
		{
			name:           "both after and before",
			filter:         &MemoryFilter{AfterTime: &past, BeforeTime: &now},
			wantConditions: 2,
			wantArgs:       2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var conditions []string
			var args []any

			appendTimeConditions(tt.filter, &conditions, &args)

			if len(conditions) != tt.wantConditions {
				t.Errorf("got %d conditions, want %d: %v", len(conditions), tt.wantConditions, conditions)
			}
			if len(args) != tt.wantArgs {
				t.Errorf("got %d args, want %d: %v", len(args), tt.wantArgs, args)
			}
		})
	}
}

func TestAppendTimeConditions_Values(t *testing.T) {
	after := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	var conditions []string
	var args []any

	filter := &MemoryFilter{AfterTime: &after, BeforeTime: &before}
	appendTimeConditions(filter, &conditions, &args)

	expectedConds := []string{"m.created_at > ?", "m.created_at < ?"}
	for i, want := range expectedConds {
		if conditions[i] != want {
			t.Errorf("condition[%d] = %q, want %q", i, conditions[i], want)
		}
	}

	expectedArgs := []string{"2026-01-01T00:00:00Z", "2026-03-01T00:00:00Z"}
	for i, want := range expectedArgs {
		if args[i] != want {
			t.Errorf("args[%d] = %v, want %v", i, args[i], want)
		}
	}
}
