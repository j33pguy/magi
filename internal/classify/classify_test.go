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
			name:    "traefik reverse proxy",
			content: "Set up Traefik reverse proxy with Let's Encrypt",
			area:    "homelab",
			subArea: "networking",
		},
		{
			name:    "td synnex power platform",
			content: "Power Platform solution architect meeting at TD SYNNEX",
			area:    "work",
			subArea: "power-platform",
		},
		{
			name:    "lego technic with kids",
			content: "Built the LEGO Technic tractor set with the boys",
			area:    "home",
			subArea: "lego",
		},
		{
			name:    "proxmox lxc containers",
			content: "Proxmox node hypervisor-01 running LXC containers for all services",
			area:    "homelab",
			subArea: "proxmox",
		},
		{
			name:    "fabric workspace",
			content: "Created a new Microsoft Fabric workspace for the analytics team",
			area:    "work",
			subArea: "fabric",
		},
		{
			name:    "pihole dns",
			content: "Updated Pi-hole blocklist and restarted unbound resolver",
			area:    "homelab",
			subArea: "dns",
		},
		{
			name:    "magi project",
			content: "Refactored magi MCP server to use gRPC transport",
			area:    "project",
			subArea: "magi",
		},
		{
			name:    "vault unsealer",
			content: "Deployed the vault-unsealer service for auto-unseal on boot",
			area:    "project",
			subArea: "vault-unsealer",
		},
		{
			name:    "grafana monitoring",
			content: "Set up Grafana dashboards with Prometheus metrics for homelab",
			area:    "homelab",
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
			content: "Wrote Ansible playbook to configure all LXC containers",
			area:    "homelab",
			subArea: "iac",
		},
		{
			name:    "gaming stationeers",
			content: "Playing Stationeers on the dedicated game server",
			area:    "home",
			subArea: "gaming",
		},
		{
			name:    "authentik sso",
			content: "Configured Authentik OIDC provider for SSO across all services",
			area:    "homelab",
			subArea: "authentik",
		},
		{
			name:    "distify project",
			content: "Working on distify behavioral trust verification module",
			area:    "project",
			subArea: "distify",
		},
		{
			name:    "family kids school",
			content: "Picked up the kids from school and drove to sports practice",
			area:    "family",
			subArea: "kids",
		},
		{
			name:    "lancache steam",
			content: "lancache pre-cached the latest Steam updates overnight",
			area:    "homelab",
			subArea: "lancache",
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
