# Sentinel Daemon — First Implementation — Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Deliver the first Sentinel daemon: a long-lived process that loads its configuration, detects its machine identity, authenticates against the Orchestrator at startup, keeps running until it is signalled, and logs every operation.

**Architecture:** A new `internal/sentinel/` subtree, wired by a thin `cmd/sentinel` entrypoint. The packages are `config` (JSON file), `identity` (IP/MAC auto-detection), `client` (Orchestrator challenge-response), `daemon` (lifecycle and fatal-error propagation) and `logging` (slog setup). The daemon owns the `IdentityDetector` and `Authenticator` interfaces it consumes; `identity.Detector` and `client.Client` satisfy them structurally. `internal/auth` and `internal/schemas` are reused unchanged.

**Tech Stack:** Go `1.26.5`, standard library only (`log/slog`, `net/http`, `encoding/json`, `net`), `chi` is not involved (the Sentinel is an API client).

**Spec:** `docs/superpowers/specs/2026-10-22-sentinel-daemon-design.md`

## Global Constraints

- Module path `dougdomingos.com/aegis`; Go `1.26.5`.
- No new third-party dependencies — standard library only.
- Every symbol carries a doc comment starting with its name, per Go convention. Doc comments on interfaces, but **not** on their concrete implementations.
- Interfaces are owned by their consumer. `daemon` declares `IdentityDetector` and `Authenticator`; `client` and `identity` implement them.
- Tests live beside their target in an external package (`package x_test`). A white-box test that needs unexported state uses `package x` in a file suffixed `_internal_test.go` (the precedent is `internal/store/nonce_store_internal_test.go`).
- Test names follow `Test<Type>_<Method>_<Scenario>_<ExpectedResult>`. Assertions use plain `t.Errorf`/`t.Fatalf` with descriptive messages. Grouped validations of one scenario become table-driven tests.
- Package-level sentinel errors live in `internal/errors`, in a file named after the subsystem. Test files alias that package as `<subsystem>Errors`.
- The Sentinel must not import `internal/api`, `internal/service` or `internal/store`.
- No schema change: no migration is created or modified.
- **Never commit automatically.** Every commit step below runs only when the developer has explicitly authorized it (AGENTS.md §6.1).
- Branch `feat/issue-10b-sentinel-daemon`. One commit per task, one pull request at the end.

## Review Focus

Each line is a failure mode the spec implies but no stated requirement calls out. Every line is pinned by a test in the task that owns the code.

1. **The detected IP must be the one the Orchestrator has configured.** A mismatch must surface as an error that names the detected address, not as a bare `401`.
2. **The lab key file may carry a trailing newline and uppercase hex.** Both must be accepted; a key file written by hand must not fail the boot.
3. **An `orchestrator_url` ending in `/` must not double the slash** in the request path — the request must hit `/machines/authenticate/init` exactly.
4. **A non-JSON response body** (an HTML error page from a proxy) must produce a decoding error, never a panic.
5. **A multi-homed machine** (`docker0`, `br-*`, `virbr0` present) must resolve to the default-route address, never to a container bridge.

---

### Task 1: Sentinel logging package

**Files:**
- Create: `internal/sentinel/logging/logging.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `logging.Format` (a `string` type), `logging.FormatText`, `logging.FormatJSON`, `logging.Config{Level slog.Level; Format Format}`, `logging.New(w io.Writer, cfg Config) *slog.Logger`, `logging.ParseLevel(raw string) (slog.Level, error)`, `logging.ParseFormat(raw string) (Format, error)`, `logging.Fingerprint(secret string) string`.

- [x] **Step 1: Create `internal/sentinel/logging/logging.go`**

Package doc comment: `// Package logging configures the structured logger used by the Sentinel daemon.`

Declare, each with a doc comment:

```go
type Format string

const (
    FormatText Format = "text"
    FormatJSON Format = "json"
)

type Config struct {
    Level  slog.Level
    Format Format
}
```

- [x] **Step 2: Implement `ParseLevel` and `ParseFormat`**

`ParseLevel(raw string) (slog.Level, error)` accepts `debug`, `info`, `warn` and `error`, case-insensitively, and returns `slog.LevelDebug`, `slog.LevelInfo`, `slog.LevelWarn` and `slog.LevelError`. Any other value returns an error naming the accepted values. Trim surrounding whitespace before matching.

`ParseFormat(raw string) (Format, error)` accepts `text` and `json`, case-insensitively, returning `FormatText` and `FormatJSON`. Any other value returns an error naming the accepted values. Trim surrounding whitespace before matching.

- [x] **Step 3: Implement `New`**

`New(w io.Writer, cfg Config) *slog.Logger` returns `slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: cfg.Level}))` for `FormatText` and `slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: cfg.Level}))` for `FormatJSON`. An unrecognized format falls back to the text handler.

- [x] **Step 4: Implement `Fingerprint`**

`Fingerprint(secret string) string` returns the first eight hexadecimal characters of the SHA-256 digest of `secret`. It is used to correlate a session token across log records without revealing it.

- [x] **Step 5: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: no output.

- [x] **Step 6: Commit (only if authorized)**

```bash
git add internal/sentinel/logging/logging.go
git commit -m "feat(sentinel): add structured logging package"
```

> This task ships without a test file, per the agreed scope in spec §11. Its verification is the build plus the smoke check in Task 3.

