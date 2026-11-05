# US10 Local Filtering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Sentinel enforce accepted policies locally — DOMAIN rules through an embedded CoreDNS resolver, IP/protocol/port rules through an Aegis-owned nftables table — and ack only after successful enforcement.

**Architecture:** A new `internal/sentinel/filtering` package declares the `Filter` interface consumed by the propagation runner, composed of two backends: a pure nftables ruleset renderer + `nft` CLI executor, and a CoreDNS-embedded resolver whose plugin swaps rules in memory and feeds resolved IPs back into an nftables named set. The propagation runner programs filtering between validation and persistence, failing open on errors.

**Tech Stack:** Go 1.26.5, `github.com/coredns/coredns` (as a library), `nft` CLI via `os/exec`, `log/slog`, existing gRPC propagation stream.

**Spec:** `docs/superpowers/specs/2026-11-04-us10-local-filtering-design.md`

## Global Constraints

- Module path is `dougdomingos.com/aegis`; Go version is `1.26.5`.
- Every exported symbol carries a doc comment starting with its name. Do not add doc comments to methods that implement an interface.
- Tests live beside their targets in external packages (`nftables_test`, `dnsserver_test`, `filtering_test`, `propagation_test`), follow `Test<Type>_<Method>_<Scenario>_<ExpectedResult>`, use Arrange-Act-Assert, and use `t.Errorf`/`t.Fatalf`. Table-driven tests use `map[string]struct{...}` with `t.Run`.
- **Do not commit automatically.** `AGENTS.md` overrides the skill's commit step. Every task ends by staging its files for developer review; the suggested message is included.
- Coverage gate is 75% across the `Makefile`'s `TEST_PACKAGES`; this plan extends that variable.
- Fail open: an apply error keeps prior filtering untouched and nacks. An ack always means "validated, applied, and persisted".
- Aegis owns exactly one nftables table, `inet filter aegis`, and never touches other tables. The resolved-IP set is named `aegis_resolved`.
- Wire formats, migrations, and existing `State` JSON are unchanged.
- Branch naming: `feat/issue-20-local-filtering`.
- The exam OS is ephemeral: no boot-time re-application of persisted state.

## Review Focus

1. **A WHITELIST containing only DOMAIN rules must permit connections to the resolved IPs.** Pinned by Task 2's renderer test asserting the `aegis_resolved` set accept, and Task 4's plugin test asserting the resolve callback fires with the answer IPs.
2. **The first WHITELIST apply must not strand the machine:** the ruleset must accept the Orchestrator's address and upstream DNS, or the machine can never receive its revoke. Pinned by Task 2's renderer tests.
3. **A DNS failure after a successful nft apply must roll back and nack** — never half-applied state. Pinned by Task 6's composite test.
4. **With filtering disabled, every package must nack** with `filtering disabled`, never ack. Pinned by Task 7's runner tests.
5. **REVOKE must fully clear both backends:** table removed, DNS rules emptied, empty state persisted. Pinned by Task 2 (renderer), Task 6 (composite), and Task 7 (runner) revoke tests.

---

### Task 1: CIDR Support in the Shared Rule Validator

**Files:**
- Modify: `internal/common/rules/rules.go:53-56` (the `IP` branch of `Validate`)
- Modify: `internal/common/rules/rules_test.go`

**Interfaces:**
- Consumes: `rules.Rule`, `internal/common/errors` (unchanged).
- Produces: `rules.Validate` now accepts both single IPs and CIDR notation for `IP` rules; error set unchanged.

- [ ] **Step 1: Write the failing tests**

Add to `internal/common/rules/rules_test.go` (package `rules_test`), table-driven:

```go
func TestRules_Validate_WithIPRuleVariations_ReturnsExpectedResult(t *testing.T)
```

