# US06 — Sentinel Heartbeat Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Keep the Orchestrator's machine status accurate via a periodic Sentinel heartbeat that refreshes `last_seen_at`, with transparent session renewal and read-time stale detection.

**Architecture:** Vertical slice, Orchestrator first: shared session-token primitives in `internal/auth`, a `POST /machines/heartbeat` route over the existing `Touch` store op, staleness derived in the output mapping. Then the Sentinel side: the daemon passes the authenticated session into tasks, a new `internal/sentinel/heartbeat` task ticks, re-authenticates on `401`, and never returns fatal errors.

**Tech Stack:** Go 1.26, chi, SQLite (existing stack; no new dependencies)

**Spec:** `docs/superpowers/specs/2026-10-28-us06-sentinel-heartbeat-design.md`

## Global Constraints

- Go module `dougdomingos.com/aegis`; no new dependencies (spec §2, §Tech).
- Staleness grace is the fixed constant `90 * time.Second` (spec §6.3).
- Default heartbeat interval is `30s` (spec §7.4).
- The heartbeat runner never returns a non-nil error (spec §7.2).
- Every symbol carries a doc comment starting with its name; no doc comments on interface concrete method implementations (AGENTS.md §4.2).
- No comments in code beyond doc comments (AGENTS.md style).
- Tests: external packages, Arrange-Act-Assert, `Test<Type>_<Method>_<Scenario>_<ExpectedResult>`, `t.Errorf`/`t.Fatalf` only (AGENTS.md §5.3).
- Never commit automatically; prepare changes for developer review (AGENTS.md §6.1). Commits below are suggestions staged by the executor only after the developer confirms, or skipped if the developer retains committing.

## Review Focus

- **Expired heartbeat token mid-flight:** must trigger transparent re-auth inside the runner (machine must never flip stale between 24h tokens) — pinned by `TestRunner_Run_SessionRejection_RenewsSessionAndRetries` (Task 8).
- **MAC preservation on heartbeat:** a `Touch` with an empty MAC must keep the stored MAC — pinned by store test (Task 2).
- **Tampered/expired token rejection:** `ParseSessionToken` must verify constant-time and wrap `ErrSessionTokenInvalid` — pinned by session tests (Task 1).
- **Never-authenticated machine display:** nil `last_seen_at` must render `DISCONNECTED`, and a machine idle longer than 90s must too — pinned by service staleness tests (Task 3).
- **Runtime resilience:** heartbeat failures must never stop the daemon — pinned by runner never-returning-error tests (Task 8).

---

### Task 1: Session-Token Primitives in `internal/auth`

**Files:**
- Create: `internal/auth/session.go`
- Create: `internal/auth/session_test.go`
- Modify: `internal/errors/machine.go`
- Modify: `internal/service/machine_service.go:1-17,149-163` (remove `issueSessionToken`, call the moved primitive)

**Interfaces:**
- Consumes: the existing token format implemented by `issueSessionToken` (`internal/service/machine_service.go:152-163`) — moved verbatim, renamed.
- Produces:
  - `auth.SignSessionToken(machineID int64, secret string, ttl time.Duration, now time.Time) (string, time.Time, error)`
  - `auth.ParseSessionToken(token string, secret string, now time.Time) (int64, error)` (wraps `errors.ErrSessionTokenInvalid` on any failure)
  - `errors.ErrSessionTokenInvalid` sentinel
  - Service imports switch: `service` calls `auth.SignSessionToken` at `machine_service.go:137` with identical arguments; delete `issueSessionToken`.

- [ ] **Step 1: Add the sentinel error** to the `var` block in `internal/errors/machine.go`, following the existing doc-comment style:

```go
// ErrSessionTokenInvalid is returned when a session token presented to the
// system is malformed, tampered with, or expired.
ErrSessionTokenInvalid = errors.New("invalid session token")
```

- [ ] **Step 2: Write the failing tests** in `internal/auth/session_test.go` (`package auth_test`). Arrange with the same secret both sides (`"test-secret"`), `now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)`:

- `TestSignSessionToken_RoundTrip_ReturnsValidToken`: `SignSessionToken(42, secret, 24*time.Hour, now)`, assert returned `expiresAt == now.Add(24*time.Hour)` and `ParseSessionToken(token, secret, now.Add(time.Second))` returns `42, nil`.
- `TestParseSessionToken_ExpiredToken_ReturnsInvalidSessionToken`: parse `now.Add(25 * time.Hour)` → error wraps `errors.ErrSessionTokenInvalid` (assert with `errors.Is`).
- `TestParseSessionToken_TamperedToken_ReturnsInvalidSessionToken`: flip a character in the token before the `.` and in the signature part (two subtests) → `ErrSessionTokenInvalid`.
- `TestParseSessionToken_MalformedToken_ReturnsInvalidSessionToken`: table with `""`, `"abc"`, `"a.b", non-decodable base64, valid-encoding-with-non-numeric-payload (sign `base64.RawURLEncoding.EncodeToString([]byte("x:1"))` + `.""`-style valid second part signed properly) → all `ErrSessionTokenInvalid`.
- `TestParseSessionToken_WrongSecret_ReturnsInvalidSessionToken`: signed with secret A, parsed with secret B → `ErrSessionTokenInvalid`.

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/auth/ -run TestSignSessionToken -v && go test ./internal/auth/ -run TestParseSessionToken -v`
Expected: FAIL (`undefined: SignSessionToken`)

- [ ] **Step 4: Implement** `internal/auth/session.go` (package `auth`): move the body of `issueSessionToken` into `SignSessionToken` unchanged (same `base64.RawURLEncoding` + `"."` + HMAC-SHA256 format, same `%d:%d` payload); implement `ParseSessionToken` by splitting on `.`, decoding both parts with `base64.RawURLEncoding`, recomputing the HMAC and comparing with `hmac.Equal`, then `fmt.Sscanf`-free integer parse of the payload (`strconv.ParseInt` on the substring before `:`; expiry is everything after), checking `now.Unix() < expiresAtUnix`. Every failure returns `0, fmt.Errorf("%w: <reason>", errors.ErrSessionTokenInvalid)`.

- [ ] **Step 5: Update the service call site** in `internal/service/machine_service.go`: replace `issueSessionToken(connected.ID, service.sessionSecret, service.sessionTTL, service.now())` with `auth.SignSessionToken(connected.ID, service.sessionSecret, service.sessionTTL, service.now())` and delete the `issueSessionToken` function plus now-unused imports (`crypto/hmac`, `crypto/sha256`, `encoding/base64` if no other use remains).

- [ ] **Step 6: Run tests to verify they pass**

Run: `go build ./... && go test ./internal/auth/ ./internal/service/ -v`
Expected: PASS — the service's existing session tests keep passing because the token format is byte-identical.

- [ ] **Step 7: Commit (on developer approval)**

```bash
git add internal/auth/session.go internal/auth/session_test.go internal/errors/machine.go internal/service/machine_service.go
git commit -m "feat(auth): add shared session token sign and parse primitives"
```

### Task 2: Touch MAC-Preservation Contract

**Files:**
- Modify: `internal/store/machine_store.go:19,95-113`
- Modify: `internal/store/machine_store_test.go`

**Interfaces:**
- Consumes: existing `MachineStore.Touch` (`domain.MachineStore` unchanged — spec §6.2 keeps the signature).
- Produces: `Touch(ctx, id, mac, status)` where `mac == ""` preserves the stored MAC. Later tasks (Task 3 heartbeats) rely on this without passing the machine's MAC.

- [ ] **Step 1: Write the failing test** in `internal/store/machine_store_test.go` (use the existing `arrangeStoreTest` fixture and seed helpers in that file — mirror how the current Touch test seeds machine rows):

- `TestMachineStore_Touch_EmptyMAC_PreservesStoredMAC`: seed a machine with `MAC "aa:bb:cc:dd:ee:ff"`; `Touch(ctx, id, "", domain.ConnectedStatus)`; assert the returned machine's `MAC == "aa:bb:cc:dd:ee:ff"`, `Status == ConnectedStatus`, and `LastSeenAt != nil`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestMachineStore_Touch_EmptyMAC_PreservesStoredMAC -v`
Expected: FAIL (MAC comes back empty)

- [ ] **Step 3: Implement**: replace `queryTouchMachine` (line 19) with

