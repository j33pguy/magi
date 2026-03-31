package classify

import "testing"

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