Cases (`map[string]struct{...}`): `10.0.0.0/8` valid, `2001:db8::/32` valid, `10.0.0.0/33` invalid (`errors.ErrMalformedRuleValue`), `10.0.0.1` still valid.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/common/rules/...`
Expected: FAIL — the CIDR cases return `ErrMalformedRuleValue` because `net.ParseIP` rejects `10.0.0.0/8`.

- [ ] **Step 3: Implement the CIDR fallback**

In the `IP` branch, accept the value when `net.ParseIP(value) != nil` **or** `net.ParseCIDR(value)` returns no error. Port/protocol checks unchanged.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/common/rules/... ./internal/orchestrator/domain/... ./internal/orchestrator/...`
Expected: PASS, including the Orchestrator's existing rule tests (it shares `rules.Validate`).

- [ ] **Step 5: Stage for review**

```bash
git add internal/common/rules
# suggested message: "feat: accept CIDR notation in IP rules"
```

### Task 2: Filtering Snapshot and Nftables Ruleset Renderer

**Files:**
- Create: `internal/sentinel/filtering/filtering.go`
- Create: `internal/sentinel/filtering/nftables/render.go`
- Test: `internal/sentinel/filtering/nftables/render_test.go`

**Interfaces:**
- Consumes: `internal/common/propagation.Rule`, `internal/common/propagation.PolicyTypeWhitelist`/`PolicyTypeBlacklist`.
- Produces:

```go
// Package filtering declares the local enforcement contract consumed by the
// propagation runner.
type PolicySnapshot struct {
    PolicyType      string   // propagation.PolicyTypeWhitelist or PolicyTypeBlacklist
    Rules           []propagation.Rule
    OrchestratorIPs []string // resolved at apply time; always accepted under WHITELIST
    UpstreamDNSIPs  []string // upstream resolvers; always accepted under WHITELIST
}

// Package nftables renders the Aegis-owned nftables ruleset.
func RenderRuleset(snapshot filtering.PolicySnapshot) string  // full `nft -f` input for table inet filter aegis
func RenderClear() string                                     // ruleset that deletes the table
```

- [ ] **Step 1: Write the failing tests**

Create `internal/sentinel/filtering/nftables/render_test.go` in `package nftables_test`, table-driven over rendered strings, asserting substrings (not exact full text):

```go
func TestRenderRuleset_WithWhitelistSnapshot_RendersDenyAllWithExplicitAccepts(t *testing.T)
func TestRenderRuleset_WithBlacklistSnapshot_RendersAcceptAllWithExplicitDrops(t *testing.T)
func TestRenderRuleset_WithIPRuleFields_RendersProtocolAndPort(t *testing.T)
func TestRenderRuleset_WithWhitelistSnapshot_RendersResolvedSetAndEssentials(t *testing.T)
func TestRenderClear_RendersTableDeletion(t *testing.T)
```

The whitelist test asserts: `table inet aegis` (or `inet filter aegis` — one table name, used consistently), a base output chain with `policy drop`, an accept for each listed IP rule, `@aegis_resolved` accept, an accept for each `OrchestratorIPs` and `UpstreamDNSIPs` entry (UDP+TCP dport 53 for the latter), and `ct state established,related accept`. The blacklist test asserts `policy accept` with a drop per listed rule and no `aegis_resolved` references. The IP-rule test asserts `ip daddr <ip>` plus `tcp dport 22` when `Protocol`/`Port` are set, and CIDR values render verbatim.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/sentinel/filtering/...`
Expected: FAIL — package/symbols undefined.

- [ ] **Step 3: Implement `PolicySnapshot` and the renderer**

`filtering.go` holds only the snapshot type. `render.go` builds the ruleset as template text: create `table inet aegis`, flush it, one `base chain output` with the policy, ordered rule lines, and — under WHITELIST only — declare `set aegis_resolved { type ipv4_addr; }` and its accept plus the essential accepts. Builder's free choice: exact chain layout, flushing strategy (`delete table` + recreate vs `flush table`), and IPv6 handling (`ipv4_addr` + `ipv6_addr` sets or `ether type` matching); tests must not depend on those choices beyond the pinned substrings.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/sentinel/filtering/...`
Expected: PASS.

- [ ] **Step 5: Stage for review**

```bash
git add internal/sentinel/filtering
# suggested message: "feat: render Aegis nftables rulesets from policy snapshots"
```

### Task 3: Nftables Executor

