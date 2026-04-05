package classify

import "testing"

func TestInferType(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"procedure how-to", "How to unseal Vault after a reboot", "procedure"},
		{"procedure steps", "Step 1: SSH into the server. Step 2: Run the script.", "procedure"},
		{"procedure runbook", "Runbook for Traefik certificate renewal", "procedure"},
		{"decision made", "Decided to use gRPC as default transport for all services", "decision"},
		{"decision rationale", "Going with SQLite over Postgres — rationale: single-node, no overhead", "decision"},
		{"incident outage", "Vault outage caused by sealed nodes after power failure", "incident"},
		{"incident postmortem", "Root cause: missing firewall zone ID on Infra network", "incident"},
		{"task queued", "[QUEUED] Fix privacy filter for magi-sync", "task"},
		{"task running", "[RUNNING] Deploy Traefik config update", "task"},
		{"task action item", "Action item: update Ansible inventory for new host", "task"},
		{"conversation summary", "Session summary: discussed VLAN migration and Vault unsealing", "conversation"},
		{"no match", "The weather is nice today", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferType(tt.content)
			if got != tt.want {
				t.Errorf("InferType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInfer_IncludesType(t *testing.T) {
	// Infer should populate Type field too
	c := Infer("How to deploy Traefik with Let's Encrypt certificates")
	if c.Type != "procedure" {
		t.Errorf("Type = %q, want procedure", c.Type)
	}
	if c.Area != "infrastructure" {
		t.Errorf("Area = %q, want infrastructure", c.Area)
	}
}

func TestInfer(t *testing.T) {
	tests := []struct {
		name    string
		content string
		area    string
		subArea string
	}{
		{
			name:    "reverse proxy networking",
			content: "Set up a reverse proxy with Let's Encrypt certificates",
			area:    "infrastructure",
			subArea: "networking",
		},
		{
			name:    "power platform work",
			content: "Power Platform solution architect meeting",
			area:    "work",
			subArea: "power-platform",
		},
		{
			name:    "compute virtualization",
			content: "Hypervisor node running LXC containers for all services",
			area:    "infrastructure",
			subArea: "compute",
		},
		{
			name:    "fabric workspace",
			content: "Created a new Microsoft Fabric workspace for the analytics team",
			area:    "work",
			subArea: "fabric",
		},
		{
			name:    "dns resolver",
			content: "Updated DNS blocklist and restarted the resolver",
			area:    "infrastructure",
			subArea: "dns",
		},
		{
			name:    "magi project",
			content: "Refactored magi MCP server to use gRPC transport",
			area:    "project",
			subArea: "magi",
		},
		{
			name:    "monitoring dashboards",
			content: "Set up Grafana dashboards with Prometheus metrics for infrastructure",
			area:    "infrastructure",
			subArea: "monitoring",
		},
		{
			name:    "sharepoint work",
			content: "Migrated team site from SharePoint on-prem to SPO",
			area:    "work",
			subArea: "sharepoint",
		},
		{
			name:    "ansible iac",
			content: "Wrote Ansible playbook to configure all containers",
			area:    "infrastructure",
			subArea: "iac",
		},
		{
			name:    "gaming personal",
			content: "Playing a game on the dedicated game server",
			area:    "personal",
			subArea: "gaming",
		},
		{
			name:    "sso security",
			content: "Configured OIDC provider for SSO across all services",
			area:    "infrastructure",
			subArea: "security",
		},
		{
			name:    "ci-cd pipeline",
			content: "Set up GitHub Actions CI/CD pipeline for the project",
			area:    "infrastructure",
			subArea: "ci-cd",
		},
		{
			name:    "storage infrastructure",
			content: "Configured NFS shares for backup storage",
			area:    "infrastructure",
			subArea: "storage",
		},
		{
			name:    "no match returns empty",
			content: "Just a random thought about nothing in particular",
			area:    "",
			subArea: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Infer(tt.content)
			if got.Area != tt.area {
				t.Errorf("area: got %q, want %q", got.Area, tt.area)
			}
			if got.SubArea != tt.subArea {
				t.Errorf("sub_area: got %q, want %q", got.SubArea, tt.subArea)
			}
			if got.Speaker != "" {
				t.Errorf("speaker: got %q, want empty", got.Speaker)
			}
		})
	}
}