---

### Task 2: Sentinel daemon package

**Files:**
- Create: `internal/sentinel/daemon/daemon.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `daemon.Task{Name string; Run func(ctx context.Context) error}`, `daemon.Session{Token string; ExpiresAt time.Time; MachineName string}`, `daemon.Daemon`, `daemon.New(logger *slog.Logger, tasks ...Task) *Daemon`, `(d *Daemon) Run(ctx context.Context) error`.

- [x] **Step 1: Create `internal/sentinel/daemon/daemon.go`**

Package doc comment: `// Package daemon implements the lifecycle of the Sentinel daemon.`

Declare `Task` with a doc comment on the type and on both fields: `Name` identifies the task in log records, and `Run` executes the task until the context is cancelled; a non-nil error is fatal.

Declare `Session` with a doc comment on the type and on each field: `Token` is the session token issued by the Orchestrator, `ExpiresAt` is the moment it expires, and `MachineName` is the name the Orchestrator assigned to the machine. Task 7 returns this type, so it is declared here rather than later.

Declare `Daemon` with an unexported `logger *slog.Logger` and `tasks []Task`.

- [x] **Step 2: Implement `New`**

`New(logger *slog.Logger, tasks ...Task) *Daemon` returns a daemon holding the logger and the tasks.

- [x] **Step 3: Implement `Run`**

`Run(ctx context.Context) error` performs, in order:

1. Log `"daemon starting"` at info level.
2. Start each task in its own goroutine. A task that returns an error sends `fmt.Errorf("task %q failed: %w", task.Name, err)` to a buffered channel sized `len(d.tasks)`. A task that returns `nil` sends nothing.
3. Log `"daemon running"` at info level with the attribute `tasks` set to `len(d.tasks)`.
4. Block on a `select` over `ctx.Done()` and the error channel. On `ctx.Done()`, log `"daemon stopping"` at info level and return `nil`. On the error channel, return the received error unchanged.

The daemon must not return when a task finishes successfully: only a context cancellation or a task error ends it.

- [x] **Step 4: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: no output.

> This package has no observable behaviour until Task 3 wires it into the entrypoint; its verification is the build here and the signal smoke test in Task 3.

- [x] **Step 5: Commit (only if authorized)**

```bash
git add internal/sentinel/daemon/daemon.go
git commit -m "feat(sentinel): add daemon lifecycle package"
```

---

### Task 3: Sentinel entrypoint

**Files:**
- Create: `cmd/sentinel/main.go`

**Interfaces:**
- Consumes: `daemon.New(logger *slog.Logger, tasks ...Task) *Daemon`, `daemon.Run(ctx context.Context) error`, `logging.New(w io.Writer, cfg Config) *slog.Logger`, `logging.Config{Level slog.Level; Format Format}`.
- Produces: the `sentinel` binary.

- [x] **Step 1: Create `cmd/sentinel/main.go`**

Package doc comment: `// Command sentinel runs the Aegis Sentinel daemon, the per-endpoint agent that authenticates with the Orchestrator and enforces the policies it distributes.`

`main` calls `os.Exit(run())`.

`run() int` performs, in order:

1. Build the bootstrap logger: `logging.New(os.Stderr, logging.Config{Level: slog.LevelInfo, Format: logging.FormatText})`.
2. Build the signal context via `notifySignals(logger)`.
3. Run `daemon.New(logger).Run(ctx)`. On error, log `"daemon stopped with a fatal error"` at error level with the attribute `error`, and return `1`. On success, return `0`.

- [x] **Step 2: Implement `notifySignals`**

`notifySignals(logger *slog.Logger) (context.Context, func())` returns a context and a stop function:

1. Derive a cancellable context from `context.Background()`.
2. Register `os.Interrupt` and `syscall.SIGTERM` on a buffered channel of capacity `2` via `signal.Notify`.
3. In a goroutine, wait for the first signal: log `"received signal, shutting down"` at info level with the attribute `signal`, then start a second goroutine that waits for another signal, logs `"received second signal, forcing exit"` at error level and calls `os.Exit(1)`, then cancel the context.
4. The stop function calls `signal.Stop` and `cancel`.

- [x] **Step 3: Build the binary**

Run: `go build -o /tmp/aegis-sentinel ./cmd/sentinel && go vet ./...`
Expected: no output; `/tmp/aegis-sentinel` exists.

- [x] **Step 4: Smoke test the signal handling**

```bash
/tmp/aegis-sentinel > /tmp/sentinel-smoke.log 2>&1 &
SENTINEL_PID=$!
sleep 1
kill -TERM $SENTINEL_PID
wait $SENTINEL_PID; echo "exit code: $?"
cat /tmp/sentinel-smoke.log
```

Expected: the exit code is `0`, and the log contains `daemon starting`, `daemon running`, `received signal, shutting down` and `daemon stopping`.

- [x] **Step 5: Smoke test the forced exit**

```bash
/tmp/aegis-sentinel > /tmp/sentinel-smoke2.log 2>&1 &
SENTINEL_PID=$!
sleep 1
kill -TERM $SENTINEL_PID
kill -TERM $SENTINEL_PID
wait $SENTINEL_PID; echo "exit code: $?"
cat /tmp/sentinel-smoke2.log
```

Expected: the exit code is `1`, and the log contains `received second signal, forcing exit`.