**Files:**
- Create: `internal/sentinel/filtering/nftables/executor.go`
- Test: `internal/sentinel/filtering/nftables/executor_test.go`

**Interfaces:**
- Consumes: `RenderRuleset`, `RenderClear` from Task 2.
- Produces:

```go
// Executor programs the Aegis nftables table through the nft CLI.
func NewExecutor(binaryPath string) *Executor
func (e *Executor) Apply(snapshot filtering.PolicySnapshot) error  // runs `nft -f` with RenderRuleset via stdin
func (e *Executor) Clear() error                                   // runs `nft -f` with RenderClear via stdin
func (e *Executor) AddResolvedIP(ip string) error                  // runs `nft add element inet aegis aegis_resolved { ip }`
```

- [ ] **Step 1: Write the failing tests**

Create `executor_test.go` in `package nftables_test` with a fake runner injected for testing:

```go
// runFunc executes a command, mirroring exec.Command's signature surface
// needed here. It is a field on Executor so tests can capture invocations.
type runFunc func(binary string, args []string, stdin string) error
```

Tests (`TestExecutor_<Method>_<Scenario>_<ExpectedResult>`): `Apply` invokes the binary once with `-f`, stdin containing the rendered ruleset, and returns the runner's error; `Clear` likewise; `AddResolvedIP` passes `add element inet aegis aegis_resolved { 1.2.3.4 }`; a runner error propagates wrapped with the command context. Use table-driven cases where natural.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/sentinel/filtering/...`
Expected: FAIL — `NewExecutor` undefined.

- [ ] **Step 3: Implement the executor**

Field `run runFunc` defaulting to an `os/exec` implementation: `cmd := exec.Command(binary, args...)`, stdin fed the ruleset, `cmd.Run()`. `NewExecutor(binaryPath)` defaults to `nft` when the path is empty. Errors wrap the failed command.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/sentinel/filtering/...`
Expected: PASS.

- [ ] **Step 5: Stage for review**

```bash
git add internal/sentinel/filtering
# suggested message: "feat: apply Aegis rulesets through the nft CLI"
```

### Task 4: DNS Server with the Aegis CoreDNS Plugin

**Files:**
- Create: `internal/sentinel/filtering/dnsserver/server.go`
- Create: `internal/sentinel/filtering/dnsserver/plugin.go`
- Test: `internal/sentinel/filtering/dnsserver/server_test.go`

**Interfaces:**
- Consumes: `filtering.PolicySnapshot` (Task 2); `github.com/coredns/coredns` libraries.
- Produces:

```go
// Server runs the embedded CoreDNS resolver enforcing DOMAIN rules.
func NewServer(listenAddress string, upstream []string, onResolve func(ip string)) *Server
func (s *Server) Start(ctx context.Context) error  // blocks until ctx is cancelled
func (s *Server) Update(snapshot filtering.PolicySnapshot) // atomic in-memory swap; synchronous-safe
func (s *Server) Clear()                            // swap to the empty set
```

`onResolve` is invoked with each A/AAAA answer IP for every allowed (WHITELIST) resolution — never for BLACKLIST, never for refused queries. The wiring passes the nftables `AddResolvedIP` here; the package itself stays transport-agnostic.

- [ ] **Step 0: Add the dependency**

Run: `go get github.com/coredns/coredns@latest` and `go mod tidy`. Note: CoreDNS pulls a large dependency tree; this is expected and accepted by the spec.

- [ ] **Step 1: Write the failing tests**

Create `server_test.go` in `package dnsserver_test` using a stub upstream DNS server (a `net.UDPConn`-backed helper answering A queries with a fixed IP) on an ephemeral port, and the Aegis server on an ephemeral loopback port. Arranged per scenario via `NewServer`, `Update`, then a client query:

```go
func TestServer_Update_WithWhitelist_ResolvesListedDomainAndRefusesOthers(t *testing.T)
func TestServer_Update_WithWhitelist_MatchesSubdomainsOfListedDomain(t *testing.T)
func TestServer_Update_WithWhitelist_ReportsResolvedIPsToCallback(t *testing.T)
func TestServer_Update_WithBlacklist_RefusesListedDomainAndForwardsOthers(t *testing.T)
func TestServer_Update_WithBlacklist_NeverReportsResolvedIPs(t *testing.T)
func TestServer_Clear_ResolvesNothingAfterwards(t *testing.T)
func TestServer_Update_WithConcurrentQueries_NeverServesTornState(t *testing.T)
```

Assertions use `github.com/miekg/dns` client messages: refused = `dns.RcodeRefused`, success = answer contains the stub's IP. Subdomain test: list `example.com`, query `www.example.com`. Callback test: `onResolve` receives the stub's answer IP. Concurrency test: hammer queries from N goroutines while calling `Update` in a loop; every response is consistent (either old or new ruleset, never a mix).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/sentinel/filtering/...`
Expected: FAIL — `NewServer` undefined.

