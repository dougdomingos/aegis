// Package nftables renders the Aegis-owned nftables ruleset.
package nftables

import (
	"fmt"
	"net"
	"strings"

	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/internal/sentinel/filtering"
)

const (
	// ruleTypeIP is the wire type of a rule the packet filter enforces;
	// DOMAIN rules are enforced by the DNS backend instead.
	ruleTypeIP = "IP"

	// tableDeclaration is the full name of the single nftables table Aegis
	// owns: family inet, table aegis.
	tableDeclaration = "table inet aegis"

	// resolvedSetName is the IPv4 named set the DNS backend fills with the
	// answer addresses of allowed resolutions.
	resolvedSetName = "aegis_resolved"

	// resolvedSetName6 is the IPv6 companion of resolvedSetName.
	resolvedSetName6 = "aegis_resolved6"
)

// RenderRuleset renders the complete `nft -f` input that replaces the Aegis
// nftables table with the ruleset enforcing the provided snapshot. A
// WHITELIST snapshot yields a default-drop output chain that accepts exactly
// the listed IP rules, the resolved-IP sets, and the essential control-plane
// traffic; a BLACKLIST snapshot yields a default-accept output chain that
// drops exactly the listed IP rules. Any other policy type renders the
// permissive BLACKLIST shape so an unexpected value never strands the
// machine.
func RenderRuleset(snapshot filtering.PolicySnapshot) string {
	var ruleset strings.Builder

	writeTableReplacement(&ruleset)
	fmt.Fprintln(&ruleset)
	fmt.Fprintf(&ruleset, "%s {\n", tableDeclaration)

	if snapshot.PolicyType == propagation.PolicyTypeWhitelist {
		renderWhitelist(&ruleset, snapshot)
	} else {
		renderBlacklist(&ruleset, snapshot)
	}

	fmt.Fprintln(&ruleset, "\t}")
	fmt.Fprintln(&ruleset, "}")

	return ruleset.String()
}

// RenderClear renders the `nft -f` input that removes the Aegis nftables
// table, leaving every other table untouched.
func RenderClear() string {
	var ruleset strings.Builder

	writeTableReplacement(&ruleset)

	return ruleset.String()
}

// writeTableReplacement writes the idempotent table removal that precedes
// every ruleset: `add table` is a no-op when the table already exists and
// creates it otherwise, so the `delete` always succeeds whether or not the
// table is present.
func writeTableReplacement(ruleset *strings.Builder) {
	fmt.Fprintln(ruleset, "add "+tableDeclaration)
	fmt.Fprintln(ruleset, "delete "+tableDeclaration)
}

// renderWhitelist renders the empty resolved-IP sets and the default-drop
// output chain that enforce a WHITELIST snapshot.
func renderWhitelist(ruleset *strings.Builder, snapshot filtering.PolicySnapshot) {
	fmt.Fprintf(ruleset, "\tset %s {\n\t\ttype ipv4_addr\n\t}\n\n", resolvedSetName)
	fmt.Fprintf(ruleset, "\tset %s {\n\t\ttype ipv6_addr\n\t}\n\n", resolvedSetName6)

	fmt.Fprintln(ruleset, "\tchain output {")
	fmt.Fprintln(ruleset, "\t\ttype filter hook output priority filter; policy drop;")
	fmt.Fprintln(ruleset, "\t\tct state established,related accept")

	// Loopback carries the queries clients send to the embedded resolver,
	// which are new connections the policy would otherwise drop.
	fmt.Fprintln(ruleset, "\t\toifname \"lo\" accept")

	for _, rule := range snapshot.Rules {
		writeRuleStatements(ruleset, rule, "accept")
	}

	fmt.Fprintf(ruleset, "\t\tip daddr @%s accept\n", resolvedSetName)
	fmt.Fprintf(ruleset, "\t\tip6 daddr @%s accept\n", resolvedSetName6)

	for _, address := range snapshot.OrchestratorIPs {
		fmt.Fprintf(ruleset, "\t\t%s accept\n", destinationMatch(address))
	}

	for _, address := range snapshot.UpstreamDNSIPs {
		fmt.Fprintf(ruleset, "\t\t%s udp dport 53 accept\n", destinationMatch(address))
		fmt.Fprintf(ruleset, "\t\t%s tcp dport 53 accept\n", destinationMatch(address))
	}
}

// renderBlacklist renders the default-accept output chain that enforces a
// BLACKLIST snapshot.
func renderBlacklist(ruleset *strings.Builder, snapshot filtering.PolicySnapshot) {
	fmt.Fprintln(ruleset, "\tchain output {")
	fmt.Fprintln(ruleset, "\t\ttype filter hook output priority filter; policy accept;")

	for _, rule := range snapshot.Rules {
		writeRuleStatements(ruleset, rule, "drop")
	}
}

// writeRuleStatements writes the nft statements enforcing one rule with the
// given verdict, skipping DOMAIN rules, which the DNS backend enforces.
func writeRuleStatements(ruleset *strings.Builder, rule propagation.Rule, verdict string) {
	if rule.Type != ruleTypeIP {
		return
	}

	destination := destinationMatch(rule.Value)

	switch {
	case rule.Protocol != nil && rule.Port != nil:
		protocol := strings.ToLower(*rule.Protocol)
		fmt.Fprintf(ruleset, "\t\t%s %s dport %d %s\n", destination, protocol, *rule.Port, verdict)

	case rule.Protocol != nil:
		protocol := strings.ToLower(*rule.Protocol)
		fmt.Fprintf(ruleset, "\t\t%s meta l4proto %s %s\n", destination, protocol, verdict)

	case rule.Port != nil:
		fmt.Fprintf(ruleset, "\t\t%s tcp dport %d %s\n", destination, *rule.Port, verdict)
		fmt.Fprintf(ruleset, "\t\t%s udp dport %d %s\n", destination, *rule.Port, verdict)

	default:
		fmt.Fprintf(ruleset, "\t\t%s %s\n", destination, verdict)
	}
}

// destinationMatch renders the destination-address match for one address or
// prefix, selecting the IPv6 family for IPv6 values. Values that parse as
// neither match as IPv4, letting `nft -f` reject malformed input so the
// applying executor fails open.
func destinationMatch(value string) string {
	if isIPv6Value(value) {
		return "ip6 daddr " + value
	}

	return "ip daddr " + value
}

// isIPv6Value reports whether the value is an IPv6 address or prefix.
func isIPv6Value(value string) bool {
	if ip := net.ParseIP(value); ip != nil {
		return ip.To4() == nil
	}

	if ip, _, err := net.ParseCIDR(value); err == nil {
		return ip.To4() == nil
	}

	return false
}
