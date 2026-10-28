# E2E Testcontainers Suite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate the end-to-end suite from host processes to Testcontainers: the real Orchestrator and N Sentinel containers share a fixed-subnet Docker network, and the suite proves the full wire contract — fleet authentication, the machine `CONNECTED` flip, graceful stop, and wrong-key rejection.

**Architecture:** One multi-stage Dockerfile builds a single image containing both binaries; `TestMain` builds it once via the testcontainers Docker provider. The harness creates a user-defined bridge network with a fixed IPAM subnet (`172.30.0.0/24`), writes the accepted-machines registry from the *planned* static IPs before the Orchestrator starts (the registry sync is startup-only), and gives every container a static IP through `EndpointSettingsModifier`. Configuration files are injected with `WithFiles` from in-memory readers. Scenarios assert `GET /machines` status transitions, container exit codes, and captured container logs.

**Tech Stack:** Go `1.26.5`, `github.com/testcontainers/testcontainers-go` v0.44.0 (includes the `/wait` and `/network` packages), Docker, `chi`/SQLite untouched.

**Spec:** `docs/superpowers/specs/2026-10-22-e2e-testcontainers-design.md`

## Global Constraints

- Module path `dougdomingos.com/aegis`; Go `1.26.5`.
- The only new direct dependency is `github.com/testcontainers/testcontainers-go v0.44.0`. The import of `github.com/moby/moby/api/types/network` (for `EndpointSettings`) resolves through it; run `go mod tidy` after the harness compiles.
- Every file in `e2e/` keeps `//go:build e2e` and `package e2e`.
- `make e2e` keeps its exact command line (`go test -tags e2e -v ./e2e/...`); it now requires a running Docker daemon (29.x locally, preinstalled on `ubuntu-latest` CI runners).
- `e2e` stays OUT of `TEST_PACKAGES`; `make tests` and the 75% coverage gate must stay green and are unaffected.
- Every symbol carries a doc comment starting with its name; no doc comments on interface method implementations (AGENTS.md §4.2).
- Test names follow `Test<Type>_<Method>_<Scenario>_<ExpectedResult>`; assertions use plain `t.Errorf`/`t.Fatalf` with descriptive messages.
- **Never commit automatically.** Every commit step runs only when the developer has explicitly authorized it (AGENTS.md §6.1). Commit style: conventional commits with scope `e2e` (e.g. `test(e2e): ...`).

## Shared constants (defined once in `e2e/harness_test.go`, used by all tasks)

| Constant | Value |
| --- | --- |
| `networkSubnet` | `"172.30.0.0/24"` |
| `orchestratorIP` | `"172.30.0.10"` |
| `sentinelIP(i int)` | `fmt.Sprintf("172.30.0.%d", 11+i)` |
| `machineName(i int)` | `fmt.Sprintf("lcc1-%03d", i+1)` |
| `validLabKeyHex` | `"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"` (reused from the old suite) |
| `wrongLabKeyHex` | `"fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"` (reused) |
| `sessionSecret` | `"e2e-session-secret"` (reused) |
| `labKeyPath` | `"/etc/aegis/lab.key"` |
| `machinesFilePath` | `"/etc/aegis/machines.csv"` |
| `sentinelConfigPath` | `"/etc/aegis/sentinel.json"` |
| `readyTimeout` | `60 * time.Second` |
| `imageRepo` / `imageTag` | `"aegis-e2e"` / `"test"` |

## Review Focus

Failure modes the spec implies but no single task headline calls out; each line is pinned by a test in the owning task.