- [ ] **Step 3: Implement the server and plugin**

`plugin.go`: the Aegis plugin holds `atomic.Pointer[ruleSet]` where `ruleSet` is the compiled snapshot (policy type + domain set); `ServeDNS` checks the query name (exact or dot-suffix match against the set), answers `REFUSED` or delegates to the forward upstream, and — for allowed WHITELIST answers — calls `onResolve` per A/AAAA record. `server.go`: builds a minimal CoreDNS `dnsserver.Server` (one server block, the plugin first, `forward` to upstreams), `Start` runs it under `ctx`, `Update`/`Clear` swap the pointer.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/sentinel/filtering/...`
Expected: PASS.

- [ ] **Step 5: Stage for review**

```bash
git add internal/sentinel/filtering go.mod go.sum
# suggested message: "feat: enforce DOMAIN rules through an embedded CoreDNS"
```

### Task 5: Sentinel Configuration for Filtering

**Files:**
- Modify: `internal/sentinel/config/config.go`
- Modify: `internal/sentinel/config/config_test.go`

**Interfaces:**
- Consumes: existing `document`/`Load` machinery.
- Produces: `Config` gains `FilteringEnabled bool`, `DNSListenAddress string`, `DNSUpstreamServers []string`, `NftBinaryPath string`; JSON fields `filtering_enabled`, `dns_listen_address`, `dns_upstream_servers`, `nft_binary_path`.

- [ ] **Step 1: Write the failing tests**

Extend `config_test.go` (package `config_test`) with:

```go
func TestConfig_Load_WithFilteringDefaults_AppliesDefaults(t *testing.T)   // enabled=true, 127.0.0.1:53, upstream from /etc/resolv.conf, nft
func TestConfig_Load_WithFilteringOverrides_AppliesProvidedValues(t *testing.T)
func TestConfig_Load_WithInvalidDNSListenAddress_ReturnsError(t *testing.T)
func TestConfig_Load_WithInvalidDNSUpstream_ReturnsError(t *testing.T)
```

The defaults test injects a temporary `resolv.conf` (the loader accepts its path or the parsing is factored to take content — builder's choice) so it does not depend on the host file. Invalid upstream = non-IP entries.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/sentinel/config/...`
Expected: FAIL — unknown fields rejected by `DisallowUnknownFields`, new accessors missing.

- [ ] **Step 3: Implement the fields**

Follow the existing pattern exactly: add fields to `document`, parse functions with defaults (`filtering_enabled` parses with `strconv.ParseBool`; listen address via `net.SplitHostPort`; upstream via `net.ParseIP` per entry). Upstream default: parse `nameserver` lines from `/etc/resolv.conf` at load time.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/sentinel/config/...`
Expected: PASS.

- [ ] **Step 5: Stage for review**

```bash
git add internal/sentinel/config
# suggested message: "feat: configure Sentinel local filtering"
```

### Task 6: Composite Filter with Rollback

**Files:**
- Create: `internal/sentinel/filtering/composite.go`
- Test: `internal/sentinel/filtering/composite_test.go`

**Interfaces:**
- Consumes: `nftables.Executor` (Task 3), `dnsserver.Server` (Task 4), `filtering.PolicySnapshot` (Task 2).
- Produces:

```go
// Filter applies and clears the local packet and DNS filtering. It is the
// contract the propagation runner consumes.
type Filter interface {
    Apply(snapshot PolicySnapshot) error
    Clear() error
}

