# Sentinel Daemon — First Implementation — Design

- **Issue:** [#10](https://github.com/dougdomingos/aegis/issues/10) (US05 — machine connectivity)
- **Branch:** `feat/issue-10b-sentinel-daemon`
- **Status:** Approved design, pending implementation plan
- **Date:** 2026-10-22

## 1. Context & Goal

Aegis is a two-tier system: a central **Orchestrator** (REST API + SQLite) and a
per-endpoint **Sentinel** daemon. The Orchestrator side of the challenge-response
protocol already exists (`internal/auth`, `internal/service/machine_service.go`,
`POST /machines/authenticate/init`, `POST /machines/authenticate`). The Sentinel
side does not exist yet.

This is the **first implementation of the Sentinel daemon**. It establishes the
process, its lifecycle, its configuration, its identity, and its boot-time
authentication against the Orchestrator. It does not yet enforce any policy:
the rule-propagation channel is a separate story.

The three features this branch delivers, in the order they are implemented:

1. **Daemon behavior** — once started, the daemon keeps running and stops only
   on `SIGINT`/`SIGKILL` or on a fatal error.
2. **Auth at startup** — the Sentinel initializes the authentication process
   immediately after startup. If the required configuration is missing, or the
   authentication fails, the process exits immediately.
3. **Logging** — every operation is logged.

## 2. Scope

**In scope:**

- A `sentinel` binary that runs as a long-lived per-endpoint daemon.
- The startup sequence: load configuration → detect the machine identity →
  authenticate against the Orchestrator → run.
- The lifecycle: run until `SIGINT`/`SIGTERM`; exit immediately on a fatal
  error (invalid or missing configuration, failed authentication).
- Structured logging of every operation, with no secret material in the output.

**Out of scope (separate stories):**

- The **rule-propagation channel**. The distribution model is push-based: the
  Orchestrator pushes rules to the Sentinels. The Sentinel will eventually serve
  an inbound HTTPS endpoint for that. TLS, self-signed certificates and
  `ca_cert_path` belong to that story.
- **Heartbeats** from the Sentinel to the Orchestrator.
- **Kernel-level enforcement** of the received rules.
- A **systemd unit** for the daemon.

## 3. Decisions

| Decision | Choice | Rationale |
| --- | --- | --- |
| Package layout | `internal/sentinel/` subtree (`config`, `identity`, `client`, `daemon`, `logging`) | The Sentinel is a separate deployable; keeping it self-contained lets it be removed or moved without touching the Orchestrator's packages. |
| Reuse boundary | Reuses `internal/auth` (signing) and `internal/schemas` (wire payloads) only | `internal/auth` was documented for exactly this ("used by the Orchestrator to verify requests and by the Sentinel to sign them"). `internal/schemas` holds the auth payloads, so the wire format has one definition. |
| Configuration source | JSON file, path from the `-config` flag (default `/etc/aegis/sentinel.json`) | Issue #14 already assumes a Sentinel JSON configuration file. A file is easier to provision on ephemeral lab images than environment variables. |
| Configuration strictness | Unknown JSON fields are rejected | A typo in a key must fail fast instead of silently falling back to a default. |
| Machine identity | Auto-detected from the host's network interfaces; not configurable | Keeps the daemon fully automatic (US05 AC1) and removes a field that would have to be edited per machine. |
| Identity algorithm | Default-route source IP first, first usable IPv4 interface as fallback | The default route is the address the Orchestrator is expected to have configured; container bridges (`docker0`, `br-*`) must not win. |
| Transport (Sentinel → Orchestrator) | Plain HTTP | TLS is required on the **propagation** channel only. The Sentinel → Orchestrator direction carries at most heartbeats, and the push model removes the need for mTLS. |
| Transport (Orchestrator → Sentinel) | Not implemented in this branch | Belongs to the propagation story, together with the self-signed certificate handling. |
| Run loop | Idle: the daemon blocks until a signal arrives | There is no work to do until propagation lands. The task abstraction is in place so the propagation listener plugs in without reshaping the loop. |
| Logging stack | `log/slog` to stderr, level and format configurable | Structured records with severity levels; stderr is what systemd/journald captures. |
| Logging sequencing | The logger is introduced in increment 1; increment 3 audits coverage | Retrofitting a logger across two finished commits is pure churn. |
| Signals | `SIGINT` and `SIGTERM`; a second signal forces an immediate exit | `systemd` sends `SIGTERM`. The second-signal escape hatch mirrors `cmd/orchestrator/main.go`. `SIGKILL` cannot be caught. |
| Exit codes | `0` on graceful stop, `1` on fatal error | Distinguishes "stopped as instructed" from "failed to start". |
| Shared lab key | Parsed by a shared `internal/auth.ReadLabKey` used by both tiers | The hex/32-byte rule exists in two places otherwise. |
| Test scope | Config loading, identity detection and auth client only | Agreed scope: the daemon lifecycle and the logging coverage are shipped unverified (see §11). |
| Git | One branch, one commit per increment, one pull request | Each increment is independently reviewable. |

## 4. Architecture

```
cmd/sentinel/main.go            process entrypoint: flags, wiring, signals, exit code
internal/sentinel/
  config/                       JSON configuration loading and validation
  identity/                     machine identity detection
  client/                       Orchestrator authentication client
  daemon/                       lifecycle: startup sequence, task set, fatal propagation
  logging/                      slog setup, level/format parsing, secret fingerprinting
```

Dependency flow, keeping the layered rule of AGENTS.md (interfaces are owned by
their consumer):

```
cmd/sentinel ──> daemon ──> identity
                    │   └──> client ──> internal/auth, internal/schemas
                    └──> logging
```

- `daemon` declares the `IdentityDetector` and `Authenticator` interfaces it
  consumes; `identity.Detector` and `client.Client` satisfy them structurally.
- `client` imports `daemon` only to return the `daemon.Session` value the
  `Authenticator` contract defines — the return type belongs to the consumer.
- No Sentinel package imports anything from `internal/api`,
  `internal/service` or `internal/store`.

## 5. Configuration

The Sentinel reads a single JSON document:

```json
{
  "orchestrator_url": "http://10.0.0.5:8080",
  "lab_key_path": "/etc/aegis/lab.key",
  "request_timeout": "10s",
  "log_level": "info",
  "log_format": "text"
}
```

| Field | Required | Default | Rules |
| --- | --- | --- | --- |
| `orchestrator_url` | yes | — | Must parse, must use the `http` or `https` scheme, must include a host. |
| `lab_key_path` | yes | — | Path to a file holding the hex-encoded shared lab key; must decode to exactly 32 bytes. |
| `request_timeout` | no | `10s` | Go duration string; must be positive. |
| `log_level` | no | `info` | One of `debug`, `info`, `warn`, `error`. |
| `log_format` | no | `text` | One of `text`, `json`. |

Rules that apply to the whole document:

- Unknown fields are rejected.
- Every failure produces an error, and the process exits with code `1` before
  any network call is attempted.

The `lab_key_path` file follows the same format as the Orchestrator's
`AEGIS_LAB_KEY`: a hex-encoded string decoding to exactly 32 bytes. Whitespace
around the value is trimmed and uppercase hex is accepted.

## 6. Identity Detection

The Sentinel identifies itself by the IP address the Orchestrator has configured
for it; the MAC address is optional metadata.

Resolution order:

1. Resolve the source address of the default route (by opening a UDP socket —
   no packet is sent). Use it if it belongs to a local interface.
2. Otherwise, take the first usable IPv4 address of an interface that is up
   and not loopback.
3. If no usable IPv4 address exists, fail: the error is fatal and the process
   exits with code `1`.

The MAC address is the hardware address of the interface that owns the chosen
IP; it is empty when that interface has none.

The interface lister and the route resolver are injectable, so the resolution
rules are testable without depending on the host's real network.

## 7. Authentication Client

The client performs the Orchestrator's challenge-response protocol:

1. `POST {orchestrator_url}/machines/authenticate/init` with the machine IP.
   The response carries a single-use nonce and its expiry.
2. Sign the nonce: `hex(HMAC-SHA256(lab_key, canonicalPayload(ip, nonce)))`,
   using `internal/auth.CanonicalPayload` and `internal/auth.Sign` unchanged.
3. `POST {orchestrator_url}/machines/authenticate` with the IP, the MAC address
   and the signature. The response carries a session token, its expiry and the
   machine record.

Payloads are marshalled from and unmarshalled into the `internal/schemas` types
(`InitiateAuthenticationSchema`, `InitiateOutputSchema`,
`AuthenticateMachineSchema`, `AuthenticateOutputSchema`). Those types carry no
JSON tags, and the Orchestrator emits Go field names, so the two sides agree on
the wire format by construction. Changing the encoding requires changing the
shared schemas.

Failure handling:

- A non-2xx response is a rejection. A `401` means the credentials were
  refused; the Orchestrator's error message is included in the returned error.
- A transport failure or a timeout means the Orchestrator is unreachable.
- A response that is not valid JSON is a decoding error.
- **There are no retries.** Any failure is fatal: the process logs it and exits
  with code `1`.

## 8. Daemon Lifecycle

The startup sequence runs in order, and any failure in it is fatal:

1. Detect the machine identity.
2. Authenticate against the Orchestrator and obtain the session.
3. Start the task set.
4. Block until the context is cancelled or a task fails.

The task set is a list of named long-running units of work. Each task receives
the daemon's context; the first task to return an error is fatal and stops the
daemon. A task that returns `nil` is simply finished, and the daemon keeps
running. In this branch the task set is empty: the daemon idles until it is
signalled.

Signal handling lives in `cmd/sentinel/main.go`. The first `SIGINT` or
`SIGTERM` cancels the daemon's context, the daemon returns, and the process
exits with code `0`. A second signal logs a forced exit and terminates the
process immediately with code `1`.

## 9. Logging

Every operation emits a record. The complete inventory:

| Record | Emitted by | Level |
| --- | --- | --- |
| `daemon starting` | daemon | info |
| `machine identity detected` | daemon | info |
| `machine authenticated` | daemon | info |
| `daemon running` | daemon | info |
| `daemon stopping` | daemon | info |
| `received signal, shutting down` | entrypoint | info |
| `received second signal, forcing exit` | entrypoint | error |
| `configuration loaded` | entrypoint | info |
| `authentication challenge requested` | client | info |
| `authentication challenge completed` | client | info |
| `daemon stopped with a fatal error` | entrypoint | error |

Secret material is never logged:

- The lab key is never logged in any form.
- The raw signature is never logged.
- The session token is never logged. When it must be correlated across
  records, it is logged as `logging.Fingerprint`, the first eight hexadecimal
  characters of its SHA-256 digest.

The logger writes to stderr. When the configuration itself cannot be loaded, a
bootstrap logger (level `info`, format `text`) reports the failure.

## 10. Error Model

Two package-level sentinels live in `internal/errors`, following the
repository convention:

- `ErrAuthenticationRejected` — the Orchestrator refused the credentials.
- `ErrOrchestratorUnreachable` — the Orchestrator could not be contacted.

The client wraps these with the context of the failed step. The daemon wraps
failures with the failed phase. The entrypoint maps a fatal error to exit
code `1`.

## 11. Testing Strategy

Tests follow the vertical-slice conventions of AGENTS.md: external test
packages, Arrange-Act-Assert, `Test<Type>_<Method>_<Scenario>_<ExpectedResult>`
naming, and table-driven tests for grouped validations.

Covered:

- **`internal/sentinel/config`** — a valid file; the optional fields; a missing
  file; malformed JSON; an unknown field; an invalid `orchestrator_url` (empty,
  malformed, wrong scheme, missing host); an invalid `lab_key_path` (empty,
  missing file, non-hex, wrong size); an invalid `request_timeout` (malformed,
  zero, negative); an invalid `log_level`; an invalid `log_format`.
- **`internal/sentinel/identity`** — the default-route address wins; the
  fallback to the first usable IPv4 interface; loopback and down interfaces are
  skipped; IPv4 is preferred; the MAC comes from the owning interface; an
  interface without a hardware address yields an empty MAC; no usable IPv4 is
  an error; a failure to list the interfaces is an error.
- **`internal/sentinel/client`** — the full handshake against an `httptest`
  server, asserting the exact signature bytes; a non-2xx init; a `401`
  authenticate; a malformed JSON reply; a refused connection; a request
  timeout; the request path is exact when the base URL has a trailing slash.

Deliberately **not** covered in this branch (agreed scope):

- The daemon lifecycle — that it stays alive and stops only on a signal, and
  that a fatal error propagates.
- The logging coverage — that every operation emits a record, and that no
  secret reaches the output.
- An end-to-end check that the Sentinel flips the machine to `CONNECTED` in the
  Orchestrator, which is the only test that proves both halves agree on the
  wire contract.

`Makefile`'s `TEST_PACKAGES` therefore gains only the three tested packages;
the daemon, logging and entrypoint packages stay outside the coverage profile
until they have tests of their own. The consequence is accepted explicitly: the
coverage gate stays green while the daemon's lifecycle and the logging
guarantees ship unverified.

## 12. Done Criteria

1. `sentinel` starts, loads its configuration, detects its identity,
   authenticates against the Orchestrator, and keeps running.
2. The process exits with code `1` when the configuration is missing or
   invalid, and when the authentication fails. No retry is attempted.
3. The process stops on `SIGINT` or `SIGTERM` with exit code `0`; a second
   signal terminates it immediately.
4. A fatal error in a running task stops the process with exit code `1`.
5. Every operation in §9 emits a log record, and no lab key, signature or
   session token appears in the output.
6. The configuration, identity and client packages are covered by the tests in
   §11, and the coverage gate is met.
7. The shared lab key is parsed by `internal/auth.ReadLabKey`, used by both
   tiers.

## 13. Follow-ups

- The push-based rule-propagation channel, with TLS and the self-signed
  certificate handling that issue #14 describes — reshaped, since the
  Orchestrator pushes to the Sentinel and therefore verifies the *Sentinel's*
  certificate.
- Heartbeats, to keep the Orchestrator's machine status accurate.
- Kernel-level enforcement of the received rules.
- Tests for the daemon lifecycle and the logging coverage.
- A systemd unit with `Restart=on-failure`, so that a transient Orchestrator
  outage does not turn into a tight restart loop.