```go
queryTouchMachine = "UPDATE machines SET mac = CASE WHEN ? = '' THEN mac ELSE ? END, status = ?, last_seen_at = CURRENT_TIMESTAMP WHERE id = ?"
```

and pass the MAC argument twice in `Touch`: `store.executor.Exec(ctx, queryTouchMachine, mac, mac, status, id)`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/store/ -v`
Expected: PASS — existing Touch tests pass because they pass non-empty MACs.

- [ ] **Step 5: Commit (on developer approval)**

```bash
git add internal/store/machine_store.go internal/store/machine_store_test.go
git commit -m "feat(store): preserve stored mac when Touch receives an empty one"
```

### Task 3: Heartbeat Service Method and Staleness-Derived Output

**Files:**
- Modify: `internal/service/machine_service.go`
- Modify: `internal/service/machine_service_test.go`

**Interfaces:**
- Consumes: `auth.ParseSessionToken` (Task 1), the `Touch` empty-MAC contract (Task 2), the service's existing `store`, `sessionSecret`, `now` fields.
- Produces:
  - `func (service *MachineService) Heartbeat(ctx context.Context, token string) error`
  - `mapMachineToOutputSchema(machine *domain.Machine, now time.Time) *schemas.MachineOutputSchema` (signature change; both existing call sites at `ListMachines` and `Authenticate` pass `service.now()`)
  - `const machineStalenessGrace = 90 * time.Second` (package scope, `machine_service.go`)
  - Display rule: `LastSeenAt == nil` → `DISCONNECTED`; `now.Sub(*LastSeenAt) > machineStalenessGrace` → `DISCONNECTED`; else `CONNECTED`.

- [ ] **Step 1: Write the failing tests** in `internal/service/machine_service_test.go` (package `service_test`; `arrangeMachineServiceTest` returns `(ctx, mockStore, svc, clock)` and builds the service with session secret `"sign-secret"` and a 24h TTL; sign heartbeat tokens with `auth.SignSessionToken(id, "sign-secret", 24*time.Hour, clock.Now())`):

- `TestMachineService_Heartbeat_ValidToken_TouchesMachineConnected`: sign a token for the seeded machine's ID with `auth.SignSessionToken`; call `Heartbeat`; assert no error and that `ListMachines` now shows the machine as `CONNECTED` with a fresh `LastSeenAt`.
- `TestMachineService_Heartbeat_InvalidToken_ReturnsInvalidSessionTokenError`: table with missing header equivalent (`""` token), malformed (`"not-a-token"`), wrong-secret, and expired (signed with negative TTL) → each wraps `errors.ErrSessionTokenInvalid` via `errors.Is`.
- `TestMachineService_Heartbeat_UnknownMachineID_ReturnsError`: a valid token for an ID that has no row → error (from `Touch`); decide the assertion against the actual returned error (the store wraps it as `fmt.Errorf`) — assert `err != nil` and that it is NOT `nil`; do not assert `ErrSessionTokenInvalid`.
- `TestMachineService_ListMachines_MachineNeverSeen_DisplaysDisconnected`: synced-but-never-authenticated machine → output `Status == "DISCONNECTED"`, `LastSeenAt == nil`.
- `TestMachineService_ListMachines_StaleMachine_DisplaysDisconnected`: authenticate via `Heartbeat` (or `Authenticate`), then `clock.Advance(91 * time.Second)` → output `Status == "DISCONNECTED"`.
- `TestMachineService_ListMachines_FreshMachine_DisplaysConnected`: same setup, clock forward 10s → `Status == "CONNECTED"`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/service/ -run 'TestMachineService_(Heartbeat|ListMachines)' -v`
Expected: FAIL (`service.MachineService` has no method Heartbeat / no staleness knob)

- [ ] **Step 3: Implement** in `internal/service/machine_service.go`:

- Add `const machineStalenessGrace = 90 * time.Second` with its doc comment (worded per spec §6.3).
- `Heartbeat(ctx, token string) error`: parse with `auth.ParseSessionToken(token, service.sessionSecret, service.now())` (on error return it as-is — it already wraps the sentinel), then `_, err := service.store.Touch(ctx, machineID, "", domain.ConnectedStatus)` and return the error unchanged-or-wrapped as the other store callers do.
- Change `mapMachineToOutputSchema(machine *domain.Machine, now time.Time) *schemas.MachineOutputSchema`: keep the nil guard; compute `status := machine.Status; if effectiveStatusString(...)` per the display rule above (a small unexported helper `displayStatus(machine, now) domain.MachineStatus` or inline — one reasonable choice is inline: derive from `machine.LastSeenAt`, with the nil case resolving to `DisconnectedStatus`), set `Status: string(<derived>)`, keep every other field as-is.
- Update the two call sites: `mapMachineToOutputSchema(&machines[i], service.now())` and `mapMachineToOutputSchema(connected, service.now())`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go build ./... && go test ./internal/service/ -v`
Expected: PASS — all pre-existing service tests pass (their `validSessionToken` helper at line ~550 remains valid; leave it or replace its body with `auth.ParseSessionToken` if trivially equivalent — replacing removes duplicated format knowledge and is the preferred choice when assertions still pass).

- [ ] **Step 5: Commit (on developer approval)**

```bash
git add internal/service/machine_service.go internal/service/machine_service_test.go
git commit -m "feat(service): add machine heartbeat and staleness-derived status"
```

### Task 4: Heartbeat API Endpoint

**Files:**
- Modify: `internal/api/handler/machine_handler.go`
- Modify: `internal/api/handler/mock_machine_service_test.go`
- Modify: `internal/api/handler/machine_handler_test.go`

**Interfaces:**
- Consumes: `service.MachineService.Heartbeat` (Task 3) satisfies the new interface method; `errors.ErrSessionTokenInvalid` (Task 1).
- Produces:
  - `MachineServiceInterface` gains `Heartbeat(ctx context.Context, token string) error`
  - Route `POST /machines/heartbeat` inside the existing `/machines` route group
  - Handler method `func (handler *MachineHandler) Heartbeat(w http.ResponseWriter, r *http.Request)` → `204` on success; `401` when the service returns `ErrSessionTokenInvalid`; other errors via `utils.MapByError` unmapped default.

- [ ] **Step 1: Update the mock** in `internal/api/handler/mock_machine_service_test.go`: add the `Heartbeat` field/method following the existing mock's pattern (a function field returning `error`).

- [ ] **Step 2: Write the failing tests** in `internal/api/handler/machine_handler_test.go` (package `handler_test`; mirror the `Authenticate` test block's shape, asserting HTTP status codes only):

- `TestMachineHandler_Heartbeat_ValidBearerToken_ReturnsNoContent`: mock returns `nil`; request `POST /machines/heartbeat` with `Authorization: Bearer <token>`; assert `204`.
- `TestMachineHandler_Heartbeat_InvalidToken_ReturnsUnauthorized`: table — mock returns `errors.ErrSessionTokenInvalid`; also scenario request without an `Authorization` header at all (mock asserts it was called with an empty token, then returns `ErrSessionTokenInvalid`) → `401`.

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/api/handler/ -run TestMachineHandler_Heartbeat -v`
Expected: FAIL (`404` — the route does not exist)

- [ ] **Step 4: Implement** in `internal/api/handler/machine_handler.go`:

