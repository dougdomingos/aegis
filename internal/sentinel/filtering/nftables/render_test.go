package nftables_test

import (
	"strings"
	"testing"

	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/internal/sentinel/filtering"
	"dougdomingos.com/aegis/internal/sentinel/filtering/nftables"
)

func TestRenderRuleset_WithWhitelistSnapshot_RendersDenyAllWithExplicitAccepts(t *testing.T) {
	testCases := map[string]struct {
		snapshot     filtering.PolicySnapshot
		wantContains []string
		wantOmits    []string
	}{
		"listed rules become explicit accepts": {
			snapshot: filtering.PolicySnapshot{
				PolicyType: propagation.PolicyTypeWhitelist,
				Rules: []propagation.Rule{
					{Type: "IP", Value: "10.0.0.5"},
					{Type: "IP", Value: "10.0.0.6", Protocol: ptr("TCP"), Port: ptr(22)},
					{Type: "DOMAIN", Value: "example.com"},
				},
				OrchestratorIPs: []string{"192.168.10.1"},
				UpstreamDNSIPs:  []string{"192.168.10.53"},
			},
			wantContains: []string{
				"table inet aegis",
				"policy drop",
				"ip daddr 10.0.0.5 accept",
				"ip daddr 10.0.0.6",
				"tcp dport 22",
			},
			wantOmits: []string{"policy accept", "example.com"},
		},
		"empty snapshot renders the deny-all base": {
			snapshot: filtering.PolicySnapshot{
				PolicyType: propagation.PolicyTypeWhitelist,
				Rules:      []propagation.Rule{},
			},
			wantContains: []string{
				"table inet aegis",
				"policy drop",
				"ct state established,related accept",
				"ip daddr @aegis_resolved accept",
			},
			wantOmits: []string{"dport"},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			rendered := nftables.RenderRuleset(tc.snapshot)

			assertContains(t, rendered, tc.wantContains)
			assertOmits(t, rendered, tc.wantOmits)
		})
	}
}

func TestRenderRuleset_WithBlacklistSnapshot_RendersAcceptAllWithExplicitDrops(t *testing.T) {
	testCases := map[string]struct {
		snapshot     filtering.PolicySnapshot
		wantContains []string
		wantOmits    []string
	}{
		"listed rules become explicit drops": {
			snapshot: filtering.PolicySnapshot{
				PolicyType: propagation.PolicyTypeBlacklist,
				Rules: []propagation.Rule{
					{Type: "IP", Value: "10.0.0.5"},
					{Type: "IP", Value: "10.0.0.6", Protocol: ptr("UDP"), Port: ptr(53)},
					{Type: "DOMAIN", Value: "blocked.example"},
				},
			},
			wantContains: []string{
				"table inet aegis",
				"policy accept",
				"ip daddr 10.0.0.5 drop",
				"ip daddr 10.0.0.6",
				"udp dport 53",
			},
			wantOmits: []string{"aegis_resolved", "policy drop", "blocked.example"},
		},
		"empty snapshot renders the accept-all base": {
			snapshot: filtering.PolicySnapshot{
				PolicyType: propagation.PolicyTypeBlacklist,
				Rules:      []propagation.Rule{},
			},
			wantContains: []string{"table inet aegis", "policy accept"},
			wantOmits:    []string{"aegis_resolved", " drop"},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			rendered := nftables.RenderRuleset(tc.snapshot)

			assertContains(t, rendered, tc.wantContains)
			assertOmits(t, rendered, tc.wantOmits)
		})
	}
}