- [x] **Step 6: Commit (only if authorized)**

```bash
git add cmd/sentinel/main.go
git commit -m "feat(sentinel): add daemon entrypoint with signal handling"
```

---

### Task 4: Shared lab key reader

**Files:**
- Create: `internal/auth/labkey.go`
- Modify: `internal/config/config.go` — the `parseLabKey` function and the `labKeyLength` constant.

**Interfaces:**
- Consumes: nothing.
- Produces: `auth.LabKeyLength` (an untyped constant `32`) and `auth.ReadLabKey(path string) ([]byte, error)`.

- [x] **Step 1: Create `internal/auth/labkey.go`**

Package `auth` already exists; add to it:

```go
// LabKeyLength is the required length, in bytes, of the shared lab key.
const LabKeyLength = 32

// ReadLabKey reads the shared lab key from the file at the provided path. The
// file must hold a hex-encoded value that decodes to exactly LabKeyLength
// bytes. Surrounding whitespace is ignored and uppercase hex is accepted.
func ReadLabKey(path string) ([]byte, error)
```

The implementation trims the file contents, decodes them as hex, and returns these errors, in this order:

- `fmt.Errorf("failed to read key file: %w", err)`
- `fmt.Errorf("key file must be hex-encoded: %w", err)`
- `fmt.Errorf("key file must decode to exactly %d bytes", LabKeyLength)`

- [x] **Step 2: Delegate from `internal/config`**

In `internal/config/config.go`, delete the `labKeyLength` constant and replace the body of `parseLabKey` so that it keeps its current contract — an empty path is still `fmt.Errorf("%s is required", envLabKey)`, and a failure is still wrapped as `fmt.Errorf("invalid %s: %w", envLabKey, err)` — but delegates the parsing itself:

```go
func parseLabKey(path string) ([]byte, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, fmt.Errorf("%s is required", envLabKey)
	}

	key, err := auth.ReadLabKey(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", envLabKey, err)
	}

	return key, nil
}
```

Add the import `"dougdomingos.com/aegis/internal/auth"`.

- [x] **Step 3: Build and run the existing tests**

Run: `go build ./... && go test ./internal/config ./internal/auth`
Expected: PASS. The existing config tests assert only that an error occurred, so the delegation keeps them green.

- [x] **Step 4: Commit (only if authorized)**

```bash
git add internal/auth/labkey.go internal/config/config.go
git commit -m "refactor(auth): extract shared lab key reader"
```

---

### Task 5: Sentinel configuration

**Files:**
- Create: `internal/sentinel/config/config.go`
- Test: `internal/sentinel/config/config_test.go`
- Modify: `Makefile` — add the package to `TEST_PACKAGES`.

**Interfaces:**
- Consumes: `auth.ReadLabKey(path string) ([]byte, error)`, `logging.ParseLevel(raw string) (slog.Level, error)`, `logging.ParseFormat(raw string) (Format, error)`.
- Produces: `config.Config{OrchestratorURL *url.URL; LabKey []byte; RequestTimeout time.Duration; LogLevel slog.Level; LogFormat logging.Format}`, `config.Load(path string) (*Config, error)`.

- [x] **Step 1: Write the failing test file**

Create `internal/sentinel/config/config_test.go` in package `config_test`. Declare, at the top of the file:

```go
// validLabKey is a 32-byte key encoded as 64 uppercase hexadecimal
// characters, to pin that uppercase hex and a trailing newline are accepted.
const validLabKey = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF"

// labKeyFile is the contents of a valid lab key file.
const labKeyFile = validLabKey + "\n"
```

Declare these test functions:

- `TestConfig_Load_WithValidFile_ReturnsParsedConfig` — writes a configuration file whose `lab_key_path` points at a lab key file holding `labKeyFile`; asserts `cfg.OrchestratorURL.String()` is `http://10.0.0.5:8080`, `cfg.LabKey` equals the 32 decoded bytes, `cfg.RequestTimeout` is `10 * time.Second`, `cfg.LogLevel` is `slog.LevelInfo` and `cfg.LogFormat` is `logging.FormatText`. This is the test that pins Review Focus item 2.
- `TestConfig_Load_WithOptionalFields_ReturnsConfiguredValues` — sets `request_timeout` to `5s`, `log_level` to `debug` and `log_format` to `json`; asserts the three parsed values.
- `TestConfig_Load_WithMissingFile_ReturnsError` — asserts a non-nil error for a path under `t.TempDir()` that does not exist.
- `TestConfig_Load_WithMalformedJSON_ReturnsError` — asserts a non-nil error for the content `{`.
- `TestConfig_Load_WithUnknownField_ReturnsError` — asserts a non-nil error for a document containing `"unknown_field": true`.
- `TestConfig_Load_WithInvalidOrchestratorURL_ReturnsError` — table-driven with the cases `empty`, `malformed` (`"://bad"`), `unsupported scheme` (`"ftp://10.0.0.5"`) and `missing host` (`"http://"`); each case asserts a non-nil error.
- `TestConfig_Load_WithInvalidLabKey_ReturnsError` — table-driven with the cases `empty path` (no `lab_key_path` key), `missing file` (a path under `t.TempDir()` that does not exist), `non-hex` (a file containing `not-hex`) and `wrong size` (a file containing `0123456789abcdef`); each case asserts a non-nil error.
- `TestConfig_Load_WithInvalidRequestTimeout_ReturnsError` — table-driven with the cases `malformed` (`"soon"`), `zero` (`"0s"`) and `negative` (`"-1s"`); each case asserts a non-nil error.
- `TestConfig_Load_WithInvalidLogLevel_ReturnsError` — asserts a non-nil error for `"log_level": "verbose"`.
- `TestConfig_Load_WithInvalidLogFormat_ReturnsError` — asserts a non-nil error for `"log_format": "xml"`.

