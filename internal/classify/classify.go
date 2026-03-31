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
	{regexp.MustCompile(`(?i)azure|microsoft 365|m365|\bteams\b|entra`), "work", "azure"},

	// project (before infrastructure — more specific patterns must match before generic ones)
	{regexp.MustCompile(`(?i)magi|memory server|mcp server`), "project", "magi"},

	// infrastructure (iac before compute — "ansible playbook to configure containers" should match iac, not compute)
	{regexp.MustCompile(`(?i)ansible|playbook|\brole\b|inventory|semaphore`), "infrastructure", "iac"},
	{regexp.MustCompile(`(?i)terraform|tfstate|pulumi|cloudformation`), "infrastructure", "iac"},
	{regexp.MustCompile(`(?i)hypervisor|virtualization|\bvm\b|qemu|container runtime|lxc|docker|podman`), "infrastructure", "compute"},
	{regexp.MustCompile(`(?i)\bdns\b|nameserver|resolver|blocklist|domain resolution`), "infrastructure", "dns"},
	{regexp.MustCompile(`(?i)reverse proxy|load.?balancer|ingress|let.?s encrypt|acme|\bcertificate\b|tls termination`), "infrastructure", "networking"},
	{regexp.MustCompile(`(?i)\bvault\b|hashicorp|approle|\bsecret\b|credential.?store`), "infrastructure", "security"},
	{regexp.MustCompile(`(?i)oidc|\bsso\b|forward.?auth|webauthn|yubikey|identity.?provider|saml`), "infrastructure", "security"},
	{regexp.MustCompile(`(?i)grafana|prometheus|loki|alertmanager|monitoring|observability|metrics`), "infrastructure", "monitoring"},
	{regexp.MustCompile(`(?i)cdn.?cache|object.?storage|\bnfs\b|\bceph\b|backup.?storage`), "infrastructure", "storage"},
	{regexp.MustCompile(`(?i)\bvlan\b|\bswitch\b|\bfirewall\b|\brouter\b|\bnetwork\b|subnet`), "infrastructure", "networking"},
	{regexp.MustCompile(`(?i)ci.?cd|github.?actions|gitlab.?ci|jenkins|pipeline|build.?server`), "infrastructure", "ci-cd"},

	// personal
	{regexp.MustCompile(`(?i)hobby|side.?project|personal|free.?time`), "personal", "general"},
	{regexp.MustCompile(`(?i)gaming|\bgame\b|streaming|\bobs\b`), "personal", "gaming"},

	// development
	{regexp.MustCompile(`(?i)\bapi\b|endpoint|rest|grpc|graphql`), "development", "api"},
	{regexp.MustCompile(`(?i)database|\bsql\b|migration|schema`), "development", "database"},
	{regexp.MustCompile(`(?i)testing|\btest\b|coverage|benchmark`), "development", "testing"},
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