// Composite programs nftables first, then the resolver, rolling the resolver
// back if the DNS update fails.
func NewComposite(packets PacketFilter, resolver ResolverFilter) Filter
```

with `PacketFilter` and `ResolverFilter` narrow interfaces declared here and satisfied structurally by the executor (`Apply`/`Clear`/`AddResolvedIP`) and the server (`Update`/`Clear`).

- [ ] **Step 1: Write the failing tests**

Create `composite_test.go` in `package filtering_test` with mocks implementing the two interfaces:

```go
func TestComposite_Apply_WithSuccessfulBackends_AppliesPacketsThenResolver(t *testing.T)
func TestComposite_Apply_WithResolverFailure_RollsBackResolver(t *testing.T)
func TestComposite_Apply_WithPacketFailure_NeverTouchesResolver(t *testing.T)
func TestComposite_Clear_WithSuccessfulBackends_ClearsBoth(t *testing.T)
func TestComposite_Clear_WithResolverFailure_PacketClearIsNotUndone(t *testing.T)
```

Order assertions via recorded call sequences. Rollback: resolver failure calls `Clear` on the resolver mock (its previous set restored) and returns the error; packet state is untouched.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/sentinel/filtering`
Expected: FAIL — `NewComposite` undefined.

- [ ] **Step 3: Implement the composite**

`Apply`: `packets.Apply(snapshot)`; on error return it (fail open — nothing changed). Then `resolver.Update(snapshot)`; on error call `resolver.Clear()` and return a wrapped error. `Clear`: `packets.Clear()` then `resolver.Clear()`, joining errors. The `onResolve` wiring (server callback → `packets.AddResolvedIP`) happens in `cmd/sentinel` (Task 8), not here.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/sentinel/filtering`
Expected: PASS.

- [ ] **Step 5: Stage for review**

```bash
git add internal/sentinel/filtering
# suggested message: "feat: compose nftables and DNS filtering with rollback"
```

### Task 7: Propagation Runner Enforcement Flow

**Files:**
- Modify: `internal/sentinel/propagation/runner.go:49-63` (`Runner` fields and `New`)
- Modify: `internal/sentinel/propagation/apply.go`
- Modify: `internal/sentinel/propagation/apply_test.go`, `runner_test.go` (mocks + new cases)

**Interfaces:**
- Consumes: `filtering.Filter` and `filtering.PolicySnapshot` (Tasks 2/6), `internal/sentinel/config.Config` fields (Task 5, only at wiring time — not imported here).
- Produces: `New(store StateStore, connector Connector, renewer SessionRenewer, filter Filter, logger *slog.Logger) *Runner`. A `nil` filter means filtering is disabled.

- [ ] **Step 1: Write the failing tests**

Extend `apply_test.go` with a `filtering.Filter` mock (records calls; scriptable error). New/updated cases:

```go
func TestRunner_Apply_WithValidPackage_AppliesFilteringBeforePersistAndAcks(t *testing.T)
func TestRunner_Apply_WithFilterApplyFailure_NacksAndKeepsPreviousState(t *testing.T)
func TestRunner_Apply_WithNilFilter_NacksWithFilteringDisabled(t *testing.T)
func TestRunner_Apply_WithRevoke_ClearsFilteringAndAcks(t *testing.T)
func TestRunner_Apply_WithRevokeAndClearFailure_NacksAndKeepsPreviousState(t *testing.T)
```

"Applies before persist" is asserted by the mock call order (filter `Apply` recorded before `store.Save`). Fail-open cases assert the store was never called with the new state and the returned state is unchanged.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/sentinel/propagation/...`
Expected: FAIL — `New` signature mismatch in existing tests, new tests reference the mock's missing behavior.

- [ ] **Step 3: Implement the enforcement flow**