- Add `Heartbeat(ctx context.Context, token string) error` to `MachineServiceInterface`.
- Register `r.Post("/heartbeat", handler.Heartbeat)` in `RegisterRoutes`.
- Implement `Heartbeat`: read the token from the `Authorization` header (`strings.TrimPrefix` after a case-insensitive "bearer " check; `strings.TrimSpace` on the result), call the service, map errors `errors.ErrSessionTokenInvalid: http.StatusUnauthorized` via `utils.MapByError`, else `utils.WriteResponse(w, http.StatusNoContent, nil)`-equivalent — check how other 204-worthy responses are written in `internal/api/utils/http_utils.go` and mirror it (one reasonable choice: `w.WriteHeader(http.StatusNoContent)` with no body).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go build ./... && go test ./internal/api/... -v`
Expected: PASS — router tests are unaffected (the concrete service gained the method in Task 3).

- [ ] **Step 6: Commit (on developer approval)**

```bash
git add internal/api/handler/machine_handler.go internal/api/handler/mock_machine_service_test.go internal/api/handler/machine_handler_test.go
git commit -m "feat(api): add machine heartbeat endpoint"
```

### Task 5: Sentinel Client Heartbeat Method

**Files:**
- Modify: `internal/sentinel/client/client.go`
- Modify: `internal/sentinel/client/client_test.go`

**Interfaces:**
- Consumes: the `errorFromStatus` mapping already in `client.go` (401 wraps `errors.ErrAuthenticationRejected`).
- Produces:
  - `const heartbeatPath = "/machines/heartbeat"`
  - `func (c *Client) Heartbeat(ctx context.Context, token string) error` — `nil` on any 2xx; `401` wrapped `ErrAuthenticationRejected`; other statuses plain error; transport failure wrapped `ErrOrchestratorUnreachable`.

- [ ] **Step 1: Write the failing tests** in `internal/sentinel/client/client_test.go` (package `client_test`; mirror the handshake test's `httptest` server pattern and config/constants from that file):

- Test that `Client.Heartbeat` sends `POST` to the exact path (assert with `r.URL.Path`, also verifying a trailing-slash base URL doesn't double it, mirroring the existing path test), with header `Authorization: Bearer <token>` and empty body.
- Test that a `204` response yields `nil`.
- Test that a `401` response yields an error wrapping `errors.ErrAuthenticationRejected` (`errors.Is`).
- Test that a non-2xx non-401 (e.g., `500`) yields a plain error not wrapping the sentinel.
- Test that a refused connection yields an error wrapping `errors.ErrOrchestratorUnreachable`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sentinel/client/ -run TestClient_Heartbeat -v`
Expected: FAIL (method undefined)

- [ ] **Step 3: Implement** in `client.go`: build the request with `http.NewRequestWithContext` and `nil` body, `Authorization: "Bearer " + token`, reuse `c.httpClient` and handle the response as in `post` — success means any `2xx` → `nil` (no body decode), non-2xx → `c.errorFromStatus(status, raw)` after the usual read.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sentinel/client/ -v` — Expected: PASS

- [ ] **Step 5: Commit (on developer approval)**

```bash
git add internal/sentinel/client/client.go internal/sentinel/client/client_test.go
git commit -m "feat(sentinel): send heartbeat requests to the orchestrator"
```

### Task 6: Daemon Passes the Session to Tasks

**Files:**
- Modify: `internal/sentinel/daemon/daemon.go:31-39,84-124`
- Modify: `internal/sentinel/client/client.go:84-89`
- Modify: `internal/sentinel/client/client_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces (used by Tasks 8–9):
  - `daemon.Session` gains `IP string` and `MAC string` fields with doc comments.
  - `daemon.Task.Run` becomes `Run func(ctx context.Context, session *Session) error`; `Daemon.Run` invokes `task.Run(ctx, session)`.
  - `client.Client.Authenticate` fills `IP: id.IP, MAC: id.MAC` in the returned session.

- [ ] **Step 1: Write the failing test** in `internal/sentinel/client/client_test.go`: extend the happy-path handshake test's assertions to check the returned session carries `IP` and `MAC` equal to the identity passed in.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/sentinel/client/ -v` — Expected: FAIL (session has no IP/MAC)

- [ ] **Step 3: Implement**: add `IP`/`MAC` to `daemon.Session`, fill them in `client.go`'s return, and change the task signature + `go func(task Task) { ... task.Run(ctx, session) ... }` in `daemon.go`. Update the doc comments on `Task`/`Run`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go build ./... && go test ./internal/sentinel/... -v` — Expected: PASS

- [ ] **Step 5: Commit (on developer approval)**

```bash
git add internal/sentinel/daemon/daemon.go internal/sentinel/client/client.go internal/sentinel/client/client_test.go
git commit -m "feat(sentinel): deliver the authenticated session to daemon tasks"
```

### Task 7: Sentinel Configuration — `heartbeat_interval`

**Files:**
- Modify: `internal/sentinel/config/config.go`
- Modify: `internal/sentinel/config/config_test.go`

**Interfaces:**
- Consumes: the `parseRequestTimeout` pattern in `config.go:149-167`.
- Produces: `Config.HeartbeatInterval time.Duration` (default `30s` via `defaultHeartbeatInterval`), JSON field `heartbeat_interval`, validated as a Go duration that must be positive when set and present in the document schema.