func TestRenderRuleset_WithIPRuleFields_RendersProtocolAndPort(t *testing.T) {
	testCases := map[string]struct {
		rules        []propagation.Rule
		wantContains []string
		wantOmits    []string
	}{
		"tcp rule with port": {
			rules:        []propagation.Rule{{Type: "IP", Value: "10.0.0.5", Protocol: ptr("TCP"), Port: ptr(22)}},
			wantContains: []string{"ip daddr 10.0.0.5", "tcp dport 22"},
		},
		"udp rule with port": {
			rules:        []propagation.Rule{{Type: "IP", Value: "10.0.0.6", Protocol: ptr("UDP"), Port: ptr(53)}},
			wantContains: []string{"ip daddr 10.0.0.6", "udp dport 53"},
		},
		"sctp rule with port": {
			rules:        []propagation.Rule{{Type: "IP", Value: "10.0.0.4", Protocol: ptr("SCTP"), Port: ptr(80)}},
			wantContains: []string{"ip daddr 10.0.0.4", "sctp dport 80"},
		},
		"dccp rule with port": {
			rules:        []propagation.Rule{{Type: "IP", Value: "10.0.0.12", Protocol: ptr("DCCP"), Port: ptr(80)}},
			wantContains: []string{"ip daddr 10.0.0.12", "dccp dport 80"},
		},
		"rule without protocol or port": {
			rules:        []propagation.Rule{{Type: "IP", Value: "10.0.0.9"}},
			wantContains: []string{"ip daddr 10.0.0.9"},
			wantOmits:    []string{"dport", "l4proto"},
		},
		"rule with protocol only": {
			rules:        []propagation.Rule{{Type: "IP", Value: "10.0.0.7", Protocol: ptr("TCP")}},
			wantContains: []string{"ip daddr 10.0.0.7", "meta l4proto tcp"},
		},
		"icmp rule with port is skipped while others still render": {
			rules: []propagation.Rule{
				{Type: "IP", Value: "10.0.0.5", Protocol: ptr("ICMP"), Port: ptr(5)},
				{Type: "IP", Value: "10.0.0.6", Protocol: ptr("TCP"), Port: ptr(22)},
			},
			wantContains: []string{"ip daddr 10.0.0.6", "tcp dport 22"},
			wantOmits:    []string{"10.0.0.5", "icmp"},
		},
		"unknown protocol without port is skipped": {
			rules:     []propagation.Rule{{Type: "IP", Value: "10.0.0.3", Protocol: ptr("HTTP")}},
			wantOmits: []string{"10.0.0.3", "http", "l4proto"},
		},
		"port without protocol is skipped": {
			rules:     []propagation.Rule{{Type: "IP", Value: "10.0.0.8", Port: ptr(53)}},
			wantOmits: []string{"10.0.0.8", "dport"},
		},
		"ipv4 cidr value renders verbatim": {
			rules:        []propagation.Rule{{Type: "IP", Value: "10.0.0.0/8"}},
			wantContains: []string{"ip daddr 10.0.0.0/8"},
		},
		"ipv6 address renders on the ip6 family": {
			rules:        []propagation.Rule{{Type: "IP", Value: "2001:db8::1"}},
			wantContains: []string{"ip6 daddr 2001:db8::1"},
			wantOmits:    []string{"ip daddr 2001:db8::1"},
		},
		"ipv6 cidr renders on the ip6 family": {
			rules:        []propagation.Rule{{Type: "IP", Value: "2001:db8::/32"}},
			wantContains: []string{"ip6 daddr 2001:db8::/32"},
			wantOmits:    []string{"ip daddr 2001:db8::/32"},
		},
		"ipv4-mapped ipv6 cidr renders on the ip6 family": {
			rules:        []propagation.Rule{{Type: "IP", Value: "::ffff:10.0.0.1/128"}},
			wantContains: []string{"ip6 daddr ::ffff:10.0.0.1/128"},
			wantOmits:    []string{"ip daddr ::ffff:10.0.0.1/128"},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			snapshot := filtering.PolicySnapshot{
				PolicyType: propagation.PolicyTypeWhitelist,
				Rules:      tc.rules,
			}

			rendered := nftables.RenderRuleset(snapshot)

			assertContains(t, rendered, tc.wantContains)
			assertOmits(t, rendered, tc.wantOmits)
		})
	}
}

func TestRenderRuleset_WithWhitelistSnapshot_RendersResolvedSetAndEssentials(t *testing.T) {
	testCases := map[string]struct {
		snapshot     filtering.PolicySnapshot
		wantContains []string
	}{
		"essentials are always accepted": {
			snapshot: filtering.PolicySnapshot{
				PolicyType:      propagation.PolicyTypeWhitelist,
				Rules:           []propagation.Rule{{Type: "IP", Value: "10.0.0.5"}},
				OrchestratorIPs: []string{"192.168.10.1", "192.168.10.2"},
				UpstreamDNSIPs:  []string{"192.168.10.53"},
			},
			wantContains: []string{
				"set aegis_resolved",
				"ip daddr @aegis_resolved accept",
				"ct state established,related accept",
				`oifname "lo" accept`,
				"ip daddr 192.168.10.1 accept",
				"ip daddr 192.168.10.2 accept",
				"ip daddr 192.168.10.53 udp dport 53 accept",
				"ip daddr 192.168.10.53 tcp dport 53 accept",
			},
		},
		"domain-only whitelist keeps the resolved accept": {
			snapshot: filtering.PolicySnapshot{
				PolicyType:      propagation.PolicyTypeWhitelist,
				Rules:           []propagation.Rule{{Type: "DOMAIN", Value: "example.com"}},
				OrchestratorIPs: []string{"192.168.10.1"},
				UpstreamDNSIPs:  []string{"192.168.10.53"},
			},
			wantContains: []string{
				"set aegis_resolved",
				"ip daddr @aegis_resolved accept",
				"policy drop",
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			rendered := nftables.RenderRuleset(tc.snapshot)

			assertContains(t, rendered, tc.wantContains)
		})
	}
}

func TestRenderClear_RendersTableDeletion(t *testing.T) {
	rendered := nftables.RenderClear()

	assertContains(t, rendered, []string{"delete table inet aegis"})
	assertOmits(t, rendered, []string{"aegis_resolved"})
}

// assertContains asserts that the rendered ruleset holds every wanted
// fragment.
func assertContains(t *testing.T, rendered string, fragments []string) {
	t.Helper()

	for _, fragment := range fragments {
		if !strings.Contains(rendered, fragment) {
			t.Errorf("rendered ruleset is missing %q:\n%s", fragment, rendered)
		}
	}
}

// assertOmits asserts that the rendered ruleset does not hold any unwanted
// fragment.
func assertOmits(t *testing.T, rendered string, fragments []string) {
	t.Helper()

	for _, fragment := range fragments {
		if strings.Contains(rendered, fragment) {
			t.Errorf("rendered ruleset unexpectedly holds %q:\n%s", fragment, rendered)
		}
	}
}

// ptr returns a pointer to the provided value.
func ptr[T any](v T) *T {
	return &v
}
