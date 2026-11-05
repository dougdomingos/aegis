# US10 — Local Rule Application (Issue #20)

## Intent and Why

Aegis restricts web access in college labs during online exams: the
Orchestrator distributes policies (e.g. "only the exam site", "block personal
VMs over SSH"), and each lab machine must enforce them locally. Today the
Sentinel validates, persists, and acks received packages
(`internal/sentinel/propagation`), but nothing touches the machine's network
stack. US10 closes that gap: rules received through propagation are applied as
local filtering, and the Orchestrator can see which machines confirmed
application.

## Goals

- Apply every rule of an accepted package to local filtering: DOMAIN rules via
  an embedded CoreDNS resolver, IP/protocol/port rules via an Aegis-owned
  nftables table.
- Apply the policy's semantics in full: WHITELIST means deny-all-outbound
  except explicitly listed traffic and what the OS needs to function;
  BLACKLIST means allow-all except listed traffic.
- Enforce DOMAIN rules at the connection level too: when CoreDNS resolves a
  whitelisted domain, the answer IPs are inserted into an nftables named set
  the whitelist accepts, so a DOMAIN-only whitelist (e.g. "only the exam
  site") actually permits connections to that site.
- Ack only after successful application, so an ack is an honest "this machine
  is enforcing the policy" signal (acceptance criterion 2, via the existing
  ack stream).
- Clear all filtering on REVOKE.

## Non-goals

- DoH/DoT blocking and other DNS-bypass hardening (explicitly out of MVP
  scope; the exam OS image is built by the team, so resolver configuration is
  baked into the ISO, not fought over at runtime).
- eBPF: considered and rejected — for static IP rules it offers no capability
  nftables lacks, at a higher maintenance cost.
- Orchestrator-side "machines confirmed" reporting UI — the ack data already
  flows; surfacing it is a separate story.
- Per-user or per-process enforcement.

## Anti-goals

- Failing closed on apply errors. Rule updates are occasional, not the norm:
  a failed apply keeps the previous filtering in place and nacks, so the
  proctor sees the machine did not confirm and can intervene.
- Clobbering the OS image's own nftables rules: Aegis owns exactly one table
  and never touches anything else.
- Managing `/etc/resolv.conf` or external processes at runtime.

## Constraints

- The exam OS is built in-house and **ephemeral**: every boot starts clean
  from the image, so there is no persisted state to re-apply at boot. The
  machine authenticates fresh at startup and receives its rules over the
  propagation stream. The image provides: `/etc/resolv.conf` pointing at the
  Sentinel's local resolver, a baseline nftables ruleset, and systemd ordering
  so the Sentinel starts before login.
- Go 1.26.5, module `dougdomingos.com/aegis`, existing layered
  architecture and test conventions (AGENTS.md).
- Rule types are DOMAIN and IP (IP rules may carry protocol/port); policy
  types are WHITELIST and BLACKLIST.
- Existing migrations and wire formats must not change.

## Approaches Considered

1. **CoreDNS embedded as a library + `nft` CLI via exec** (chosen). One
   process, one systemd unit; a custom CoreDNS plugin is fed rules in-process
   from the propagation runner — no generated config files, reload signaling,
   or child-process supervision. `nft -f` with a generated ruleset is stable,
   hand-debuggable, and easy to exercise in the e2e Testcontainers suite.
2. **CoreDNS as a managed external binary + `nft` CLI.** Requires supervising
   a child, rendering and reloading Corefile on every package, and IPC between
   the runner and the resolver. More moving parts, no added capability.
3. **nftables via the Go netlink library (`google/nftables`).** Avoids exec,
   but its API is verbose and error-prone around kernel/nft version
   differences, and rulesets are harder to inspect by hand in a lab.
4. **eBPF (XDP/cgroup) for IP rules.** Rejected: equal enforcement to
   nftables for static rules, plus kernel-version floors, BPF map syncing,
   and harder debugging. Its unique strengths (in-kernel DNS/SNI parsing)
   are redundant while CoreDNS covers domains.

## Design

### New package: `internal/sentinel/filtering`

Declares the interface consumed by the propagation runner:

```go
// Filter applies and clears the local packet and DNS filtering.
type Filter interface {
    // Apply programs nftables and the resolver for the given state.
    Apply(state State) error
    // Clear removes every Aegis-owned filtering rule.
    Clear() error
}
```

`State` here is the propagation state (epoch/sequence/policy metadata/rules)
or a slimmed view of it; the concrete composition lives in `cmd/sentinel`
wiring. Sub-packages:

- `filtering/nftables` — renders an nftables ruleset for the dedicated
  `inet filter aegis` table and applies it via `nft -f`. Whitelist: base
  chain with a drop policy on output, explicit accepts for the listed
  IP/protocol/port rules, an accept for the `aegis_resolved` named set fed by
  the DNS plugin (see below), plus accepts for OS-essential traffic — outbound
  DNS to the configured upstream resolvers (so CoreDNS can forward), the
  Orchestrator's address (resolved from `orchestrator_url` /
  `propagation_address` at apply time and refreshed on every apply, otherwise
  the first whitelist would strand the machine from its revoke path), and
  established/related conntrack state. Blacklist: accept-all base with
  explicit drops for the listed rules. Revoke/empty state: table removed.
  Rendering is pure (ruleset text from state) and unit-tested without root.
