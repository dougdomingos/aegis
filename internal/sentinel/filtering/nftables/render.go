// Package nftables renders the Aegis-owned nftables ruleset.
package nftables

import (
	"fmt"
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

	// ResolvedSetName is the IPv4 named set the DNS backend fills with the
	// answer addresses of allowed resolutions.
	ResolvedSetName = "aegis_resolved"

	// ResolvedSetName6 is the IPv6 companion of ResolvedSetName.
	ResolvedSetName6 = "aegis_resolved6"
)

// supportedProtocols lists the protocols the renderer may emit as nftables
// clauses, keyed by their lowercase keywords. A rule naming any other
// protocol is skipped: nftables would reject its clause and abort the whole
// `nft -f` file.
var supportedProtocols = map[string]struct{}{
	"tcp":  {},
	"udp":  {},
	"sctp": {},
	"dccp": {},
}

// RenderRuleset renders the complete `nft -f` input that replaces the Aegis
// nftables table with the ruleset enforcing the provided snapshot. A
// WHITELIST snapshot yields a default-drop output chain that accepts exactly
// the listed IP rules, the resolved-IP sets, and the essential control-plane
// traffic; a BLACKLIST snapshot yields a default-accept output chain that
// drops exactly the listed IP rules. Any other policy type renders the
// permissive BLACKLIST shape so an unexpected value never strands the
// machine. Rules whose protocol or port clauses nftables cannot express are
// skipped rather than rendered, so one bad rule cannot abort the whole file.
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
	fmt.Fprintf(ruleset, "\tset %s {\n\t\ttype ipv4_addr\n\t}\n\n", ResolvedSetName)
	fmt.Fprintf(ruleset, "\tset %s {\n\t\ttype ipv6_addr\n\t}\n\n", ResolvedSetName6)

	fmt.Fprintln(ruleset, "\tchain output {")
	fmt.Fprintln(ruleset, "\t\ttype filter hook output priority filter; policy drop;")
	fmt.Fprintln(ruleset, "\t\tct state established,related accept")

	// Loopback carries the queries clients send to the embedded resolver,
	// which are new connections the policy would otherwise drop.
	fmt.Fprintln(ruleset, "\t\toifname \"lo\" accept")

	for _, rule := range snapshot.Rules {
		writeRuleStatements(ruleset, rule, "accept")
	}

	fmt.Fprintf(ruleset, "\t\tip daddr @%s accept\n", ResolvedSetName)
	fmt.Fprintf(ruleset, "\t\tip6 daddr @%s accept\n", ResolvedSetName6)

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
// given verdict. DOMAIN rules are skipped, as the DNS backend enforces them,
// and so are rules whose protocol or port clauses nftables cannot express:
// an unparseable clause would abort the whole `nft -f` file, and skipping
// keeps one bad rule from disabling the ruleset.
func writeRuleStatements(ruleset *strings.Builder, rule propagation.Rule, verdict string) {
	if rule.Type != ruleTypeIP {
		return
	}

	switch {
	case rule.Port != nil:
		// A destination-port clause needs a protocol nftables can match by
		// port; a port with no protocol at all is equally inexpressible.
		protocol, ok := supportedProtocol(rule.Protocol)
		if !ok {
			return
		}

		fmt.Fprintf(ruleset, "\t\t%s %s dport %d %s\n", destinationMatch(rule.Value), protocol, *rule.Port, verdict)

	case rule.Protocol != nil:
		// Without a port, the protocol still has to be one nftables knows
		// by name, or the clause would be rejected.
		protocol, ok := supportedProtocol(rule.Protocol)
		if !ok {
			return
		}

		fmt.Fprintf(ruleset, "\t\t%s meta l4proto %s %s\n", destinationMatch(rule.Value), protocol, verdict)

	default:
		fmt.Fprintf(ruleset, "\t\t%s %s\n", destinationMatch(rule.Value), verdict)
	}
}

// supportedProtocol resolves a rule protocol to the lowercase nftables
// keyword the renderer may emit, reporting whether it is supported.
func supportedProtocol(protocol *string) (string, bool) {
	if protocol == nil {
		return "", false
	}

	keyword := strings.ToLower(*protocol)
	if _, ok := supportedProtocols[keyword]; !ok {
		return "", false
	}

	return keyword, true
}

// destinationMatch renders the destination-address match for one address or
// prefix: values containing a colon use the IPv6 family, everything else
// matches as IPv4. Malformed values are still passed through so `nft -f`
// rejects them and the applying executor fails open.
func destinationMatch(value string) string {
	if strings.Contains(value, ":") {
		return "ip6 daddr " + value
	}

	return "ip daddr " + value
}
