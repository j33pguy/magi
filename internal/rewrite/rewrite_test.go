package rewrite

import "testing"

func TestQuery(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Filler stripping
		{"what is kubernetes", "kubernetes"},
		{"how do I configure DNS", "configure domain name system DNS"},
		{"tell me about proxmox clusters", "proxmox clusters"},
		{"can you help with terraform", "help with terraform"},
		{"please show me the config", "show me the config"},

		// Question mark removal
		{"what is a VPN?", "a VPN"},
		{"how does routing work?", "routing work"},

		// Why → cause
		{"why does X fail", "X fail cause"},
		{"why is the server slow?", "the server slow cause"},

		// Abbreviation expansion
		{"k8s deployment", "kubernetes deployment"},
		{"pve cluster setup", "proxmox cluster setup"},
		{"UDM firewall rules", "UDM UniFi Dream Machine firewall rules"},

		// Combined
		{"how do I set up k8s?", "set up kubernetes"},
		{"why does pve crash?", "proxmox crash cause"},
		{"tell me about udm firewall", "UDM UniFi Dream Machine firewall"},

		// No change
		{"exact search term", "exact search term"},
		{"", ""},

		// Preserves casing of non-prefix text
		{"what is My Custom Config", "My Custom Config"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Query(tt.input)
			if got != tt.want {
				t.Errorf("Query(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestQueryReturnsOriginalWhenNoRulesApply(t *testing.T) {
	input := "network topology diagram"
	got := Query(input)
	if got != input {
		t.Errorf("Query(%q) = %q, want unchanged", input, got)
	}
}
