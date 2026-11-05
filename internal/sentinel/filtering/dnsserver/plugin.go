package dnsserver

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"

	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/internal/sentinel/filtering"
)

// domainRuleType is the rule classification the resolver enforces.
const domainRuleType = "DOMAIN"

// ruleSet is the compiled, immutable view of one policy snapshot's DOMAIN
// rules. The zero value forwards every query (no filtering).
type ruleSet struct {
	// whitelist reports whether only listed domains may resolve; otherwise
	// listed domains are refused and everything else is forwarded.
	whitelist bool

	// domains holds the listed domains, lowercased and without a trailing dot.
	domains map[string]struct{}
}

// compileRuleSet builds the ruleSet for snapshot, keeping only DOMAIN rules.
func compileRuleSet(snapshot filtering.PolicySnapshot) *ruleSet {
	compiled := &ruleSet{
		whitelist: snapshot.PolicyType == propagation.PolicyTypeWhitelist,
		domains:   make(map[string]struct{}),
	}
	for _, rule := range snapshot.Rules {
		if rule.Type != domainRuleType {
			continue
		}
		if domain := normalizeDomain(rule.Value); domain != "" {
			compiled.domains[domain] = struct{}{}
		}
	}
	return compiled
}

// allows reports whether name may be forwarded upstream under the rule set.
func (r *ruleSet) allows(name string) bool {
	return r.lists(name) == r.whitelist
}

// lists reports whether name is a listed domain or a subdomain of one.
func (r *ruleSet) lists(name string) bool {
	candidate := normalizeDomain(name)
	for candidate != "" {
		if _, ok := r.domains[candidate]; ok {
			return true
		}
		dot := strings.IndexByte(candidate, '.')
		if dot < 0 {
			return false
		}
		candidate = candidate[dot+1:]
	}
	return false
}

// normalizeDomain lowercases name and strips surrounding space and the
// trailing root dot.
func normalizeDomain(name string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
}

// filterPlugin is the Aegis CoreDNS plugin: it refuses queries the current
// rule set disallows and hands the rest to the next handler (forward).
type filterPlugin struct {
	// rules is the current rule set, swapped atomically on every update.
	rules atomic.Pointer[ruleSet]

	// onResolve receives every A/AAAA answer IP of an allowed WHITELIST
	// resolution; nil disables reporting.
	onResolve func(ip string) error

	// logger records callback failures.
	logger *slog.Logger

	// next is the handler allowed queries are delegated to.
	next plugin.Handler
}

// Name implements plugin.Handler.
func (p *filterPlugin) Name() string { return "aegis" }

// ServeDNS implements plugin.Handler.
func (p *filterPlugin) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	rules := p.rules.Load()
	state := request.Request{W: w, Req: r}

	if !rules.allows(state.Name()) {
		return dns.RcodeRefused, nil
	}

	if rules.whitelist && p.onResolve != nil {
		w = &resolveReporter{ResponseWriter: w, plugin: p}
	}
	return plugin.NextOrFailure(p.Name(), p.next, ctx, w, r)
}

// report hands ip to onResolve, logging a failure without blocking the answer.
func (p *filterPlugin) report(ip string) {
	if err := p.onResolve(ip); err != nil {
		p.logger.Error("failed to report resolved address", "ip", ip, "error", err)
	}
}

// resolveReporter wraps the client writer to report each A/AAAA answer before
// the response reaches the client, so the address is accepted by the packet
// filter by the time the client connects.
type resolveReporter struct {
	dns.ResponseWriter

	// plugin owns the callback the answers are reported to.
	plugin *filterPlugin
}

// WriteMsg reports the answer addresses of m and then writes it to the client.
func (w *resolveReporter) WriteMsg(m *dns.Msg) error {
	for _, answer := range m.Answer {
		switch record := answer.(type) {
		case *dns.A:
			w.plugin.report(record.A.String())
		case *dns.AAAA:
			w.plugin.report(record.AAAA.String())
		}
	}
	return w.ResponseWriter.WriteMsg(m)
}
