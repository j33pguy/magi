package db

import (
	"testing"
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