- [ ] **Step 1: Write the failing tests** in `config_test.go`, mirroring the existing `request_timeout` table:

- `TestConfig_Load_HeartbeatInterval_UnsetField_DefaultsTo30Seconds`
- Table `TestConfig_Load_HeartbeatInterval_InvalidValues_ReturnError`: malformed (`"abc"`), zero (`"0s"`), negative (`"-5s"`) → error mentioning `heartbeat_interval`.
- One valid case (`"5s"`) asserting `cfg.HeartbeatInterval == 5*time.Second`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sentinel/config/ -run TestConfig_Load_HeartbeatInterval -v` — Expected: FAIL

- [ ] **Step 3: Implement**: add `HeartbeatInterval string \`json:"heartbeat_interval"\`` to `document`; `HeartbeatInterval time.Duration` to `Config`; `defaultHeartbeatInterval = 30 * time.Second`; `parseHeartbeatInterval` (same shape as `parseRequestTimeout`); wire it in `Load`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sentinel/config/ -v` — Expected: PASS

- [ ] **Step 5: Commit (on developer approval)**

```bash
git add internal/sentinel/config/config.go internal/sentinel/config/config_test.go
git commit -m "feat(sentinel): add configurable heartbeat interval"
```

### Task 8: Heartbeat Runner

**Files:**
- Create: `internal/sentinel/heartbeat/heartbeat.go`
- Create: `internal/sentinel/heartbeat/heartbeat_test.go`
- Modify: `Makefile:5-16` (`TEST_PACKAGES` gains `./internal/sentinel/heartbeat`)

**Interfaces:**
- Consumes: `client.Client` structurally satisfies both narrow interfaces (`Heartbeat` Task 5; `Authenticate` existing — the identity lands via the session, Task 6); `daemon.Session` with `IP`/`MAC`; `identity.Identity{IP, MAC}` struct fields; `logging.Fingerprint` for token correlation; `errors.ErrAuthenticationRejected`.
- Produces (used by Task 9):
  - `heartbeat.Poster` interface: `Heartbeat(ctx context.Context, token string) error`
  - `heartbeat.SessionRenewer` interface: `Authenticate(ctx context.Context, id identity.Identity) (*daemon.Session, error)`
  - `heartbeat.New(interval time.Duration, poster Poster, renewer SessionRenewer, logger *slog.Logger) *Runner`
  - `func (runner *Runner) Run(ctx context.Context, session *daemon.Session) error` — never returns non-nil (spec §7.2)

- [ ] **Step 1: Write the failing tests** in `heartbeat_test.go` (package `heartbeat_test`). Arrange with fakes: a `fakePoster` with a function field `send func(ctx context.Context, token string) error` plus a send counter recording the tokens seen; a `fakeRenewer` with a function field returning a preset `*daemon.Session` plus a call counter. Drive the loop with a real, tiny interval (e.g., `5 * time.Millisecond`) and await specific fake interactions with bounded waits (`<-time.After(100 * time.Millisecond)` then `t.Fatalf`). The daemon starts each task in exactly one goroutine, so plain field access with no locking is correct — each test below asserts `Run` returns `nil` (on context cancellation) before checking the interactions:

- `TestRunner_Run_TickerBeat_SendsHeartbeatWithSessionToken`: `Run` with interval `5ms`, session token `"tok-1"`; cancel after the poster records one send; assert the send happened, its token argument was `"tok-1"`, and `Run` returned `nil`.
- `TestRunner_Run_SessionRejection_RenewsSessionAndRetries`: first send returns `fmt.Errorf("%w: expired", errors.ErrAuthenticationRejected)`; renewer returns a session with token `"tok-2"`; cancel after the poster has been called twice; assert the renewer was called once and the second send's token was `"tok-2"`.
- `TestRunner_Run_ReauthFailure_WaitsForNextTick`: renewal fails; cancel after two sends failed; assert `Run` returned `nil` (it can never return an error) and the renewer was attempted on ticks.
- `TestRunner_Run_PlainFailure_ContinuesNextTick`: send fails with a plain error; cancel after two sends; assert the renewer was never called and `Run` returned `nil`.
- `TestRunner_Run_ContextCancelled_ReturnsNil`: cancel before any tick; assert `Run` returned `nil` promptly with zero sends.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sentinel/heartbeat/ -v` — Expected: FAIL (package undefined)