Declare the helper at the end of the file:

```go
// writeTempFile writes the provided contents to a file in a temporary
// directory and returns its path.
func writeTempFile(t *testing.T, contents string) string

// writeTempConfig writes the provided JSON document to a file in a temporary
// directory and returns its path.
func writeTempConfig(t *testing.T, document string) string
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/sentinel/config`
Expected: FAIL — `no required module provides package dougdomingos.com/aegis/internal/sentinel/config` or `undefined: config.Load`.

- [x] **Step 3: Implement `internal/sentinel/config/config.go`**

Package doc comment: `// Package config loads and validates the Sentinel daemon configuration.`

Declare unexported constants:

```go
const (
    defaultRequestTimeout = 10 * time.Second
    defaultLogLevel       = "info"
    defaultLogFormat      = "text"
)
```

Declare the exported `Config` struct, each field documented: `OrchestratorURL *url.URL`, `LabKey []byte`, `RequestTimeout time.Duration`, `LogLevel slog.Level`, `LogFormat logging.Format`.

Declare an unexported `document` struct mirroring the JSON file, with the tags `orchestrator_url`, `lab_key_path`, `request_timeout`, `log_level` and `log_format`.

- [x] **Step 4: Implement `Load`**

`Load(path string) (*Config, error)` performs these steps in order, returning the first error it meets:

1. Read the file at `path`; on failure return `fmt.Errorf("failed to read configuration file: %w", err)`.
2. Decode it with `json.NewDecoder` and `DisallowUnknownFields()`; on failure return `fmt.Errorf("failed to parse configuration file: %w", err)`.
3. Trim `orchestrator_url`; if empty return `fmt.Errorf("orchestrator_url is required")`. Parse it with `url.Parse`; on failure return `fmt.Errorf("invalid orchestrator_url: %w", err)`. The scheme must be `http` or `https`, otherwise return `fmt.Errorf("orchestrator_url must use the http or https scheme")`. The host must be non-empty, otherwise return `fmt.Errorf("orchestrator_url must include a host")`.
4. Trim `lab_key_path`; if empty return `fmt.Errorf("lab_key_path is required")`. Call `auth.ReadLabKey`; on failure return `fmt.Errorf("invalid lab_key_path: %w", err)`.
5. If `request_timeout` is empty, use `defaultRequestTimeout`. Otherwise parse it with `time.ParseDuration`; on failure return `fmt.Errorf("invalid request_timeout: %w", err)`. A value that is not positive returns `fmt.Errorf("invalid request_timeout: must be positive")`.
6. If `log_level` is empty, use `defaultLogLevel`. Otherwise call `logging.ParseLevel`; on failure return `fmt.Errorf("invalid log_level: %w", err)`.
7. If `log_format` is empty, use `defaultLogFormat`. Otherwise call `logging.ParseFormat`; on failure return `fmt.Errorf("invalid log_format: %w", err)`.
8. Return the assembled `Config`.

