# US05 — Machine Connectivity & Authentication (Orchestrator API) — Design

- **Issue:** [#10](https://github.com/dougdomingos/aegis/issues/10)
- **Status:** Approved design, pending implementation plan
- **Date:** 2026-10-13

## 1. Context & Goal

Aegis is a two-tier system: a central **Orchestrator** (REST API + SQLite) and a
per-endpoint **Sentinel** daemon. US05 is the first story that concerns endpoint
identity. It requires the system to **automatically authenticate each laboratory
machine at boot** so that policies can later be applied to it.

Acceptance criteria (from the issue):

1. Connection and identification must be **fully automatic** once machine
   credentials are ready.
2. Each machine must be **individually identifiable**, even in ephemeral
   environments without local persistence, through **IP and/or MAC**.
3. **Only certified machines** may be authenticated.

## 2. Scope

**In scope — Orchestrator side only:**

- A **config-driven machine registry**: accepted sources are a list of
  `{ip, name}` entries provided in server configuration, synced into the
  database at startup. Presence in this list is what makes a machine
  "certified" (AC3).
- A machine authentication endpoint implementing the challenge-response
  credential protocol.
- The protocol contract shared with the future Sentinel.

**Out of scope (separate issues / follow-ups):**

- The Sentinel daemon and its boot-time registration client.
- gRPC policy distribution (referenced only as the future transport).
- The trusted laboratory network-range restriction (trusted subnets). This is a
  **separate, cross-cutting issue** that US05 *consumes* as an extra layer. It is
  distinct from the accepted-sources check performed during authentication,
  which is based on the configured machine list.
- Linking machines to logical groups. Groups already model a set of machines,
  but assignment is deferred to its own issue.
- Real-time machine status streaming (US06 / #11).

## 3. Decisions

| Decision | Choice | Rationale |
| --- | --- | --- |
| Deliverable | Orchestrator API + protocol contract | Keep the new Sentinel subsystem as its own story. |
| Credential model | **Server-issued nonce challenge** + fixed IP (two-step) | No PKI; the shared key is never transmitted; replay is prevented by a single-use server nonce. |
| Canonical identity | **Fixed IP** (MAC optional metadata) | The lab has control over addressing, so IPs are static and stable across reboots. |
| MAC handling | Optional, stored/refreshed as metadata | Not relied upon; assists diagnostics in US06. |
| Auth transport | **REST** for auth; gRPC later for policy push | Matches the existing chi API; gRPC reserved for streaming distribution. |
| Registry source | Server config `{ip, name}` list, synced to DB at startup | IPs map deterministically to machines using the `lccX-YYY` naming pattern; no runtime create/update API is needed. |
| Certification | IP present in the configured accepted-sources list | AC3: only configured machines can authenticate. |
| Auth result | **Signed session token** (HMAC) + expiry | Stateless; usable by the future gRPC channel; no sessions table. |
| Shared key storage | Server configuration (env), not DB | It is a lab-wide secret provisioned at deploy time. |
| Challenge key model | Two secrets: a 32-byte `lab_key` (challenge HMAC) and `session_secret` (token signing) | Key separation; the challenge key is a symmetric HMAC key that never travels on the wire. |
| Network restriction | Separate middleware, consumed by US05 | Trusted subnets are a distinct, system-wide compensating control. |

## 4. Domain Model

New file `internal/domain/machine.go`.

```go
type MachineStatus string

const (
    DisconnectedStatus MachineStatus = "DISCONNECTED"
    ConnectedStatus    MachineStatus = "CONNECTED"
)

type Machine struct {
    ID         int64
    Name       string        // assigned from config (e.g. lccX-YYY)
    IP         string        // canonical identity, unique
    MAC        string        // optional metadata
    Status     MachineStatus
    LastSeenAt *time.Time    // nil until first successful authentication
    CreatedAt  time.Time
}
```

`MachineStore` interface (owned by the domain, satisfied by the store layer):

- `GetByIP(ctx, ip string) (*Machine, error)`
- `List(ctx) ([]Machine, error)`
- `Upsert(ctx, machine Machine) (*Machine, error)` — used by the config sync
- `RemoveByIP(ctx, ip string) error` — used by the config sync to prune
- `Touch(ctx, id int64, mac string, status MachineStatus) (*Machine, error)`

New sentinels in `internal/errors/machine.go`:

- `ErrMachineIPRequired`
- `ErrInvalidCredentials`

## 5. Persistence

New migration `internal/infra/migrations/sql/004_create_machine_table.sql`
(single schema change per file, per AGENTS.md):

```sql
CREATE TABLE IF NOT EXISTS machines (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT,
    ip TEXT NOT NULL UNIQUE,
    mac TEXT,
    status TEXT NOT NULL DEFAULT 'DISCONNECTED',
    last_seen_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

Session tokens are **not** persisted. They are HMAC-SHA256-signed payloads
containing the machine ID and an expiry, signed with a server-side secret.

Authentication nonces are **not** persisted. They are held in memory only (see
§7).

## 6. Configuration

Server configuration adds the shared secrets and the machine source list. The
lab key and the machine list are read from files referenced by path so that
secret material and the registry are not stored in environment variables.

```yaml
lab_key: /etc/aegis/lab.key        # file containing the hex-encoded 32-byte key
session_secret: "<hmac signing secret>"
session_ttl: 24h
machines: /etc/aegis/machines.csv   # CSV file with an "ip,name" header row
```

- `AEGIS_LAB_KEY` is the path to a file holding the shared symmetric HMAC key
  used to verify authentication challenges. The file contains a hex-encoded
  string that must decode to exactly 32 bytes (64 hex characters). The key is
  **never transmitted** to or presented by a machine.
- `AEGIS_SESSION_SECRET` signs issued session tokens. Both secrets are required
  and must never be logged.
- `AEGIS_MACHINES` is the path to a CSV file with an `ip,name` header row
  followed by one machine per line. Names follow the `lccX-YYY` pattern
  (lab room + machine index) so they are deterministic from the IP mapping.
- Configuration is passed into `NewRouter` so tests can inject values.

An example is provided in `.env.example`.

## 7. Service Layer

New file `internal/service/machine_service.go`, depending on `MachineStore`, and
the shared `internal/auth` primitives.

- `SyncMachines(ctx, machines []Machine) error` — called once at startup.
  Upserts every configured `{ip, name}` and removes rows whose IP is no longer
  configured, so the DB mirrors the accepted-sources list.
- `ListMachines(ctx) ([]schemas.MachineOutputSchema, error)`
- `InitiateAuthentication(ctx, schema) (*schemas.InitiateOutputSchema, error)` —
  starts a challenge:
  1. Require a non-empty IP, else `ErrMachineIPRequired`.
  2. Generate a fresh single-use nonce (16 random bytes, hex), store it keyed by
     the IP (replacing any prior pending nonce), and return it with its expiry.
  3. A nonce is issued for **any** caller, registered or not, so the response
     does not reveal whether an IP is configured (§9).
- `Authenticate(ctx, schema) (*schemas.AuthenticateOutputSchema, error)` — the
  second step of the challenge-response protocol:
  1. Require a non-empty IP, else `ErrMachineIPRequired`.
  2. Look up the pending nonce for the IP; if absent or expired →
     `ErrInvalidCredentials`.
  3. Decode the supplied `signature` as hex; on failure → `ErrInvalidCredentials`.
  4. Compute `HMAC-SHA256(lab_key, canonicalPayload(ip, nonce))` and compare in
     constant time with `auth.Verify`; on mismatch → `ErrInvalidCredentials`.
  5. Look up the machine by IP; if absent → `ErrInvalidCredentials` (AC3).
  6. Consume (remove) the nonce so it cannot be reused.
  7. `Touch` the row: set `status = CONNECTED`, `last_seen_at = now`, and refresh
     `mac` when provided.
  8. Issue an HMAC-signed session token with expiry (`session_ttl`, default
     **24 hours**) and return it.

The lab key, session secret, and TTL are injected from configuration through the
service constructor so tests can supply values.

### 7.1 Nonce store

New file `internal/service/nonce_store.go` (in-memory, mutex-guarded):

- `Issue(ip) (nonce string, expiresAt time.Time)` — one pending nonce per IP; a
  new issue replaces the previous one.
- `Lookup(ip) (nonce string, ok bool)` — read-only; returns `false` when no live
  nonce exists, evicting an expired entry.
- `Consume(ip)` — removes the pending nonce so it cannot be reused.

Nonces live for **60 seconds** (`nonceTTL`) and are single-use: consumed only on
a successful verification. The store uses an injectable clock for testing.

### 7.2 Shared authentication primitives

New file `internal/auth/hmac.go` provides the canonical-payload and HMAC
operations shared with the future Sentinel:

- `CanonicalPayload(ip, nonce string) []byte` — length-prefixed (`u32be` length
  followed by the bytes) concatenation of the IP and nonce fields.
- `Sign(secret, payload []byte) []byte` — HMAC-SHA256.
- `Verify(secret, payload, signature []byte) bool` — constant-time comparison.

## 8. API Layer

New `internal/api/handler/machine_handler.go` and `internal/schemas/machine.go`.

The handler declares its own `MachineServiceInterface` (owned by the consumer,
per the layered architecture in AGENTS.md).

**Routes:**

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/machines` | List the configured machines and their current status |
| POST | `/machines/authenticate/init` | `{ip}` → `{nonce, expires_at}` (start a challenge) |
| POST | `/machines/authenticate` | `{ip, mac?, signature}` → `{session_token, expires_at, machine}` (complete a challenge) |

There is **no create/update/delete API**: the registry is driven entirely by
configuration (see §6). `GET /machines` exists for visibility and will feed US06.

Schemas:

- `InitiateAuthenticationSchema{ IP }`
- `InitiateOutputSchema{ Nonce, ExpiresAt }`
- `AuthenticateMachineSchema{ IP, MAC, Signature }`
- `MachineOutputSchema{ ID, Name, IP, MAC, Status, LastSeenAt, CreatedAt }`
- `AuthenticateOutputSchema{ SessionToken, ExpiresAt, Machine }`

The `signature` field is the hex-encoded HMAC-SHA256 over
`canonicalPayload(ip, nonce)` keyed with `lab_key`.

Wiring in `internal/api/router.go`: construct the config, store, service, and
handler, run `SyncMachines` at startup, and add the handler to
`registerHandlers`.

The trusted-subnet middleware from the separate issue is applied to these routes
(at minimum `/machines/authenticate/init` and `/machines/authenticate`) as an
additional layer. US05 assumes its interface exists; if unavailable at
implementation time, the address is wired through the same point so the
middleware drops in without further changes.

## 9. Security Notes

- The shared `lab_key` is never transmitted; a machine proves possession by
  signing a server-issued nonce, so a captured signature cannot be replayed
  (the nonce is single-use and bound to the IP).
- The signature covers both the IP and the nonce, so a signature minted for one
  IP cannot be presented for another.
- The auth endpoint does not reveal whether an IP is registered vs. whether the
  signature is wrong: init issues a nonce to any caller and every verify failure
  returns `ErrInvalidCredentials` (HTTP 401).
- Signature verification is constant-time (`hmac.Equal`).
- The lab key and session secret live in configuration, never in the database
  or logs.
- Nonces are short-lived (60s), single-use, and held in memory only.
- IP-only identity is spoofable by design; the trusted-subnet middleware is the
  compensating control and a documented dependency.

## 10. Testing Strategy

Follows the vertical-slice test conventions in AGENTS.md:

- `internal/auth/hmac_test.go` — golden-vector tests for `CanonicalPayload`,
  `Sign`, and `Verify`.
- `internal/store/machine_store_test.go` — uses `arrangeStoreTest`; migrations
  provision the schema; tests for upsert, prune, list, and touch.
- `internal/service/nonce_store_internal_test.go` — white-box tests for issue,
  lookup, consume, replacement, and expiry using an injectable clock.
- `internal/service/machine_service_test.go` — `arrangeMachineServiceTest` plus
  `mapOutputToMachine`; covers config sync, challenge initiation, the full
  challenge-response happy path, wrong/expired/replayed nonces, IP binding, and
  unregistered IPs.
- `internal/api/handler/machine_handler_test.go` — a `MachineServiceInterface`
  mock overriding each method per scenario; asserts HTTP status codes only.
- Coverage must meet the 75% gate; `TEST_PACKAGES` in the `Makefile` is extended
  to include the new packages.

## 11. Done Criteria

1. The configured `{ip, name}` list is synced into the database at startup and
   pruned of removed entries.
2. `GET /machines` lists the configured machines and their status.
3. `POST /machines/authenticate/init` returns a single-use nonce for any caller.
4. A configured machine that signs the nonce with the shared `lab_key`
   authenticates, is marked connected with an updated `last_seen_at`, and
   receives a session token.
5. An invalid signature, an expired or replayed nonce, or an unconfigured IP is
   rejected (`ErrInvalidCredentials`, HTTP 401).
6. The shared key is never transmitted; signatures are bound to the issuing IP.
7. Store, service, and handler tests pass and coverage meets the 75% gate.