- [ ] **Step 3: Implement** `internal/sentinel/heartbeat/heartbeat.go` (package `heartbeat`): the two consumer-owned interfaces with doc comments; the `Runner` struct holding `interval time.Duration`, `poster Poster`, `renewer SessionRenewer`, `logger *slog.Logger`, `token string`. The daemon runs each task in a single goroutine, so plain field access needs no locking. Behavior:

```go
// Run ticks until the context is cancelled, sending heartbeats with the
// current session token and renewing the session when a beat is rejected.
func (runner *Runner) Run(ctx context.Context, session *daemon.Session) error {
    runner.token = session.Token
    ticker := time.NewTicker(runner.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            runner.beat(ctx, session)
        }
    }
}

// beat sends one heartbeat, renewing the session first when the Orchestrator
// rejects the current token. Every outcome is reported through the logger; a
// heartbeat failure never stops the daemon.
func (runner *Runner) beat(ctx context.Context, session *daemon.Session) {
    err := runner.poster.Heartbeat(ctx, runner.token)
    if err == nil {
        runner.logger.Info("heartbeat sent")

        return
    }

    if !errors.Is(err, errors.ErrAuthenticationRejected) {
        runner.logger.Warn("heartbeat failed", "error", err)

        return
    }

    renewed, err := runner.renewer.Authenticate(ctx, identity.Identity{IP: session.IP, MAC: session.MAC})
    if err != nil {
        runner.logger.Warn("heartbeat failed", "error", fmt.Errorf("failed to renew session: %w", err))

        return
    }

    runner.token = renewed.Token
    runner.logger.Info("session renewed", "fingerprint", logging.Fingerprint(runner.token))

    if err := runner.poster.Heartbeat(ctx, runner.token); err != nil {
        runner.logger.Warn("heartbeat failed", "error", err)
    } else {
        runner.logger.Info("heartbeat sent")
    }
}
```

(The code block above fixes the algorithm that the test names leave open — copy it verbatim; it is the only place bodies appear.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sentinel/heartbeat/ -v` — Expected: PASS

- [ ] **Step 5: Extend `Makefile`'s `TEST_PACKAGES`** with `./internal/sentinel/heartbeat` (alphabetical position among the sentinel packages, matching the existing list's grouping).

- [ ] **Step 6: Commit (on developer approval)**

```bash
git add internal/sentinel/heartbeat/ Makefile
git commit -m "feat(sentinel): add periodic heartbeat task with session renewal"
```

### Task 9: Entrypoint Wiring

**Files:**
- Modify: `cmd/sentinel/main.go:54-61`

**Interfaces:**
- Consumes: Tasks 7–8 (`cfg.HeartbeatInterval`, `heartbeat.New`, `daemon.Task`).
- Produces: a running heartbeat task in the Sentinel binary. No exports.

- [ ] **Step 1: Implement** in `run()` (no test — the daemon lifecycle is deliberately out of the tested scope, per the US05 sentinel-daemon design §11):

```go
detector := identity.NewDetector()
orchestrator := client.New(cfg.OrchestratorURL, cfg.LabKey, cfg.RequestTimeout, logger)
heartbeats := heartbeat.New(cfg.HeartbeatInterval, orchestrator, orchestrator, logger)

daemon.New(logger, detector, orchestrator, daemon.Task{
	Name: "heartbeat",
	Run:  heartbeats.Run,
}).Run(ctx)
```

(rename the local `authenticator` to `orchestrator` or equivalent so the one instance serves both roles; keep the existing error/return shape around it)

- [ ] **Step 2: Verify the build and full suite**

Run: `go build ./... && go test ./internal/sentinel/... ./internal/auth/ ./internal/service/ ./internal/store/ ./internal/api/... -v && go vet ./...`
Expected: PASS

- [ ] **Step 3: Commit (on developer approval)**

```bash
git add cmd/sentinel/main.go
git commit -m "feat(sentinel): wire the heartbeat task into the daemon lifecycle"
```