`apply()`: after building `next` and before `store.Save(next)` — if `runner.filter == nil`, nack `filtering disabled`; for `KindPackage` call `filter.Apply(snapshotFor(next))`; for `KindRevoke` call `filter.Clear()`. On filter error, nack `failed to apply filtering: <err>`. The snapshot maps `State` fields to `filtering.PolicySnapshot`; `OrchestratorIPs`/`UpstreamDNSIPs` are resolved by the caller (Task 8) and carried on `Runner` as fields, not recomputed here. Update all existing `New(...)` call sites in tests with a passing mock.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/sentinel/propagation/...`
Expected: PASS.

- [ ] **Step 5: Stage for review**

```bash
git add internal/sentinel/propagation
# suggested message: "feat: enforce accepted policies and ack only after application"
```

### Task 8: Sentinel Wiring

**Files:**
- Modify: `cmd/sentinel/main.go`
- Modify: `Makefile:5-21` (`TEST_PACKAGES`)

**Interfaces:**
- Consumes: everything from Tasks 2–7.
- Produces: a running Sentinel that filters; no new exported symbols.

- [ ] **Step 1: Extend `TEST_PACKAGES`**

Add `./internal/sentinel/filtering`, `./internal/sentinel/filtering/nftables`, `./internal/sentinel/filtering/dnsserver` to the `Makefile` variable so the coverage gate includes them.

- [ ] **Step 2: Wire `main.go`**

When `cfg.FilteringEnabled`: build `nftables.NewExecutor(cfg.NftBinaryPath)`, `dnsserver.NewServer(cfg.DNSListenAddress, cfg.DNSUpstreamServers, executor.AddResolvedIP)`, start the server in a `daemon.Task` named `dns`, and pass `filtering.NewComposite(executor, server)` to `propagation.New` — otherwise pass `nil`. Resolve `OrchestratorIPs` from `cfg.OrchestratorURL`/`cfg.PropagationAddress` host at startup (and re-resolve on every apply by rebuilding the snapshot in the wiring's snapshot provider — simplest: resolve once at startup and document the refresh as a builder choice already noted in the spec). Extend the `configuration loaded` log record with the filtering fields.

- [ ] **Step 3: Build and run the full suite**

Run: `make tests && make build`
Expected: PASS, both binaries build, no lint errors (`golangci-lint run` on touched files if available locally).

- [ ] **Step 4: Stage for review**

```bash
git add cmd/sentinel Makefile
# suggested message: "feat: wire local filtering into the Sentinel daemon"
```

### Task 9: End-to-End Filtering Verification

**Files:**
- Modify: `e2e/harness_test.go` (if the container needs privileges)
- Create: `e2e/filtering_e2e_test.go`

**Interfaces:**
- Consumes: the e2e harness (existing `propagation_e2e_test.go` patterns), the real binaries from `make build`.

- [ ] **Step 1: Write the failing test**

Following the existing e2e suite's arrange helpers: boot Orchestrator + Sentinel containers, link a whitelist policy to the machine's group (using the API helpers already proven in `propagation_e2e_test.go`), then assert, inside the Sentinel container:

```go
func TestFilteringE2E_WithWhitelistPackage_AppliesNftablesAndDNS(t *testing.T)
func TestFilteringE2E_WithRevoke_ClearsNftablesAndDNS(t *testing.T)
```

Assertions: `nft list table inet aegis` output contains the resolved accept lines; a DNS query for a listed domain answers, an unlisted one is refused; the machine's propagation state (via the Orchestrator API) is `ACKED`; after revoke, the table is gone and DNS forwards freely.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `make e2e`
Expected: FAIL — no filtering e2e file / table absent.

- [ ] **Step 3: Make the containers filter-capable**

Per the spec's builder choice: the Sentinel container needs `nft` installed (extend `e2e/Dockerfile`) and root/`NET_ADMIN` capabilities (Testcontainers `WithPrivileged` or the minimal capability set). Keep the delta to the existing harness as small as possible.

- [ ] **Step 4: Run the e2e suite to verify it passes**

Run: `make e2e && make tests`
Expected: PASS — full suite, including all prior e2e tests.

- [ ] **Step 5: Stage for review**

```bash
git add e2e
# suggested message: "feat: verify local filtering end to end"
```
