// Package rewrite provides deterministic query rewriting for adaptive retrieval.
package rewrite

import "strings"

// fillerPrefixes are common question prefixes that can be stripped to improve search.
var fillerPrefixes = []string{
	"what is ",
	"what are ",
	"how do i ",
	"how does ",
	"how do ",
	"how to ",
	"tell me about ",
	"can you tell me ",
	"can you ",
	"could you ",
	"please ",
	"i want to know ",
	"i want to ",
	"i need to ",
	"why does ",
	"why do ",
	"why is ",
	"where is ",
	"where are ",
	"when did ",
	"when does ",
}

// abbreviations maps common shorthand to expanded forms for better recall.
var abbreviations = map[string]string{
	"udm":  "UDM UniFi Dream Machine",
	"usg":  "USG UniFi Security Gateway",
	"k8s":  "kubernetes",
	"pve":  "hypervisor",
	"tf":   "terraform",
	"gh":   "github",
	"gha":  "github actions",
	"cicd": "continuous integration deployment",
	"dns":  "domain name system DNS",
	"lb":   "load balancer",
	"vm":   "virtual machine",
	"vms":  "virtual machines",
	"ha":   "high availability",
	"hass": "home assistant",
	"ts":   "typescript",
	"js":   "javascript",
	"py":   "python",
	"pg":   "postgres postgresql",
	"db":   "database",
	"api":  "API application programming interface",
}

// whyPrefixes are the "why" filler prefixes that trigger cause-suffix addition.
var whyPrefixes = []string{
	"why does ",
	"why do ",
	"why is ",
}

// Query rewrites a search query for better retrieval by stripping filler words,
// expanding abbreviations, and converting questions to statements.
// Returns the original query unchanged if no rewriting rules apply.
func Query(query string) string {
	result := strings.TrimSpace(query)
	if result == "" {
		return result
	}

	lower := strings.ToLower(result)
	strippedWhy := false

	// Strip filler prefixes (case-insensitive match, preserve casing of remainder)
	for _, prefix := range fillerPrefixes {
		if strings.HasPrefix(lower, prefix) {
			for _, wp := range whyPrefixes {
				if strings.HasPrefix(lower, wp) {
					strippedWhy = true
					break
				}
			}
			result = result[len(prefix):]
			lower = strings.ToLower(result)
			break
		}
	}

	// Strip trailing question mark and whitespace
	result = strings.TrimRight(result, "? ")
	result = strings.TrimSpace(result)

	// If we stripped a "why" prefix, append "cause" for better matching
	if strippedWhy && result != "" {
		result += " cause"
	}

	// Expand abbreviations (case-insensitive whole-word match)
	words := strings.Fields(result)
	changed := false
	for i, w := range words {
		if replacement, ok := abbreviations[strings.ToLower(w)]; ok {
			words[i] = replacement
			changed = true
		}
	}
	if changed {
		result = strings.Join(words, " ")
	}

	return result
}
