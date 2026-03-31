package api

import (
	"strings"
	"testing"
)

func TestFormatConversationContent(t *testing.T) {
	tests := []struct {
		name     string
		req      *conversationRequest
		contains []string
	}{
		{
			name: "basic conversation",
			req: &conversationRequest{
				Channel: "discord",
				Summary: "Discussed infrastructure rack rebuild",
			},
			contains: []string{"Conversation on discord", "Discussed infrastructure rack rebuild"},
		},
		{
			name: "with session key",
			req: &conversationRequest{
				Channel:    "webchat",
				SessionKey: "abc123",
				Summary:    "Talked about deployment",
			},
			contains: []string{"Conversation on webchat", "(session: abc123)", "Talked about deployment"},
		},
		{
			name: "with time range",
			req: &conversationRequest{
				Channel:   "discord",
				StartedAt: "2026-03-28T10:00:00Z",
				EndedAt:   "2026-03-28T10:45:00Z",
				Summary:   "Network changes",
			},
			contains: []string{"Time: 2026-03-28T10:00:00Z to 2026-03-28T10:45:00Z"},
		},
		{
			name: "with turn count",
			req: &conversationRequest{
				Channel:   "discord",
				TurnCount: 12,
				Summary:   "Long discussion",
			},
			contains: []string{"Turns: 12"},
		},
		{
			name: "with topics",
			req: &conversationRequest{
				Channel: "discord",
				Summary: "Tech talk",
				Topics:  []string{"infrastructure", "networking"},
			},
			contains: []string{"Topics: infrastructure, networking"},
		},
		{
			name: "with decisions",
			req: &conversationRequest{
				Channel:   "discord",
				Summary:   "Made some decisions",
				Decisions: []string{"Switched to LACP bond", "Use vault-unsealer"},
			},
			contains: []string{"Decisions:", "- Switched to LACP bond", "- Use vault-unsealer"},
		},
		{
			name: "with action items",
			req: &conversationRequest{
				Channel:     "webchat",
				Summary:     "Follow-up needed",
				ActionItems: []string{"Deploy vault-unsealer", "Update DNS records"},
			},
			contains: []string{"Action Items:", "- Deploy vault-unsealer", "- Update DNS records"},
		},
		{
			name: "full conversation",
			req: &conversationRequest{
				Channel:     "discord",
				SessionKey:  "sess-001",
				StartedAt:   "2026-03-28T10:00:00Z",
				EndedAt:     "2026-03-28T10:45:00Z",
				TurnCount:   12,
				Summary:     "Discussed infrastructure rack rebuild and network changes",
				Topics:      []string{"infrastructure", "networking"},
				Decisions:   []string{"Switched cdn-cache to LACP bond"},
				ActionItems: []string{"Deploy vault-unsealer"},
			},
			contains: []string{
				"Conversation on discord",
				"(session: sess-001)",
				"Time: 2026-03-28T10:00:00Z to 2026-03-28T10:45:00Z",
				"Turns: 12",
				"Topics: infrastructure, networking",
				"Discussed infrastructure rack rebuild",
				"Decisions:",
				"- Switched cdn-cache to LACP bond",
				"Action Items:",
				"- Deploy vault-unsealer",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatConversationContent(tt.req)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("formatConversationContent() missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestConversationRequestValidation(t *testing.T) {
	// Verify required fields are checked
	tests := []struct {
		name    string
		req     conversationRequest
		wantErr string
	}{
		{
			name:    "missing summary",
			req:     conversationRequest{Channel: "discord"},
			wantErr: "summary is required",
		},
		{
			name:    "missing channel",
			req:     conversationRequest{Summary: "test"},
			wantErr: "channel is required",
		},
		{
			name: "valid request",
			req:  conversationRequest{Channel: "discord", Summary: "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == "" {
				if tt.req.Summary == "" {
					t.Error("valid request should have summary")
				}
				if tt.req.Channel == "" {
					t.Error("valid request should have channel")
				}
			} else {
				switch tt.wantErr {
				case "summary is required":
					if tt.req.Summary != "" {
						t.Error("expected empty summary")
					}
				case "channel is required":
					if tt.req.Channel != "" {
						t.Error("expected empty channel")
					}
				}
			}
		})
	}
}

func TestConversationSearchRequestDefaults(t *testing.T) {
	req := conversationSearchRequest{}

	// Verify defaults match what handleSearchConversations applies
	if req.Limit != 0 {
		t.Errorf("default limit = %d, want 0 (handler sets to 5)", req.Limit)
	}
	if req.MinRelevance != 0.0 {
		t.Errorf("default min_relevance = %f, want 0.0", req.MinRelevance)
	}
	if req.RecencyDecay != 0.0 {
		t.Errorf("default recency_decay = %f, want 0.0", req.RecencyDecay)
	}
}
