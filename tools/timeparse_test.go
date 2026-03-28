package tools

import (
	"testing"
	"time"
)

func TestParseTimeParam(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name    string
		input   string
		wantNil bool
		wantErr bool
		check   func(t *testing.T, got *time.Time)
	}{
		{
			name:    "empty string",
			input:   "",
			wantNil: true,
		},
		{
			name: "7d relative",
			input: "7d",
			check: func(t *testing.T, got *time.Time) {
				expected := now.AddDate(0, 0, -7)
				diff := got.Sub(expected).Abs()
				if diff > 2*time.Second {
					t.Errorf("7d: got %v, want ~%v (diff %v)", got, expected, diff)
				}
			},
		},
		{
			name: "2w relative",
			input: "2w",
			check: func(t *testing.T, got *time.Time) {
				expected := now.AddDate(0, 0, -14)
				diff := got.Sub(expected).Abs()
				if diff > 2*time.Second {
					t.Errorf("2w: got %v, want ~%v (diff %v)", got, expected, diff)
				}
			},
		},
		{
			name: "1m relative",
			input: "1m",
			check: func(t *testing.T, got *time.Time) {
				expected := now.AddDate(0, -1, 0)
				diff := got.Sub(expected).Abs()
				if diff > 2*time.Second {
					t.Errorf("1m: got %v, want ~%v (diff %v)", got, expected, diff)
				}
			},
		},
		{
			name: "1y relative",
			input: "1y",
			check: func(t *testing.T, got *time.Time) {
				expected := now.AddDate(-1, 0, 0)
				diff := got.Sub(expected).Abs()
				if diff > 2*time.Second {
					t.Errorf("1y: got %v, want ~%v (diff %v)", got, expected, diff)
				}
			},
		},
		{
			name: "date only",
			input: "2026-03-01",
			check: func(t *testing.T, got *time.Time) {
				expected := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
				if !got.Equal(expected) {
					t.Errorf("date: got %v, want %v", got, expected)
				}
			},
		},
		{
			name: "RFC3339",
			input: "2026-03-01T00:00:00Z",
			check: func(t *testing.T, got *time.Time) {
				expected := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
				if !got.Equal(expected) {
					t.Errorf("RFC3339: got %v, want %v", got, expected)
				}
			},
		},
		{
			name:    "invalid",
			input:   "not-a-date",
			wantErr: true,
		},
		{
			name:    "invalid unit",
			input:   "7x",
			wantErr: true,
		},
		{
			name: "30d relative",
			input: "30d",
			check: func(t *testing.T, got *time.Time) {
				expected := now.AddDate(0, 0, -30)
				diff := got.Sub(expected).Abs()
				if diff > 2*time.Second {
					t.Errorf("30d: got %v, want ~%v (diff %v)", got, expected, diff)
				}
			},
		},
		{
			name: "uppercase 7D",
			input: "7D",
			check: func(t *testing.T, got *time.Time) {
				expected := now.AddDate(0, 0, -7)
				diff := got.Sub(expected).Abs()
				if diff > 2*time.Second {
					t.Errorf("7D: got %v, want ~%v (diff %v)", got, expected, diff)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTimeParam(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %q, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil for %q, got %v", tt.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected non-nil for %q", tt.input)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}