- `filtering/dnsserver` — embeds `github.com/coredns/coredns` as a library,
  listening on the loopback address the OS image points `resolv.conf` at. A
  custom plugin holds the current rule set in memory: WHITELIST resolves
  only listed domains and forwards them upstream, refusing everything else;
  BLACKLIST forwards everything except listed domains, which are refused.
  A DOMAIN rule matches the domain and all of its subdomains (`example.com`
  covers `www.example.com`), the convention filtering tools use and what
  exam sites on CDN subdomains need. Under WHITELIST, every A/AAAA answer
  for an allowed domain is also inserted into the `aegis_resolved` nftables
  set, so connections to those IPs pass the packet filter. Updates are an
  atomic pointer swap — no restarts, no config reload.

### Application flow in the propagation runner

`apply()` gains a step between validation and persistence:

1. Validate the message (unchanged).
2. Program the new filtering: `nftables` first (the stronger enforcement;
   `nft -f` applies the whole ruleset as one transaction), then `dnsserver`
   (an in-memory atomic swap). On a DNS failure the swap is rolled back, so
   a failed apply never leaves half-applied state. On any error: keep the
   previous filtering untouched, log, and nack with the cause — fail open.
3. Persist state and ack, as today.

On REVOKE: clear both backends, persist the emptied state, ack. Because the
exam OS is ephemeral, there is no boot-time re-application: every boot starts
with empty state and receives its rules over the stream after authenticating.

### Ack semantics

`AckAcked` now means "validated, applied, and persisted". A nack reason
distinguishes validation failures from application failures (e.g.
`failed to apply filtering: ...`), giving the future reporting story
everything it needs. When `filtering_enabled` is false the runner nacks with
`filtering disabled` — an ack must never claim enforcement that did not
happen, even on a development box.

### Configuration

New optional fields in the Sentinel JSON config, following existing
conventions: `dns_listen_address` (default `127.0.0.1:53`),
`dns_upstream_servers` (default resolvers read from the system, or the
Orchestrator host), `nft_binary_path` (default `nft`), and a toggle to
disable filtering (for non-lab development, e.g. `filtering_enabled`,
default true) so `make tests` and dev machines never need root.

### CIDR support

`common/rules` validation widens to accept CIDR notation for IP rules
(`net.ParseCIDR`) in addition to single addresses — nftables handles CIDRs
natively, and doing it now avoids a breaking change once rules are stored.
Orchestrator-side validation widens identically since both share
`rules.Validate`.

## Testing Strategy

- `filtering/nftables`: pure renderer unit tests over the generated ruleset
  text (whitelist/blacklist/revoke, protocol/port combinations, CIDRs).
- `filtering/dnsserver`: plugin tests resolving against a stub upstream,
  asserting whitelist/blacklist behavior.
- `propagation`: existing tests updated — ack requires the mock Filter to
  succeed; new cases for apply-failure nack and nack-when-filtering-disabled.
- e2e (Testcontainers): a real container applies a package and the test
  asserts filtering behavior (e.g. `nft list table` output, DNS response
  codes).

## Left to the Builder

- Exact nftables chain layout and rule ordering within the Aegis table.
- The upstream resolver source when `dns_upstream_servers` is unset
  (system `/etc/resolv.conf` parsed at startup vs. configured default).
- Lifecycle of `aegis_resolved` set entries: refresh cadence relative to DNS
  TTLs, and whether entries are pruned or only overwritten on each apply.
- How the e2e suite gains the privileges needed for `nft` inside containers
  (privileged container vs. a dedicated filter-enabled service).
