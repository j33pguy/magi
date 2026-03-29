// Package classify provides heuristic content classification for memories.
package classify

import "regexp"

// Classification holds inferred taxonomy fields.
type Classification struct {
	Speaker string
	Area    string
	SubArea string
}

type rule struct {
	pattern *regexp.Regexp
	area    string
	subArea string
}

var rules = []rule{
	// work
	{regexp.MustCompile(`(?i)power.?platform|power bi|power automate|power apps`), "work", "power-platform"},
	{regexp.MustCompile(`(?i)microsoft fabric|fabric capacity|fabric workspace`), "work", "fabric"},
	{regexp.MustCompile(`(?i)power.?bi|pbix|\bdax\b|\bdashboard\b|\breport\b`), "work", "power-bi"},
	{regexp.MustCompile(`(?i)sharepoint|spo\b`), "work", "sharepoint"},
	{regexp.MustCompile(`(?i)td.?synnex|synnex|partner enablement|solutions architect`), "work", "td-synnex"},
	{regexp.MustCompile(`(?i)azure|microsoft 365|m365|\bteams\b|entra`), "work", "azure"},

	// project (before homelab — more specific patterns like vault-unsealer must match before generic vault)
	{regexp.MustCompile(`(?i)magi|memory server|mcp server`), "project", "magi"},
	{regexp.MustCompile(`(?i)distify|behavioral trust|trust verification`), "project", "distify"},
	{regexp.MustCompile(`(?i)labctl|lab.?ctl`), "project", "labctl"},
	{regexp.MustCompile(`(?i)vault.?unsealer|auto.?unseal`), "project", "vault-unsealer"},

	// homelab (ansible/terraform before proxmox — "ansible playbook to configure LXC" should match iac, not proxmox)
	{regexp.MustCompile(`(?i)ansible|playbook|\brole\b|inventory|semaphore`), "homelab", "iac"},
	{regexp.MustCompile(`(?i)terraform|tfstate`), "homelab", "iac"},
	{regexp.MustCompile(`(?i)proxmox|pve|lxc container|\bvm\b|qemu|ct\d+`), "homelab", "proxmox"},
	{regexp.MustCompile(`(?i)pihole|pi-hole|\bdns\b|unbound|blocklist`), "homelab", "dns"},
	{regexp.MustCompile(`(?i)traefik|reverse proxy|let.?s encrypt|acme|\bcertificate\b`), "homelab", "networking"},
	{regexp.MustCompile(`(?i)\bvault\b|hashicorp|approle|\bsecret\b`), "homelab", "vault"},
	{regexp.MustCompile(`(?i)authentik|oidc|\bsso\b|forward.?auth|webauthn|yubikey`), "homelab", "authentik"},
	{regexp.MustCompile(`(?i)grafana|prometheus|loki|alertmanager|monitoring`), "homelab", "monitoring"},
	{regexp.MustCompile(`(?i)lancache|steam.?cache|cdn.?cache`), "homelab", "lancache"},
	{regexp.MustCompile(`(?i)\bvlan\b|\bswitch\b|ubiquiti|unifi|\bnetwork\b`), "homelab", "networking"},

	// home
	{regexp.MustCompile(`(?i)lego|technic|minifig|\bmoc\b|set \d{4,}`), "home", "lego"},
	{regexp.MustCompile(`(?i)twitch|streaming|\bobs\b|ring light`), "home", "streaming"},
	{regexp.MustCompile(`(?i)youtube|j33pguy`), "home", "streaming"},
	{regexp.MustCompile(`(?i)stationeers|gaming|\bgame\b`), "home", "gaming"},

	// family
	{regexp.MustCompile(`(?i)\bkids?\b|\bboys?\b|\bson\b|\bdaughter\b|school|sports`), "family", "kids"},
	{regexp.MustCompile(`(?i)wife|spouse|\bpartner\b`), "family", "spouse"},
}

// Infer returns a best-guess Classification for the given content.
// Returns empty strings for fields it cannot determine.
// Speaker is always left empty — callers should set it explicitly.
func Infer(content string) Classification {
	for _, r := range rules {
		if r.pattern.MatchString(content) {
			return Classification{Area: r.area, SubArea: r.subArea}
		}
	}
	return Classification{}
}
