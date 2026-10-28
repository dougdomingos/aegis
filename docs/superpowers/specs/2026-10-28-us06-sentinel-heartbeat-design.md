# US06 — Sentinel Heartbeat (Machine Connectivity) — Design

- **Issue:** [#11](https://github.com/dougdomingos/aegis/issues/11)
- **Status:** Approved design, pending implementation plan
- **Date:** 2026-10-28

## 1. Context & Goal

Aegis is a two-tier system: a central **Orchestrator** (REST API + SQLite) and a
per-endpoint **Sentinel** daemon. US05 established the boot-time
challenge-response authentication and the 24-hour session token; the Sentinel
daemon detects its identity, authenticates once, and then idles. Nothing keeps
the machine's stored status accurate after that first authentication.

US06 requires the administrator to see, in real time, the current connection
state and local filtering performed on each authenticated machine. Since rule
enforcement does not exist yet, this story delivers the connectivity half: a
**heartbeat** from the Sentinel that keeps `status` and `last_seen_at` fresh.

Acceptance criteria (from the issue):

1. The connection state and the local filtering performed on the machine must
   be visible.
2. Machine information must be updated in real time.

## 2. Scope

**In scope:**

- A Sentinel-side heartbeat loop, running as a daemon task, that reports
  connectivity every configured interval.
- An Orchestrator-side `POST /machines/heartbeat` endpoint that verifies the
  session token and refreshes the machine's status and `last_seen_at`.
- Transparent session renewal on token expiry, performed by the Sentinel
  through the existing challenge-response protocol.
- Stale-machine detection: machines whose heartbeat stops are shown as
  `DISCONNECTED` after a grace window, derived at read time.

**Out of scope (separate stories):**

- The local-filtering state report (AC1's second half). The heartbeat protocol
  is designed to gain an optional filtering field when kernel enforcement
  lands; nothing is sent today.
- A live admin feed (SSE/polling for the UI). The administrator observes
  everything through the existing `GET /machines`.
- The rule-propagation channel (Orchestrator → Sentinel push, with TLS).

## 3. Decisions

| Decision | Choice | Rationale |
| --- | --- | --- |
| Transport | **REST** (`POST /machines/heartbeat`) | The heartbeat is a simple periodic call with no streaming need; it reuses the chi API, `internal/auth` primitives and wire schemas. gRPC is no longer planned anywhere else (propagation is push-over-HTTPS with the Sentinel serving the inbound endpoint), so reserving a second transport for a place that no longer exists argues for REST. |
| Heartbeat payload | **Connectivity only** (no body) | Filter state does not exist yet; the session token rides the `Authorization` header, so the request needs no body at all. |
| Admin view scope | Heartbeat only; `GET /machines` is the observation surface | A live feed is a UI concern that enforcement scenarios will shape better. |
| Disconnect detection | **Derived from `last_seen_at` at read time** | Self-healing (survives process restarts with no sweeper to schedule) and avoids a background job in the Orchestrator. |
| Sentinel behavior on heartbeat failure | **Never fatal at runtime** | A transient Orchestrator outage must not kill the daemon (already deployed on lab machines); the machine simply goes stale and recovery happens on the next tick. |
| Token expiry handling | **Transparent re-authentication inside the ticking loop** | The lab key stays loaded for exactly this; a machine whose token expires without re-auth would otherwise flip to `DISCONNECTED` every 24 hours. |
| Heartbeat interval | Config field `heartbeat_interval`, default **30s** | Each machine's interval may vary; nothing in the Orchestrator needs to know it. |
| Orchestrator staleness grace | Fixed **90s** (3× the default interval) | Single misframe default beats a config knob nobody asked for; a machine is `CONNECTED` for exactly one grace window after its last heartbeat (or successful authentication). |
| Session-token primitives | **Move signing/verification into `internal/auth`** | Today the token format (`base64(payload).base64(hmac)`) lives only in `internal/service`. Verifying it must not duplicate that knowledge; `auth` is the shared home for both tiers' authentication primitives. |
| Session delivery to tasks | **Pass the authenticated `Session` into each task** | Tasks are constructed before `daemon.Run` authenticates, so a task cannot receive the token at construction time; handing the session in at task start is the smallest contract change. |

## 4. Session Token Primitives (`internal/auth/session.go`)

New file `internal/auth/session.go`, owning the session-token format shared by
both tiers:

- `SignSessionToken(machineID int64, secret string, ttl time.Duration, now time.Time) (string, time.Time, error)` —
  the moved (and renamed) `issueSessionToken` from `internal/service/machine_service.go`.
  Format unchanged: `base64RawURL("machineID:expiresAtUnix") + "." + base64RawURL(HMAC-SHA256(secret, payload))`.
- `ParseSessionToken(token string, secret string, now time.Time) (int64, error)` —
  constant-time HMAC verification, then expiry check. Returns the machine ID
  for a valid, unexpired token; a wrapped `errors.ErrSessionTokenInvalid`
  otherwise (malformed encoding, tampered digest, non-numeric payload, or
  expired).

`MachineService` calls `auth.SignSessionToken` in `Authenticate`; the local
`issueSessionToken` is removed.

New sentinel in `internal/errors/machine.go`:

- `ErrSessionTokenInvalid` — a heartbeat (or any token-bearer request)
  presented a missing, malformed, tampered or expired session token.

## 5. Orchestrator API (`internal/api/`)

Extension of `internal/api/handler/machine_handler.go` and
`internal/api/handler/machine_handler_test.go` (mock service gains the method).

`MachineServiceInterface` gains:

```go
Heartbeat(ctx context.Context, token string) error
```

**Routes:**

| Method | Path | Purpose | Success | Failure |
| --- | --- | --- | --- | --- |
| POST | `/machines/heartbeat` | Refresh machine connectivity from its session token | `204 No Content` | `401` when the token is absent, malformed, tampered or expired (`ErrSessionTokenInvalid`) |

- The token is read from the `Authorization: Bearer <token>` header. No
  request body is expected.
- The bearer prefix is parsed leniently (case-insensitive scheme, optional
  whitespace); a missing header is the same failure as a missing token.
- Handler signature `Heartbeat(w, r)` is mechanical: read header, call
  service, map errors — mirroring the existing `Authenticate` handler shape.

## 6. Service & Persistence (Orchestrator)

### 6.1 Heartbeat (`internal/service/machine_service.go`)

- `MachineService.Heartbeat(ctx, token string) error`:
  1. `auth.ParseSessionToken(token, service.sessionSecret, service.now())`;
     on error, propagate `ErrSessionTokenInvalid`.
  2. `service.store.Touch(ctx, machineID, "", domain.ConnectedStatus)`.

### 6.2 Touch MAC-preservation contract (`internal/store/machine_store.go`)

The current `Touch` overwrites the MAC unconditionally. Its contract becomes
**an empty MAC keeps the stored one**:

```sql
UPDATE machines SET mac = CASE WHEN ? = '' THEN mac ELSE ? END,
    status = ?, last_seen_at = CURRENT_TIMESTAMP WHERE id = ?
```

Both placeholder `?`s for the MAC receive the same argument. This is safe for
the existing `Authenticate` caller, which already resolves MAC (payload value
or machine's current one) before touching. One store test covers the
preservation.

### 6.3 Staleness at read time

`mapMachineToOutputSchema` gains a `now time.Time` parameter and computes the
displayed status instead of echoing the stored column:

```go
// machineStalenessGrace is how long a machine remains displayed as CONNECTED
// after its last heartbeat (3× the default heartbeat interval).
const machineStalenessGrace = 90 * time.Second
```

- `LastSeenAt == nil` → `Status = DISCONNECTED` (never authenticated).
- `now - LastSeenAt > machineStalenessGrace` → `Status = DISCONNECTED`.
- Otherwise → `Status = CONNECTED`.

Both list (`ListMachines`) and authentication (`Authenticate`) outputs derive
status the same way; a just-authenticated machine is always fresh, so deriving
in `Authenticate` too keeps the mapping unbiased. Relative time arithmetic on
`time.Time` instants is timezone-safe, including the SQLite-parsed
`CURRENT_TIMESTAMP` values.

## 7. Sentinel Side

### 7.1 Session delivery (`internal/sentinel/daemon/daemon.go`, `client.go`)

- `daemon.Session` gains `IP` and `MAC` fields (populated by
  `client.Client.Authenticate`), so tasks that need re-authentication can
  carry the machine identity without re-detecting it.
- `daemon.Task.Run` becomes `Run func(ctx context.Context, session *Session) error`;
  `daemon.Run` starts each task with the verified session. Existing (zero)
  tasks are unaffected; `cmd/sentinel/main.go` wires the new task accordingly.

### 7.2 Heartbeat runner (`internal/sentinel/heartbeat/`)

New package `internal/sentinel/heartbeat`, a daemon task:

```go
// Poster reports connectivity to the Orchestrator.
type Poster interface {
    Heartbeat(ctx context.Context, token string) error
}

// SessionRenewer re-authenticates the machine identity against the
// Orchestrator.
type SessionRenewer interface {
    Authenticate(ctx context.Context, id identity.Identity) (*daemon.Session, error)
}
```

Both interfaces are satisfied structurally by `client.Client` (consumer-owned,
per AGENTS.md). `heartbeat.New(interval time.Duration, poster Poster,
renewer SessionRenewer, logger *slog.Logger) *Runner` — the identity and
starting token arrive with the task invocation.

`Runner.Run(ctx context.Context, session *daemon.Session) error`:

1. Run a ticker loop with `session.Token` as the current token.
2. On each tick, `Poster.Heartbeat(ctx, token)`:
   - Success → `info` log `heartbeat sent`, continue.
   - Failure wrapping `errors.ErrAuthenticationRejected` (the client maps a
     `401` there, which is how expiry presents itself) → re-authenticate via
     `SessionRenewer.Authenticate(ctx, identity.Identity{IP: session.IP,
     MAC: session.MAC})`; on success swap the token (fingerprinted in logs)
     and retry the heartbeat immediately; on failure, log the failure and
     wait for the next tick.
   - Any other failure → `warn` log, continue on the next tick.
3. On context cancellation → return `nil` (graceful stop, daemon exit code 0).

**The runner never returns a non-nil error**, so it can never kill the daemon;
heartbeat failures can only be observed as staleness in the Orchestrator and
as warnings in the Sentinel logs.

### 7.3 Client heartbeat (`internal/sentinel/client/client.go`)

```go
// heartbeatPath is the path of the heartbeat endpoint.
const heartbeatPath = "/machines/heartbeat"

func (c *Client) Heartbeat(ctx context.Context, token string) error
```

- `POST {orchestrator_url}/machines/heartbeat` with the bearer token, empty
  body, honoring the existing per-request timeout.
- `204` (or any 2xx) → `nil`. Non-2xx → the existing `errorFromStatus`
  mapping (`401` wraps `errors.ErrAuthenticationRejected`, anything else is a
  plain status error).

### 7.4 Configuration (`internal/sentinel/config/`)

The Sentinel JSON configuration gains one optional field:

```json
{
  "heartbeat_interval": "30s"
}
```

Parsed like `request_timeout` (Go duration string, must be positive), with
default **30s** when unset. Unknown-field rejection already guards typos.

### 7.5 Wiring (`cmd/sentinel/main.go`)

```go
heartbeats := heartbeat.New(cfg.HeartbeatInterval, clientAPI, clientAPI, logger)
daemon.New(logger, detector, authenticator, daemon.Task{
    Name: "heartbeat",
    Run:  heartbeats.Run,
})
```

## 8. Log Records (Sentinel)

Extends the US05 inventory:

| Record | Emitted by | Level |
| --- | --- | --- |
| `heartbeat sent` | heartbeat | info |
| `heartbeat failed` | heartbeat | warn |
| `session renewed` | heartbeat | info |

The session token continues to appear only as `logging.Fingerprint`.

## 9. Security Notes

- The heartbeat carries no secret material; the token rides only the
  `Authorization` header and is verified in constant time
  (`hmac.Equal` in `ParseSessionToken`).
- Re-authentication reuses the single-use-nonce challenge-response, so
  replay protections carry over unchanged.
- The stale-status derivation is read-side only; nothing reveals machine
  liveness to unauthenticated callers (`GET /machines` is admin surface).

## 10. Testing Strategy

Follows the vertical-slice conventions of AGENTS.md.

- **`internal/auth/session_test.go`** — golden round-trip
  (`SignSessionToken` → `ParseSessionToken`); expired, tampered, malformed
  and non-numeric payloads all wrap `ErrSessionTokenInvalid`.
- **`internal/store/machine_store_test.go`** — new case: `Touch` with an
  empty MAC preserves the stored MAC and refreshes `status`/`last_seen_at`.
- **`internal/service/machine_service_test.go`** — `Heartbeat` with a valid
  token, an expired one, a tampered one, and a missing/empty one (all
  invalid → `ErrSessionTokenInvalid`); a valid token touches the right
  machine as `CONNECTED`; the staleness derivation in list output
  (nil `last_seen` → `DISCONNECTED`, fresh → `CONNECTED`, older than 90s →
  `DISCONNECTED`), table-driven. The
  existing `validSessionToken` test helper is replaced by
  `auth.SignSessionToken`.
- **`internal/api/handler/machine_handler_test.go`** — mock service per
  scenario; assertions on HTTP status codes only: `204` on success, `401`
  on `ErrSessionTokenInvalid`.
- **`internal/sentinel/heartbeat/runner_test.go`** — with an injectable
  clock/ticker and fake Poster/Renewer: sends on tick; renews the session on
  a wrapped `ErrAuthenticationRejected` and retries; ignores other
  failures; never returns an error; stops cleanly on context cancellation.
- **`internal/sentinel/config/config_test.go`** — the `heartbeat_interval`
  table: default when unset, valid value, malformed, zero, negative.
- **`internal/sentinel/client/client_test.go`** — the heartbeat request:
  exact path, bearer header, timeout, `204` success, non-2xx mapping
  (`401` → `ErrAuthenticationRejected`).
- `Makefile`'s `TEST_PACKAGES` gains `./internal/sentinel/heartbeat`; the
  75% gate is preserved.

## 11. Done Criteria

1. An authenticated Sentinel heartbeats every 30s by default; each success
   refreshes the machine's `last_seen_at` and displays it as `CONNECTED`.
2. An expired session token triggers a transparent in-place
   re-authentication; the daemon is never restarted and the machine never
   appears stale between tokens.
3. A machine that stops heartbeating displays `DISCONNECTED` on
   `GET /machines` after 90 seconds of staleness.
4. Heartbeat runtime errors never stop the Sentinel; they are logged as
   warnings and observed in the Orchestrator only as staleness.
5. Store, service, handler, `internal/auth`, client, config and heartbeat
   tests pass; coverage meets the 75% gate.

## 12. Follow-ups

- Optional filtering-state field in the heartbeat payload, once kernel
  enforcement exists (AC1's second half).
- The live admin feed (SSE or polling cadence on the client side) when UI
  work starts.
- Configurable staleness grace on the Orchestrator, only if deployment shows
  the 90s default is wrong.

## 13. Amendment — configurable staleness grace (2026-10-28)

The §3/§6.3 fixed 90s grace became `AEGIS_STALENESS_GRACE` (Go duration,
default `90s`, must be positive), injected into `MachineService` via its
constructor and wired from `internal/config`, mirroring `AEGIS_SESSION_TTL`.
Rationale: an unconfigurable grace made the system-level stale-display
behavior untestable without a 90-second E2E wait, and the knob follows an
established pattern. The Sentinel-side upper bound (`heartbeat_interval` must
be shorter than 90s) keeps validating against the default grace: deployments
that shorten the grace must also configure shorter Sentinel intervals, which
the fleet already does per-machine.