- [x] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/sentinel/config`
Expected: PASS.

- [x] **Step 6: Add the package to the coverage profile**

In `Makefile`, add `./internal/sentinel/config \` to `TEST_PACKAGES`, keeping the list's existing indentation.

- [x] **Step 7: Verify the whole suite**

Run: `make tests`
Expected: PASS.

- [x] **Step 8: Commit (only if authorized)**

```bash
git add internal/sentinel/config/config.go internal/sentinel/config/config_test.go Makefile
git commit -m "feat(sentinel): add configuration loading and validation"
```

---

### Task 6: Sentinel identity detection

**Files:**
- Create: `internal/sentinel/identity/identity.go`
- Test: `internal/sentinel/identity/identity_internal_test.go` — package `identity`, because the test must inject the detector's unexported function fields.

**Interfaces:**
- Consumes: nothing.
- Produces: `identity.Identity{IP string; MAC string}`, `identity.Detector`, `identity.NewDetector() *Detector`, `(d *Detector) Detect() (Identity, error)`.

- [x] **Step 1: Write the failing test file**

Create `internal/sentinel/identity/identity_internal_test.go` in package `identity`. Declare these test functions:

- `TestDetector_Detect_WithDefaultRouteAvailable_ReturnsPrimaryAddress` — the stubbed interface list holds `docker0` (up, IPv4 `172.17.0.1`, MAC `02:42:aa:bb:cc:dd`) and `eth0` (up, IPv4 `10.0.1.11`, MAC `aa:bb:cc:dd:ee:ff`); the stubbed route resolver returns `10.0.1.11`. Asserts the result is `Identity{IP: "10.0.1.11", MAC: "aa:bb:cc:dd:ee:ff"}`. This is the test that pins Review Focus item 5.
- `TestDetector_Detect_WithDefaultRouteUnavailable_FallsBackToFirstUsableInterface` — the route resolver returns an error; the interface list holds `lo` (loopback, up), `eth0` (down, IPv4 `10.0.1.11`) and `eth1` (up, IPv4 `10.0.1.12`, MAC `aa:bb:cc:dd:ee:00`). Asserts the result is `Identity{IP: "10.0.1.12", MAC: "aa:bb:cc:dd:ee:00"}`.
- `TestDetector_Detect_WithPrimaryIPOutsideLocalInterfaces_FallsBackToInterface` — the route resolver returns `192.0.2.7`, which belongs to no listed interface; the interface list holds `eth0` (up, IPv4 `10.0.1.11`). Asserts the result is `Identity{IP: "10.0.1.11", MAC: <eth0's MAC>}`.
- `TestDetector_Detect_WithNoUsableIPv4_ReturnsError` — table-driven with the cases `only loopback`, `only IPv6` and `all interfaces down`; each case asserts a non-nil error.
- `TestDetector_Detect_WithInterfaceWithoutHardwareAddress_ReturnsEmptyMAC` — the interface has an empty `HardwareAddr`; asserts the IP is resolved and the MAC is empty.
- `TestDetector_Detect_WithInterfaceListingError_ReturnsError` — the stubbed interface lister returns an error; asserts a non-nil error.

Declare the helper at the end of the file:

```go
// fakeInterface builds a net.Interface with the provided flags, name, address
// and hardware address.
func fakeInterface(name string, flags net.Flags, ip string, mac string) net.Interface
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/sentinel/identity`
Expected: FAIL — `undefined: Detector`.

- [x] **Step 3: Implement `internal/sentinel/identity/identity.go`**

Package doc comment: `// Package identity detects the network identity of the Sentinel host.`

Declare `Identity` with documented fields `IP` (the address that identifies the machine to the Orchestrator) and `MAC` (the hardware address of the interface that owns the IP).

Declare `Detector` with two unexported function fields: `interfaces func() ([]net.Interface, error)` and `primaryIP func() (net.IP, error)`.

`NewDetector() *Detector` returns a detector whose `interfaces` is `net.Interfaces` and whose `primaryIP` is `defaultPrimaryIP`.

- [x] **Step 4: Implement `defaultPrimaryIP`**

`defaultPrimaryIP() (net.IP, error)` opens a UDP connection to `8.8.8.8:80` — which sends no packet — and returns the local address of the resulting socket as a `net.IP`. It returns an error when the dial fails or the local address is not a `*net.UDPAddr`.

- [x] **Step 5: Implement `Detect`**

`Detect() (Identity, error)` performs these steps:

1. List the interfaces; on failure return `fmt.Errorf("failed to list network interfaces: %w", err)`.
2. Ask the route resolver for the primary address. If it succeeds and a listed interface owns that address, return the identity built from it.
3. Otherwise, walk the interfaces in order and take the first one that is up, is not loopback and has an IPv4 address. Return the identity built from it.
4. If no such interface exists, return `fmt.Errorf("no usable IPv4 address found")`.

Building an identity means the interface's IPv4 address rendered as a string and the interface's `HardwareAddr.String()`, which is empty when the interface has no hardware address.

- [x] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/sentinel/identity`
Expected: PASS.

- [x] **Step 7: Add the package to the coverage profile**

In `Makefile`, add `./internal/sentinel/identity \` to `TEST_PACKAGES`.

- [x] **Step 8: Verify the whole suite**

Run: `make tests`
Expected: PASS.

- [x] **Step 9: Commit (only if authorized)**

```bash
git add internal/sentinel/identity/identity.go internal/sentinel/identity/identity_internal_test.go Makefile
git commit -m "feat(sentinel): add machine identity detection"
```

---

### Task 7: Orchestrator authentication client

**Files:**
- Create: `internal/errors/sentinel.go`
- Create: `internal/sentinel/client/client.go`
- Test: `internal/sentinel/client/client_test.go`
- Modify: `Makefile` — add the package to `TEST_PACKAGES`.

**Interfaces:**
- Consumes: `auth.CanonicalPayload(ip, nonce string) []byte`, `auth.Sign(secret, payload []byte) []byte`, `schemas.InitiateAuthenticationSchema`, `schemas.InitiateOutputSchema`, `schemas.AuthenticateMachineSchema`, `schemas.AuthenticateOutputSchema`, `identity.Identity`, `daemon.Session`.
- Produces: `errors.ErrAuthenticationRejected`, `errors.ErrOrchestratorUnreachable`, `client.Client`, `client.New(baseURL *url.URL, labKey []byte, timeout time.Duration, logger *slog.Logger) *Client`, `(c *Client) Authenticate(ctx context.Context, id identity.Identity) (*daemon.Session, error)`.

- [x] **Step 1: Create `internal/errors/sentinel.go`**

```go
package errors

import "errors"

var (
    // ErrAuthenticationRejected is returned when the Orchestrator refuses the
    // credentials presented by the Sentinel.
    ErrAuthenticationRejected = errors.New("authentication rejected by the orchestrator")

    // ErrOrchestratorUnreachable is returned when the Orchestrator cannot be
    // contacted.
    ErrOrchestratorUnreachable = errors.New("orchestrator is unreachable")
)
```

- [x] **Step 2: Write the failing test file**

Create `internal/sentinel/client/client_test.go` in package `client_test`. Import the internal errors package as `sentinelErrors "dougdomingos.com/aegis/internal/errors"`.

Declare these test functions:

- `TestClient_Authenticate_WithValidChallenge_ReturnsSession` — starts an `httptest.Server` that answers `POST /machines/authenticate/init` with `{"Nonce":"abc123","ExpiresAt":"2026-10-22T12:00:00Z"}` and `POST /machines/authenticate` with `{"SessionToken":"token-xyz","ExpiresAt":"2026-10-23T12:00:00Z","Machine":{"ID":1,"Name":"lcc1-001","IP":"10.0.1.11","MAC":"aa:bb:cc:dd:ee:ff","Status":"CONNECTED"}}`. The server records the signature it received. Asserts the returned session has `Token` `token-xyz`, `ExpiresAt` equal to the server's value and `MachineName` `lcc1-001`; asserts the recorded signature equals `hex.EncodeToString(auth.Sign(labKey, auth.CanonicalPayload("10.0.1.11", "abc123")))`.
- `TestClient_Authenticate_WithRejectedCredentials_ReturnsAuthenticationRejectedError` — the server answers the authenticate call with HTTP 401 and the body `{"error_code":401,"message":"invalid machine credentials"}`. Asserts the error is non-nil, satisfies `errors.Is(err, sentinelErrors.ErrAuthenticationRejected)`, and contains the text `invalid machine credentials`. This is the test that pins Review Focus item 1.
- `TestClient_Authenticate_WithInitFailure_ReturnsError` — the server answers the init call with HTTP 500. Asserts the error is non-nil.
- `TestClient_Authenticate_WithMalformedResponse_ReturnsError` — the server answers the init call with HTTP 200 and the body `not json`. Asserts the error is non-nil. This is the test that pins Review Focus item 4.
- `TestClient_Authenticate_WithUnreachableOrchestrator_ReturnsUnreachableError` — the client points at a server that has been closed. Asserts the error is non-nil and satisfies `errors.Is(err, sentinelErrors.ErrOrchestratorUnreachable)`.
- `TestClient_Authenticate_WithRequestTimeout_ReturnsUnreachableError` — the server sleeps for 200ms while the client's timeout is 20ms. Asserts the error is non-nil and satisfies `errors.Is(err, sentinelErrors.ErrOrchestratorUnreachable)`.
- `TestClient_Authenticate_WithTrailingSlashBaseURL_RequestsExactPath` — the client's base URL ends with `/`. Asserts the path the server received is exactly `/machines/authenticate/init`. This is the test that pins Review Focus item 3.

Declare the helpers at the end of the file:

```go
// newTestClient builds a client pointing at the provided server URL, using a
// fixed lab key and a short timeout.
func newTestClient(t *testing.T, serverURL string, timeout time.Duration) *client.Client

// labKey is the 32-byte shared key used by the client tests.
var labKey = []byte("0123456789abcdef0123456789abcdef")
```

- [x] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/sentinel/client`
Expected: FAIL — `undefined: client.New`.

- [x] **Step 4: Implement `internal/sentinel/client/client.go`**

Package doc comment: `// Package client implements the Sentinel's client for the Orchestrator API.`

Declare `Client` with unexported fields `baseURL *url.URL`, `httpClient *http.Client`, `labKey []byte` and `logger *slog.Logger`.

`New(baseURL *url.URL, labKey []byte, timeout time.Duration, logger *slog.Logger) *Client` returns a client whose HTTP client has `Timeout: timeout`.

- [x] **Step 5: Implement the request helper**

`post(ctx context.Context, path string, payload any, out any) error` performs, in order:

1. Marshal `payload`; on failure return `fmt.Errorf("failed to encode request payload: %w", err)`.
2. Build the URL by resolving `path` against the base URL — use `c.baseURL.JoinPath(path)` so that a base URL with a trailing slash produces exactly one separator.
3. Create the request with `http.NewRequestWithContext`, set `Content-Type: application/json`, and send it with `c.httpClient.Do`.
4. If the send fails, return `fmt.Errorf("%w: %w", errors.ErrOrchestratorUnreachable, err)`, wrapping the sentinel declared in `internal/errors` (the package is imported plainly as `errors`, following the convention of `internal/service`).
5. Read the body with `io.ReadAll`; close it.
6. If the status code is not in the 2xx range, decode the body into an unexported `errorResponse` struct with the tags `error_code` and `message`. When the status is 401, return `fmt.Errorf("%w: %s", errors.ErrAuthenticationRejected, message)`. Otherwise return `fmt.Errorf("orchestrator returned status %d: %s", status, message)`.
7. Decode the body into `out`; on failure return `fmt.Errorf("failed to decode orchestrator response: %w", err)`.

The unexported `errorResponse` struct mirrors the Orchestrator's error payload:

```go
type errorResponse struct {
    ErrorCode int    `json:"error_code"`
    Message   string `json:"message"`
}
```

- [x] **Step 6: Implement `Authenticate`**

`Authenticate(ctx context.Context, id identity.Identity) (*daemon.Session, error)` performs, in order:

1. Log `"authentication challenge requested"` at info level with the attribute `ip`.
2. Call the init endpoint with `schemas.InitiateAuthenticationSchema{IP: id.IP}` and decode into `schemas.InitiateOutputSchema`.
3. Compute the signature: `hex.EncodeToString(auth.Sign(c.labKey, auth.CanonicalPayload(id.IP, output.Nonce)))`. Never log it.
4. Log `"authentication challenge completed"` at info level with the attribute `ip`.
5. Call the authenticate endpoint with `schemas.AuthenticateMachineSchema{IP: id.IP, MAC: id.MAC, Signature: signature}` and decode into `schemas.AuthenticateOutputSchema`.
6. Return `&daemon.Session{Token: output.SessionToken, ExpiresAt: output.ExpiresAt, MachineName: output.Machine.Name}`.

- [x] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/sentinel/client`
Expected: PASS.

- [x] **Step 8: Add the package to the coverage profile**

In `Makefile`, add `./internal/sentinel/client \` to `TEST_PACKAGES`.

- [x] **Step 9: Verify the whole suite**

Run: `make tests`
Expected: PASS.

- [x] **Step 10: Commit (only if authorized)**

```bash
git add internal/errors/sentinel.go internal/sentinel/client/client.go internal/sentinel/client/client_test.go Makefile
git commit -m "feat(sentinel): add orchestrator authentication client"
```

---

### Task 8: Startup sequence wiring

**Files:**
- Modify: `internal/sentinel/daemon/daemon.go` — add `IdentityDetector`, `Authenticator`, change `New`'s signature, extend `Run`.
- Modify: `cmd/sentinel/main.go` — add the `-config` flag, load the configuration, build the detector and the client.

**Interfaces:**
- Consumes: `config.Load(path string) (*Config, error)`, `config.Config`, `identity.NewDetector() *Detector`, `client.New(baseURL *url.URL, labKey []byte, timeout time.Duration, logger *slog.Logger) *Client`, `logging.Fingerprint(secret string) string`, `daemon.Session` (Task 2).
- Produces: `daemon.IdentityDetector`, `daemon.Authenticator`, `daemon.New(logger *slog.Logger, detector IdentityDetector, authenticator Authenticator, tasks ...Task) *Daemon`.

- [x] **Step 1: Extend the daemon package**

In `internal/sentinel/daemon/daemon.go`:

1. Add the `IdentityDetector` interface with the single method `Detect() (identity.Identity, error)`, documented. Do not add doc comments to implementations.
2. Add the `Authenticator` interface with the single method `Authenticate(ctx context.Context, id identity.Identity) (*Session, error)`, documented.
3. Add the unexported fields `detector IdentityDetector` and `authenticator Authenticator` to `Daemon`.
4. Change `New` to `New(logger *slog.Logger, detector IdentityDetector, authenticator Authenticator, tasks ...Task) *Daemon`.
5. Insert the startup sequence at the top of `Run`, before the tasks are started:

   - Detect the identity: `id, err := d.detector.Detect()`. On failure, return `fmt.Errorf("failed to detect machine identity: %w", err)`. On success, log `"machine identity detected"` at info level with the attributes `ip` and `mac`.
   - Authenticate: `session, err := d.authenticator.Authenticate(ctx, id)`. On failure, return `fmt.Errorf("failed to authenticate: %w", err)`. On success, log `"machine authenticated"` at info level with the attributes `machine`, `session_expires_at` and `session_fingerprint`, the last being `logging.Fingerprint(session.Token)`. Never log `session.Token` itself.

   Import `"dougdomingos.com/aegis/internal/sentinel/identity"` and `"dougdomingos.com/aegis/internal/sentinel/logging"`.

- [x] **Step 2: Wire the entrypoint**

In `cmd/sentinel/main.go`:

1. Declare `const defaultConfigPath = "/etc/aegis/sentinel.json"` with a doc comment.
2. In `run()`, before the daemon starts, declare a flag: `configPath := flag.String("config", defaultConfigPath, "path to the Sentinel configuration file")`, then call `flag.Parse()`.
3. Load the configuration with `config.Load(*configPath)`. On failure, log `"failed to load configuration"` at error level with the attribute `error` using the bootstrap logger, and return `1`.
4. Build the real logger: `logging.New(os.Stderr, logging.Config{Level: cfg.LogLevel, Format: cfg.LogFormat})`.
5. Build the detector `identity.NewDetector()` and the client `client.New(cfg.OrchestratorURL, cfg.LabKey, cfg.RequestTimeout, logger)`.
6. Log `"configuration loaded"` at info level with the attributes `orchestrator_url` and `request_timeout`. Never log `cfg.LabKey`.
7. Build the daemon with `daemon.New(logger, detector, authenticator)` and run it, exactly as before.

- [x] **Step 3: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: no output.

- [x] **Step 4: Smoke test the failure path**

Build the binary, then point it at an unused port:

```bash
go build -o /tmp/aegis-sentinel ./cmd/sentinel
cat > /tmp/sentinel-bad.json <<'EOF'
{ "orchestrator_url": "http://127.0.0.1:9", "lab_key_path": "/tmp/aegis-lab.key" }
EOF
printf '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n' > /tmp/aegis-lab.key
/tmp/aegis-sentinel --config /tmp/sentinel-bad.json; echo "exit code: $?"
```

Expected: the exit code is `1`, and the log contains `machine identity detected` and `daemon stopped with a fatal error`.

- [x] **Step 5: Smoke test the success path**

```bash
go build -o /tmp/aegis-orchestrator ./cmd/orchestrator
printf '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n' > /tmp/aegis-lab.key
MY_IP=$(ip -4 -o addr show scope global | awk '{print $4}' | cut -d/ -f1 | head -1)
printf 'ip,name\n%s,lcc1-001\n' "$MY_IP" > /tmp/aegis-machines.csv
AEGIS_LAB_KEY=/tmp/aegis-lab.key AEGIS_SESSION_SECRET=smoke-secret AEGIS_MACHINES=/tmp/aegis-machines.csv \
  /tmp/aegis-orchestrator > /tmp/orchestrator-smoke.log 2>&1 &
ORCHESTRATOR_PID=$!
sleep 1
cat > /tmp/sentinel-good.json <<EOF
{ "orchestrator_url": "http://127.0.0.1:8080", "lab_key_path": "/tmp/aegis-lab.key" }
EOF
/tmp/aegis-sentinel --config /tmp/sentinel-good.json > /tmp/sentinel-good.log 2>&1 &
SENTINEL_PID=$!
sleep 3
kill -INT $SENTINEL_PID
wait $SENTINEL_PID; echo "exit code: $?"
kill $ORCHESTRATOR_PID
cat /tmp/sentinel-good.log
```

Expected: the exit code is `0`, and the log contains `machine identity detected`, `machine authenticated` with the machine name `lcc1-001`, `daemon running` and `daemon stopping`. If the authentication fails with `invalid machine credentials`, the detected IP differs from `$MY_IP` — set `AEGIS_MACHINES` to the IP printed in the `machine identity detected` record and retry.

- [x] **Step 6: Commit (only if authorized)**

```bash
git add internal/sentinel/daemon/daemon.go cmd/sentinel/main.go
git commit -m "feat(sentinel): authenticate with the orchestrator at startup"
```

---

### Task 9: Operation logging audit and documentation

**Files:**
- Modify: `internal/sentinel/client/client.go`, `internal/sentinel/daemon/daemon.go`, `cmd/sentinel/main.go` — add or correct the records listed in the spec.
- Create: `sentinel.config.example.json`
- Modify: `AGENTS.md`

**Interfaces:**
- Consumes: the operation inventory in spec §9.
- Produces: no new API.

- [x] **Step 1: Walk the operation inventory**

Go through the eleven records of spec §9 one by one and confirm each one exists with the exact message text, at the exact level, in the package the spec names. The records that do not exist yet at this point are the client's two records and the entrypoint's `configuration loaded`; add them where they belong:

- `authentication challenge requested` — client, info, before the init call.
- `authentication challenge completed` — client, info, after the init call succeeds.
- `configuration loaded` — entrypoint, info, after the configuration is loaded.

- [x] **Step 2: Verify that no secret reaches the output**

Run: `rg -n "logger\.(Info|Warn|Error|Debug)" internal/sentinel cmd/sentinel`

Expected: no call site passes `cfg.LabKey`, `session.Token`, the raw signature or the lab key bytes as a log attribute. The session token appears only through `logging.Fingerprint`.

- [x] **Step 3: Create `sentinel.config.example.json`**

At the repository root, create a file holding exactly this document:

```json
{
  "orchestrator_url": "http://10.0.0.5:8080",
  "lab_key_path": "/etc/aegis/lab.key",
  "request_timeout": "10s",
  "log_level": "info",
  "log_format": "text"
}
```

- [x] **Step 4: Update `AGENTS.md`**

1. In §1, replace the Sentinel bullet with one that states it is now implemented as a daemon that loads its configuration, detects its identity, authenticates with the Orchestrator at startup and runs until it is signalled.
2. In the §2.1 directory table, add a row for `cmd/sentinel/` — the Sentinel entrypoint — and a row for `internal/sentinel/` — the Sentinel's configuration, identity, client, daemon and logging packages.
3. In §2.2, add the Sentinel's dependency flow below the Orchestrator's:

```
# Sentinel dependency flow
Sentinel Client ──> Orchestrator API
```

- [x] **Step 5: Verify everything**

Run: `go build ./... && go vet ./... && make tests && golangci-lint run ./...`
Expected: no output from the build, vet and lint; `make tests` reports PASS.

- [x] **Step 6: Check the coverage gate**

Run: `make coverage`
Expected: the aggregate stays at or above 75%. The daemon, logging and entrypoint packages have no tests and stay out of `TEST_PACKAGES`, so they are absent from the profile — that is the accepted consequence recorded in spec §11.

- [x] **Step 7: Commit (only if authorized)**

```bash
git add internal/sentinel cmd/sentinel sentinel.config.example.json AGENTS.md
git commit -m "docs(sentinel): audit operation logging and document the daemon"
```

---

## Pull Request

After the last task is reviewed and authorized:

1. Verify the branch: `git status` is clean and `git log --oneline main..HEAD` lists exactly the nine task commits.
2. Run `make tests`, `make coverage` and `golangci-lint run ./...` one final time.
3. Open the pull request with `gh pr create --base develop --title "..." --body-file ...`, following `.github/PULL_REQUEST_TEMPLATE.md`: a description in Portuguese of the Sentinel daemon's first implementation, the technical decisions from spec §3, and `Fixes #10` under "Issues relacionadas".
4. Do not push or open the pull request before the developer explicitly asks for it (AGENTS.md §6.1).