1. **The Sentinel's detected IP must equal its planned static IP.** Inside the container the default-route source address is the planned `172.30.0.x`; if identity detection resolved anything else, auth would 401 silently and the `CONNECTED` poll would time out with a confusing message. Pinned by Task 2: `waitForConnected` failures must print every machine's current status keyed by IP, and the wrong-key task proves the DISCONNECTED half of that contract.
2. **Signals must reach the binary as PID 1.** The image runs the binary directly (no shell wrapper, `CMD` is the executable); a shell entrypoint would swallow Docker's stop signal and turn a graceful stop into a SIGKILL/137. Pinned by Task 2: first stop yields exit code `0`, a `daemon stopping` record, and NO `forcing exit` record.
3. **Secrets must never appear in container logs.** The hex lab key and the session secret are asserted absent from every Sentinel and Orchestrator log capture. Pinned by Task 2 (secret scan helper shared with Task 3).
4. **The registry must be complete before the Orchestrator boots.** `SyncMachines` runs once at startup; a late-registered IP can never authenticate. The harness ordering (`newFleet` renders the CSV before `startOrchestrator` exists in the run) is the contract; pinned by Task 1 asserting all planned machines appear, and Task 3 asserting an authenticated fleet machine flips while an unauthenticated one stays `DISCONNECTED`.
5. **Host-side assertions must never assume a fixed host port.** GitHub runners and developer machines may have `:8080` occupied; every HTTP assertion goes through `Host` + `MappedPort`. Pinned by Task 1 (`machines()` helper).

---

### Task 1: Image, harness bootstrap, and Orchestrator readiness

**Files:**
- Create: `e2e/Dockerfile`
- Create: `.dockerignore`
- Create: `e2e/harness_test.go`
- Create: `e2e/fleet_e2e_test.go`
- Delete: `e2e/sentinel_auth_e2e_test.go`
- Modify: `go.mod`, `go.sum` (via `go get` / `go mod tidy`)

**Interfaces:**
- Consumes: nothing (first task).
- Produces (used by Tasks 2 and 3, exact names and signatures):
  - `var e2eImage string` — the built image reference (`repo:tag`), set by `TestMain`.
  - `type fleet struct { t *testing.T; network *testcontainers.DockerNetwork; machineCount int }`
  - `func newFleet(t *testing.T, machineCount int) *fleet` — creates the bridge network with the fixed IPAM subnet via `tcnetwork.New`; registers `testcontainers.CleanupNetwork` cleanup.
  - `func (f *fleet) startOrchestrator(t *testing.T) testcontainers.Container` — starts the Orchestrator container with `e2eImage`, the shared constants' env vars (`AEGIS_LAB_KEY`, `AEGIS_SESSION_SECRET`, `AEGIS_MACHINES`), static IP `orchestratorIP` on the fleet network, injected `lab.key` + `machines.csv` files, `ExposedPorts: ["8080/tcp"]`, `wait.ForHTTP("/machines").WithPort("8080/tcp")`; registers `Terminate` cleanup.
  - `func (f *fleet) machines(t *testing.T) map[string]schemas.MachineOutputSchema` — `GET /machines` against `Host(ctx)` + `MappedPort(ctx, "8080/tcp")`, decoded into `[]schemas.MachineOutputSchema`, keyed by `IP`.
  - `func injectedFile(containerPath, content string) testcontainers.ContainerFile` — `ContainerFile{Reader: strings.NewReader(content), ContainerFilePath: containerPath, FileMode: 0o644}` (0o644 because the distroless runtime user is nonroot; containers are ephemeral).
  - `func withStaticIP(networkName, ip string) testcontainers.CustomizeRequestOption` — `testcontainers.WithEndpointSettingsModifier` setting `IPAMConfig = &network.EndpointIPAMConfig{IPv4Address: netip.MustParseAddr(ip)}` for the named network (import `mobynetwork "github.com/moby/moby/api/types/network"`).
  - All shared constants from the table above.

- [ ] **Step 1: Add the dependency**

```bash
go get github.com/testcontainers/testcontainers-go@v0.44.0
```

- [ ] **Step 2: Create `e2e/Dockerfile`**

```dockerfile
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/aegis-orchestrator ./cmd/orchestrator \
 && CGO_ENABLED=0 go build -o /out/aegis-sentinel ./cmd/sentinel

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/aegis-orchestrator /usr/local/bin/aegis-orchestrator
COPY --from=build /out/aegis-sentinel /usr/local/bin/aegis-sentinel
CMD ["/usr/local/bin/aegis-orchestrator"]
```

`CGO_ENABLED=0` is safe: SQLite is the pure-Go `modernc.org/sqlite`. The binary is PID 1 (Review Focus 2) — no shell wrapper. `golang:1.26-alpine` must be able to build the `go 1.26.5` module; if the tag lacks that toolchain, pin the concrete patch tag instead.

- [ ] **Step 3: Create `.dockerignore`** at the repository root:

```
.git
.github
.superpowers
docs
bin
collections
coverage.out
sentinel.config.example.json
e2e/
```

(`e2e/` is excluded so the suite's own test binaries never invalidate the build cache.)

- [ ] **Step 4: Delete the old host-process suite**

Delete `e2e/sentinel_auth_e2e_test.go` — its `TestMain` cannot coexist with the harness's `TestMain`, and its reusable constants are re-declared in the table above. Its happy-path and wrong-key scenarios are covered by Tasks 2 and 3; the unreachable-Orchestrator and missing-config scenarios are explicitly deferred per the spec's out-of-scope list.

- [ ] **Step 5: Write the failing scenario** in `e2e/fleet_e2e_test.go`

```go
//go:build e2e

func TestFleetE2E_OrchestratorStart_RegistersConfiguredMachinesAsDisconnected(t *testing.T) {
	f := newFleet(t, 2)
	f.startOrchestrator(t)

	got := f.machines(t)
	for i := 0; i < 2; i++ {
		machine, ok := got[sentinelIP(i)]
		if !ok {
			t.Fatalf("machine %s is missing from the registry: %v", sentinelIP(i), got)
		}
		if machine.Name != machineName(i) {
			t.Errorf("machine %s name = %q, want %q", sentinelIP(i), machine.Name, machineName(i))
		}
		if machine.Status != "DISCONNECTED" {
			t.Errorf("machine %s status = %q, want DISCONNECTED", sentinelIP(i), machine.Status)
		}
	}
}
```

- [ ] **Step 6: Run to verify it fails**

Run: `go vet -tags e2e ./e2e/...`
Expected: compile error — `newFleet`, `startOrchestrator`, `machines` undefined.

- [ ] **Step 7: Implement `e2e/harness_test.go`** with exactly the Produces signatures above plus:

`TestMain(m *testing.M)`: `provider, err := testcontainers.NewDockerProvider()`; `defer provider.Close()`; build once:

```go
ref, err := provider.BuildImage(ctx, testcontainers.FromDockerfile{
	Context:    "..",
	Dockerfile: "e2e/Dockerfile",
	Repo:       imageRepo,
	Tag:        imageTag,
	KeepImage:  true,
})
```

set `e2eImage = ref` (fail via `fmt.Fprintf(os.Stderr, ...)` + `os.Exit(1)`), then `os.Exit(m.Run())`.

`newFleet`: network with `tcnetwork.New(ctx, tcnetwork.WithDriver("bridge"), tcnetwork.WithIPAM(&mobynetwork.IPAM{Driver: "default", Config: []mobynetwork.IPAMConfig{{Subnet: networkSubnet}}}))`.

`startOrchestrator`: `machines.csv` content is `"ip,name\n"` + one `ip,name` row per planned sentinel IP (rendered from `f.machineCount`); run via `testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{Image: e2eImage, ...}, Started: true})` with the options listed in Interfaces. The Orchestrator's `AEGIS_ADDR`/`AEGIS_DB` keep their defaults (`:8080`, `/tmp/aegis.db`); if the database file cannot be created under the nonroot user, add `AEGIS_DB=/aegis.db` to the env map instead.

- [ ] **Step 8: Run the suite**

Run: `go test -tags e2e -v -run TestFleetE2E_OrchestratorStart ./e2e/...`
Expected: PASS (first run pulls base images and the Ryuk reaper; subsequent runs reuse the Docker layer cache).

- [ ] **Step 9: Tidy and verify the unit gate**

Run: `go mod tidy && make tests`
Expected: tidy adds `github.com/testcontainers/testcontainers-go` and the moby api types to `go.mod`; unit tests still pass.

- [ ] **Step 10: Commit (on developer authorization)**

```bash
git add e2e/ .dockerignore go.mod go.sum
git commit -m "test(e2e): run the suite on Testcontainers with a fixed-subnet fleet network"
```

---

### Task 2: Fleet happy path — authentication, CONNECTED flip, graceful stop, secret scan

**Files:**
- Create: `e2e/secretscan_e2e_test.go`
- Modify: `e2e/harness_test.go`
- Modify: `e2e/fleet_e2e_test.go`

**Interfaces:**
- Consumes: Task 1's `newFleet`, `startOrchestrator`, `machines`, constants, `withStaticIP`, `injectedFile`.
- Produces (used by Task 3):
  - `type sentinelHandle struct { container testcontainers.Container; ip, name string }`
  - `func (f *fleet) startSentinel(t *testing.T, index int, labKeyHex string) *sentinelHandle` — runs `e2eImage` with entrypoint override `["/usr/local/bin/aegis-sentinel", "--config", sentinelConfigPath]`, static IP `sentinelIP(index)`, `WithFiles` of the lab key (`labKeyHex`) and `sentinel.json` rendered as `{"orchestrator_url": "http://<orchestratorIP>:8080", "lab_key_path": "/etc/aegis/lab.key"}`. No wait strategy (the process may exit on purpose); `Started: true`; `Terminate` cleanup.
  - `func (h *sentinelHandle) logs(t *testing.T) string` — full `Logs(ctx)` capture (works while running and after exit).
  - `func (h *sentinelHandle) waitForLog(t *testing.T, needle string, timeout time.Duration)` — poll `logs` every 100ms; `t.Fatalf` on deadline with the log contents.
  - `func (h *sentinelHandle) stopAndExitCode(t *testing.T) int` — `Stop(ctx, &gracefulStopTimeout)` where `gracefulStopTimeout = 10 * time.Second` (Docker sends SIGTERM; Review Focus 2), then poll `State(ctx)` until `!Running` (deadline 30s), return `Inspect(ctx).State.ExitCode`.
  - `func (f *fleet) waitForConnected(t *testing.T, ips []string, timeout time.Duration)` — poll `f.machines` every 500ms until every IP shows `Status == "CONNECTED"`; the timeout message must print the full status map (Review Focus 1).
  - `func assertNoSecrets(t *testing.T, output string)` — `t.Errorf` if `output` contains `validLabKeyHex`, `wrongLabKeyHex`, or `sessionSecret` (Review Focus 3).

- [ ] **Step 1: Write the failing scenario** in `e2e/fleet_e2e_test.go`

`TestFleetE2E_ValidLabKeys_FleetAuthenticatesFlipsConnectedAndStopsGracefully`:
`f := newFleet(t, 3)`; `orchestrator := f.startOrchestrator(t)`; loop `i := 0..2` → `sentinels[i] = f.startSentinel(t, i, validLabKeyHex)`; each `waitForLog(t, "daemon running", readyTimeout)`; `f.waitForConnected(t, []string{sentinelIP(0), sentinelIP(1), sentinelIP(2)}, readyTimeout)`; then per sentinel assert its logs contain `"machine authenticated"`, `"machine identity detected"`, `"configuration loaded"` and `"machine=" + machineName(i)`; `assertNoSecrets` on each Sentinel's logs; `stopAndExitCode` must return `0`; logs must contain `"daemon stopping"` and must NOT contain `"forcing exit"`; `assertNoSecrets` on the Orchestrator's logs (`testcontainers` `Logs(ctx)` on the orchestrator container).

- [ ] **Step 2: Run to verify it fails**

Run: `go vet -tags e2e ./e2e/...`
Expected: compile error — `startSentinel`/`waitForConnected` undefined.

- [ ] **Step 3: Implement the Sentinel helpers and `waitForConnected`** in `e2e/harness_test.go` per the Produces signatures. The `WithFiles` lab-key content is `labKeyHex + "\n"` (the config loader trims whitespace and accepts a trailing newline). `stopAndExitCode`/`waitForExitCode` must tolerate a nil `ProcessState`: read exit code from `Inspect(ctx).State.ExitCode`, not from an error value.

- [ ] **Step 4: Add the golden-log package test** in `e2e/secretscan_e2e_test.go`

`TestSecretScan_SampleOutput_WithSecrets_FailsAssertions` — a table of the three secret constants embedded in sample strings; assert `assertNoSecrets` flags each (a negative-control test of the helper itself, guarding against silently passing assertions if the needles ever go empty; runs as part of `make e2e` since it shares the build tag).

- [ ] **Step 5: Run the suite**

Run: `go test -tags e2e -v -run TestFleetE2E_ValidLabKeys ./e2e/...`
Expected: PASS. If the CONNECTED poll times out while the Sentinel logged `machine authenticated`, the container's detected IP does not match the planned static IP — diagnose with `docker inspect` (Review Focus 1); the correct fix is the network/IP plan, not a registry hack.

- [ ] **Step 6: Unit gate**

Run: `make tests`
Expected: PASS.

- [ ] **Step 7: Commit (on developer authorization)**

```bash
git add e2e/
git commit -m "test(e2e): prove the sentinel fleet authenticates, flips machines connected and stops gracefully"
```

---

### Task 3: Wrong-key rejection inside a live fleet

**Files:**
- Modify: `e2e/fleet_e2e_test.go`

**Interfaces:**
- Consumes: Task 2's `startSentinel`, `waitForConnected`, `assertNoSecrets`, `logs`.
- Produces:
  - `func (h *sentinelHandle) waitForExitCode(t *testing.T, want int, timeout time.Duration)` — poll `State(ctx)` until `!Running` (100ms interval), assert the inspected `ExitCode == want`; the timeout/failure message includes `h.logs(t)`.

- [ ] **Step 1: Write the failing scenario**

`TestFleetE2E_WrongLabKey_SentinelExitsRejectedWhileFleetStaysConnected`:
`f := newFleet(t, 3)`; `f.startOrchestrator(t)`; Sentinels at index 0 and 1 with `validLabKeyHex`; index 2 with `wrongLabKeyHex`; index 2 → `waitForExitCode(t, 1, readyTimeout)`; assert its logs contain `"authentication rejected by the orchestrator"` (the client's fatal error text from `cmd/sentinel/main.go`'s `daemon stopped with a fatal error` + the rejection wrapper — assert on `strings.Contains` of the rejection sentinel's message, matching the old suite's needle); assert `f.machines(t)` shows `sentinelIP(0)` and `sentinelIP(1)` `CONNECTED` and `sentinelIP(2)` `DISCONNECTED`; assert no secrets in the rejected Sentinel's logs (the wrong key must not be echoed either).

- [ ] **Step 2: Run to verify it fails**

Run: `go test -tags e2e -v -run TestFleetE2E_WrongLabKey ./e2e/...`
Expected: FAIL or compile error until the assertions are written; implement the test fully, then:

- [ ] **Step 3: Run the full suite**

Run: `go test -tags e2e -v ./e2e/...`
Expected: all three fleet tests PASS in one run (each test gets its own network and containers; Ryuk cleans up between).

- [ ] **Step 4: Unit gate**

Run: `make tests`
Expected: PASS.

- [ ] **Step 5: Commit (on developer authorization)**

```bash
git add e2e/
git commit -m "test(e2e): cover wrong-key rejection inside the live fleet topology"
```

---

### Task 4: CI job, Makefile wording, and docs

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `Makefile:40` (`e2e` help text)
- Modify: `AGENTS.md` (the `e2e` target line under "Tools and Environment")

**Interfaces:**
- Consumes: nothing.
- Produces: nothing.

- [ ] **Step 1: Add the `e2e` job to `.github/workflows/ci.yml`** as a sibling of `test` (runs in parallel):

```yaml
  e2e:
    name: Run E2E Suite (Testcontainers)
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Install dependencies
        uses: actions/setup-go@v5
        with:
          go-version-file: "go.mod"
          cache: true

      - name: Run end-to-end suite
        run: make e2e
```

Do not change the existing `test` job. Keep the workflow-level `on:` paths filters as they are — they already cover `**.go`, `go.mod`, `go.sum` and this file; add nothing else.

- [ ] **Step 2: Update the Makefile help text** on line 41's comment above `e2e:`:

```
e2e: ## Run the end-to-end suite with Testcontainers (requires Docker)
```

- [ ] **Step 3: Update the AGENTS.md row** describing `e2e` to match: "Run the end-to-end suite with Testcontainers against the real binaries in containers (requires Docker)".

- [ ] **Step 4: Verify locally**

Run: `make help && make e2e`
Expected: help lists the new text; the full suite passes in one run.

- [ ] **Step 5: Commit (on developer authorization)**

```bash
git add .github/workflows/ci.yml Makefile AGENTS.md
git commit -m "ci(e2e): run the testcontainers suite on pull requests"
```
