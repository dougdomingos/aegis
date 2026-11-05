// Package filtering declares the local enforcement contract consumed by the
// propagation runner.
package filtering

import "dougdomingos.com/aegis/internal/common/propagation"

// PolicySnapshot is the resolved view of one accepted policy package that the
// filtering backends program locally.
type PolicySnapshot struct {
	// PolicyType is the policy classification:
	// propagation.PolicyTypeWhitelist or propagation.PolicyTypeBlacklist.
	PolicyType string

	// Rules are the policy's filtering rules, as carried on the wire.
	Rules []propagation.Rule

	// OrchestratorIPs are the Orchestrator addresses resolved at apply time;
	// they are always accepted under WHITELIST so the machine keeps its
	// control and revoke path.
	OrchestratorIPs []string

	// UpstreamDNSIPs are the upstream resolver addresses; they are always
	// accepted under WHITELIST so the embedded resolver can forward queries.
	UpstreamDNSIPs []string
}
