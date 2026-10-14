# US05 Machine Authentication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a config-driven machine registry and a challenge-response machine authentication endpoint to the Aegis Orchestrator API.

**Architecture:** Vertical slice Store → Service → API Handler. Accepted machine sources (`{ip, name}`) come from server configuration and are synced into a `machines` table at startup. `POST /machines/authenticate/init` issues a single-use nonce; the machine signs it with the shared `lab_key`; `POST /machines/authenticate` validates the signature, marks the machine connected, and issues a signed session token.

**Tech Stack:** Go 1.26.5, chi v5.3.1, modernc.org/sqlite v1.55.0, stdlib `crypto/hmac`, `crypto/sha256`, `encoding/hex`.

**Spec:** `docs/superpowers/specs/2026-10-13-us05-machine-authentication-design.md`

## Global Constraints

- Module path: `dougdomingos.com/aegis`; Go `1.26.5`.
- No new third-party dependencies — use only the standard library for signing.
- Every exported (and unexported) symbol carries a doc comment starting with its name.
- Tests live in the same directory as their target, in an external package (`package x_test`), except white-box tests that need unexported state (e.g. the nonce store), which use `package x` with an `_internal_test.go` suffix.
- Test function names follow `Test<Type>_<Method>_<Scenario>_<ExpectedResult>`.
- Assertions use plain `t.Errorf`/`t.Fatalf` with descriptive messages.
- New SQL goes in its own migration file named `XXX_<name>.sql`; never modify existing migrations.
- Do **not** commit automatically (AGENTS.md §6.1). Each commit step runs only when the developer has explicitly authorized commits.

## Review Focus

- A machine row that has never authenticated has `last_seen_at = NULL`; scanning it must not error (Task 2, Task 4).
- Re-syncing config must update the name without resetting an existing machine's `status`/`last_seen_at` (Task 2, Task 4).
- Authenticating the same machine repeatedly must update status/`last_seen_at`, not insert duplicates (Task 2, Task 4).
- An empty or whitespace-only signature with a valid IP must be rejected (Task 5).
- Removing an IP from config must prune its machine row at startup (Task 4).
- The shared `lab_key` must be validated to decode to exactly 32 bytes (Task 4).
- A signature minted for one IP must be rejected for another (Task 5).
- A consumed (replayed) or expired nonce must be rejected (Task 5).
- Init must issue a nonce even for an unregistered IP so registration is not revealed (Task 5, Task 6).

---

### Task 1: Machine domain, errors, and migration

**Files:**
- Create: `internal/domain/machine.go`
- Create: `internal/errors/machine.go`
- Create: `internal/infra/migrations/sql/004_create_machine_table.sql`

**Interfaces:**
- Consumes: nothing.
- Produces: `domain.Machine`, `domain.MachineStatus`, `domain.DisconnectedStatus`, `domain.ConnectedStatus`, `domain.MachineStore`; sentinels `errors.ErrMachineIPRequired`, `errors.ErrInvalidCredentials`.

- [ ] **Step 1: Create the migration**

```sql
CREATE TABLE IF NOT EXISTS machines (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    ip TEXT NOT NULL UNIQUE,
    mac TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'DISCONNECTED',
    last_seen_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

- [ ] **Step 2: Verify migrations still pass**

Run: `go test ./internal/infra/migrations`
Expected: PASS (the file-count test discovers `004` automatically). A NULL `last_seen_at` is intentional so this column must stay nullable.

- [ ] **Step 3: Create `internal/domain/machine.go`**

Define, with doc comments on every symbol:

```go
type MachineStatus string

const (
    DisconnectedStatus MachineStatus = "DISCONNECTED"
    ConnectedStatus    MachineStatus = "CONNECTED"
)

type Machine struct {
    ID         int64
    Name       string
    IP         string
    MAC        string
    Status     MachineStatus
    LastSeenAt *time.Time
    CreatedAt  time.Time
}

type MachineStore interface {
    GetByIP(ctx context.Context, ip string) (*Machine, error)
    List(ctx context.Context) ([]Machine, error)
    Upsert(ctx context.Context, machine Machine) (*Machine, error)
    RemoveByIP(ctx context.Context, ip string) error
    Touch(ctx context.Context, id int64, mac string, status MachineStatus) (*Machine, error)
}
```

- [ ] **Step 4: Create `internal/errors/machine.go`**

```go
var (
    // ErrMachineIPRequired is returned when a machine IP is required but not provided.
    ErrMachineIPRequired = errors.New("must specify machine ip")

    // ErrInvalidCredentials is returned when a machine cannot be authenticated,
    // either because the IP is not a configured source or the signature is wrong.
    ErrInvalidCredentials = errors.New("invalid machine credentials")
)
```

- [ ] **Step 5: Build**

Run: `go build ./...`
Expected: no output (success).

- [ ] **Step 6: Commit (only if authorized)**

```bash
git add internal/domain/machine.go internal/errors/machine.go internal/infra/migrations/sql/004_create_machine_table.sql
git commit -m "feat(domain): add machine entity and migration"
```

---

### Task 2: Machine store

**Files:**
- Create: `internal/store/machine_store.go`
- Test: `internal/store/machine_store_test.go`

**Interfaces:**
- Consumes: `domain.Machine`, `domain.MachineStore`, `domain.MachineStatus` (Task 1); existing `query.QueryExecutor`, `arrangeStoreTest`.
- Produces: `store.NewMachineStore(db *sql.DB) *MachineStore` implementing `domain.MachineStore`.

- [ ] **Step 1: Write the failing store tests**

Create `internal/store/machine_store_test.go` (package `store_test`, importing `dougdomingos.com/aegis/internal/store`). Use `arrangeStoreTest(t, store.NewMachineStore)` (function value, matching the existing generic helper in `internal/store/helpers_test.go`). Cases:

- `TestMachineStore_Upsert_WithNewIP_InsertsDisconnectedMachine` — Upsert `{IP:"10.0.1.11", Name:"lcc1-011"}`; assert non-zero `ID`, `Status == domain.DisconnectedStatus`, `LastSeenAt == nil`, `CreatedAt` non-zero.
- `TestMachineStore_Upsert_WithExistingIP_UpdatesNameOnly` — Upsert, then `Touch` to connected, then Upsert same IP with a new name; assert `Name` updated, `Status` still `domain.ConnectedStatus`, `LastSeenAt` still non-nil, and `List` returns one row.
- `TestMachineStore_GetByIP_WithUnknownIP_ReturnsNil` — assert returned machine is `nil` and error is `nil`.
- `TestMachineStore_List_ReturnsAllMachines` — upsert two, assert length 2.
- `TestMachineStore_RemoveByIP_RemovesMachine` — upsert then remove; `GetByIP` returns `nil`.
- `TestMachineStore_Touch_UpdatesStatusMACAndLastSeen` — upsert, touch with `{mac:"aa:bb", status:domain.ConnectedStatus}`; assert `MAC`, `Status`, and non-nil `LastSeenAt`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/store -run TestMachineStore -v`
Expected: FAIL — `NewMachineStore` undefined.

- [ ] **Step 3: Implement `internal/store/machine_store.go`**

Model it on `internal/store/group_store.go`. SQL constants:

```go
queryUpsertMachine = `INSERT INTO machines (name, ip, status) VALUES (?, ?, 'DISCONNECTED')
    ON CONFLICT(ip) DO UPDATE SET name = excluded.name`
queryGetMachineByIP   = "SELECT id, name, ip, mac, status, last_seen_at, created_at FROM machines WHERE ip = ?"
queryGetMachineByID   = "SELECT id, name, ip, mac, status, last_seen_at, created_at FROM machines WHERE id = ?"
queryListMachines     = "SELECT id, name, ip, mac, status, last_seen_at, created_at FROM machines"
queryDeleteMachineByIP = "DELETE FROM machines WHERE ip = ?"
queryTouchMachine     = "UPDATE machines SET mac = ?, status = ?, last_seen_at = CURRENT_TIMESTAMP WHERE id = ?"
```

- `MachineStore` wraps `*query.QueryExecutor[domain.Machine]`, built with `query.NewQueryExecutor(db, mapRowToMachine)`.
- `Upsert` executes `queryUpsertMachine(name, ip)` then returns `GetByIP(ip)`. The `ON CONFLICT` clause updates only `name`, preserving `status`/`last_seen_at` (Review Focus #2).
- `Touch(id, mac, status)` executes `queryTouchMachine`, returns the machine via `executor.QueryOne(queryGetMachineByID, id)`. If `RowsAffected() == 0`, return `fmt.Errorf("machine with id %d not found", id)`.
- `RemoveByIP` uses `executor.Exec`; ignore zero rows (pruning an absent row is not an error).
- `mapRowToMachine(scan query.ScanFunc) (domain.Machine, error)` scans `&m.ID, &m.Name, &m.IP, &m.MAC, &m.Status, &m.LastSeenAt, &m.CreatedAt`. Do **not** use `sql.NullTime` — `*time.Time` scans NULL as nil via `database/sql`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/store -run TestMachineStore -v`
Expected: PASS.

- [ ] **Step 5: Commit (only if authorized)**

```bash
git add internal/store/machine_store.go internal/store/machine_store_test.go
git commit -m "feat(store): implement machine persistence"
```

---

### Task 3: Shared authentication primitives

**Files:**
- Create: `internal/auth/hmac.go`
- Test: `internal/auth/hmac_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `auth.CanonicalPayload(ip, nonce string) []byte`, `auth.Sign(secret, payload []byte) []byte`, `auth.Verify(secret, payload, signature []byte) bool`.

- [ ] **Step 1: Write the failing tests**

Create `internal/auth/hmac_test.go` (package `auth_test`). Use golden hex vectors.

- `TestAuth_CanonicalPayload_WithGoldenInput_ReturnsKnownEncoding` — `CanonicalPayload("192.168.0.10", "abc123")` equals the hex `0000000c3139322e3136382e302e313000000006616263313233`.
- `TestAuth_Sign_WithGoldenInput_ReturnsKnownSignature` — `Sign` over that payload with key `"0123456789abcdef0123456789abcdef"` equals the hex `74cd02b7ad2fb3d3ba018ba811db2e2ba947f96014e8fd7ed628b70d10caae02`.
- `TestAuth_Verify_WithValidSignature_ReturnsTrue`
- `TestAuth_Verify_WithTamperedPayload_ReturnsFalse` — change the IP.
- `TestAuth_Verify_WithWrongSecret_ReturnsFalse`

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/auth -v`
Expected: FAIL — `auth.CanonicalPayload` undefined.

- [ ] **Step 3: Implement `internal/auth/hmac.go`**

Package doc: "Package auth provides the shared authentication primitives used by the Orchestrator to verify requests and by the Sentinel to sign them."

```go
func CanonicalPayload(ip, nonce string) []byte {
    fields := []string{ip, nonce}
    payload := make([]byte, 0, 8+len(ip)+len(nonce))
    for _, field := range fields {
        length := make([]byte, 4)
        binary.BigEndian.PutUint32(length, uint32(len(field)))
        payload = append(payload, length...)
        payload = append(payload, field...)
    }
    return payload
}

func Sign(secret, payload []byte) []byte {
    mac := hmac.New(sha256.New, secret)
    mac.Write(payload)
    return mac.Sum(nil)
}

func Verify(secret, payload, signature []byte) bool {
    return hmac.Equal(Sign(secret, payload), signature)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/auth -v`
Expected: PASS.

- [ ] **Step 5: Commit (only if authorized)**

```bash
git add internal/auth/hmac.go internal/auth/hmac_test.go
git commit -m "feat(auth): add shared canonical payload and HMAC primitives"
```

---

### Task 4: Configuration loader

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `config.Machine{ IP, Name string }`, `config.Config{ LabKey []byte; SessionSecret string; SessionTTL time.Duration; Machines []Machine }`, `config.Load() (*Config, error)`.

- [ ] **Step 1: Write the failing config tests**

Update `internal/config/config_test.go` (package `config_test`). Use `t.Setenv`. Replace `AEGIS_LAB_TOKEN` with a 64-hex-char `AEGIS_LAB_KEY`. Cases:

- `TestConfig_Load_WithValidEnv_ReturnsParsedConfig` — set `AEGIS_LAB_KEY` (64 hex chars), `AEGIS_SESSION_SECRET`, `AEGIS_MACHINES`; assert `LabKey` decodes to the 32 bytes, secret, TTL `24h`, and two machines.
- `TestConfig_Load_WithCustomTTL_ReturnsConfiguredTTL`
- `TestConfig_Load_WithoutLabKey_ReturnsError`
- `TestConfig_Load_WithNonHexLabKey_ReturnsError` — `"not-hex"`.
- `TestConfig_Load_WithWrongSizeLabKey_ReturnsError` — 16 bytes hex.
- `TestConfig_Load_WithoutSessionSecret_ReturnsError`
- `TestConfig_Load_WithInvalidTTL_ReturnsError`
- `TestConfig_Load_WithMalformedMachineEntry_ReturnsError`
- `TestConfig_Load_WithInvalidMachineIP_ReturnsError`
- `TestConfig_Load_WithoutMachines_LogsWarning`

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config -v`
Expected: FAIL — `LabKey` / `AEGIS_LAB_KEY` undefined.

- [ ] **Step 3: Implement `internal/config/config.go`**

- Add `envLabKey = "AEGIS_LAB_KEY"` and a `labKeyLength = 32` constant; remove `envLabToken`.
- Replace `Config.LabToken string` with `Config.LabKey []byte`.
- Add `parseLabKey(raw string) ([]byte, error)`: trim; empty → `fmt.Errorf("%s is required", envLabKey)`; `hex.DecodeString` failure → `fmt.Errorf("invalid %s value %q: must be hex-encoded", envLabKey, trimmed)`; `len != 32` → `fmt.Errorf("invalid %s value: must decode to exactly %d bytes", envLabKey, labKeyLength)`.
- Keep `parseSessionTTL` and `parseMachines` unchanged.
- `Load` builds `Config{LabKey, SessionSecret, SessionTTL, Machines}`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config -v`
Expected: PASS.

- [ ] **Step 5: Commit (only if authorized)**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): load 32-byte lab key, session secret, and machine sources"
```

---

### Task 5: Nonce store and machine service

**Files:**
- Create: `internal/service/nonce_store.go`
- Test: `internal/service/nonce_store_internal_test.go` (white-box, `package service`)
- Modify: `internal/schemas/machine.go`
- Modify: `internal/service/machine_service.go`
- Modify: `internal/service/machine_service_test.go`

**Interfaces:**
- Consumes: `domain.MachineStore` (Task 2), `auth.*` (Task 3), schemas.
- Produces: `service.NewMachineService(store domain.MachineStore, labKey []byte, sessionSecret string, sessionTTL time.Duration, now func() time.Time) *MachineService` with methods `SyncMachines`, `ListMachines`, `InitiateAuthentication`, `Authenticate`.

- [ ] **Step 1: Update the schemas**

In `internal/schemas/machine.go`:

```go
// InitiateAuthenticationSchema declares the fields sent by a machine to start
// an authentication challenge.
type InitiateAuthenticationSchema struct {
    IP string
}

// InitiateOutputSchema declares the result of starting an authentication
// challenge.
type InitiateOutputSchema struct {
    Nonce     string
    ExpiresAt time.Time
}

// AuthenticateMachineSchema declares the fields sent by a machine to complete
// an authentication challenge.
type AuthenticateMachineSchema struct {
    IP        string
    MAC       string
    Signature string
}
```

Keep `MachineOutputSchema` and `AuthenticateOutputSchema` unchanged.

- [ ] **Step 2: Write the failing nonce store tests**

Create `internal/service/nonce_store_internal_test.go` (`package service`). Use a closure clock (`now := time.Now(); store := newNonceStore(func() time.Time { return now })`). Cases:

- `TestNonceStore_Issue_ReturnsNonceAndExpiry` — 32-char nonce, expiry `now+60s`.
- `TestNonceStore_Issue_WithExistingNonce_ReplacesPrevious`
- `TestNonceStore_Lookup_WithValidNonce_ReturnsTrue`
- `TestNonceStore_Lookup_WithUnknownIP_ReturnsFalse`
- `TestNonceStore_Lookup_AfterExpiry_ReturnsFalse`
- `TestNonceStore_Consume_RemovesPendingNonce`

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/service -run TestNonceStore -v`
Expected: FAIL — `newNonceStore` undefined.

- [ ] **Step 4: Implement `internal/service/nonce_store.go`**

```go
const nonceTTL = 60 * time.Second
const nonceLength = 16

type nonceEntry struct {
    nonce     string
    expiresAt time.Time
}

type nonceStore struct {
    mu      sync.Mutex
    pending map[string]nonceEntry
    now     func() time.Time
}

func newNonceStore(now func() time.Time) *nonceStore
func (store *nonceStore) Issue(ip string) (string, time.Time)   // replaces prior nonce
func (store *nonceStore) Lookup(ip string) (string, bool)       // evicts expired
func (store *nonceStore) Consume(ip string)                     // removes pending
```

`randomNonce()` reads `nonceLength` bytes from `crypto/rand` and hex-encodes them.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/service -run TestNonceStore -v`
Expected: PASS.

- [ ] **Step 6: Write the failing service tests**

Update `internal/service/machine_service_test.go` (package `service_test`). Change `arrangeMachineServiceTest` to return `(context.Context, *MockMachineStore, *service.MachineService, *testClock)` and construct the service with `NewMachineService(mock, []byte(labSecretKey), "sign-secret", 24*time.Hour, clock.Now)` where `labSecretKey` is a 32-byte string. Add helpers `initiateChallenge`, `signChallenge`, `signChallengeFor`, and a `testClock` type with `Now`/`Advance`. Cases:

- Sync/List cases (unchanged behavior).
- `TestMachineService_InitiateAuthentication_WithValidIP_ReturnsNonce`
- `TestMachineService_InitiateAuthentication_WithUnknownIP_ReturnsNonce`
- `TestMachineService_InitiateAuthentication_WithEmptyIP_ReturnsIPRequired`
- `TestMachineService_Authenticate_WithValidChallenge_ReturnsSession` — initiate, sign, authenticate; assert connected, `LastSeenAt`, token, expiry.
- `TestMachineService_Authenticate_WithUnknownIP_ReturnsInvalidCredentials`
- `TestMachineService_Authenticate_WithUnknownIPAndValidSignature_ReturnsInvalidCredentials`
- `TestMachineService_Authenticate_WithWrongSignature_ReturnsInvalidCredentials`
- `TestMachineService_Authenticate_WithSignatureForDifferentIP_ReturnsInvalidCredentials`
- `TestMachineService_Authenticate_WithReplayedNonce_ReturnsInvalidCredentials`
- `TestMachineService_Authenticate_WithExpiredNonce_ReturnsInvalidCredentials`
- `TestMachineService_Authenticate_WithEmptySignature_ReturnsInvalidCredentials`
- `TestMachineService_Authenticate_WithMalformedSignature_ReturnsInvalidCredentials`
- `TestMachineService_Authenticate_WithEmptyIP_ReturnsIPRequired`
- `TestMachineService_Authenticate_WhenStoreFails_ReturnsError`

- [ ] **Step 7: Run tests to verify they fail**

Run: `go test ./internal/service -run TestMachineService -v`
Expected: FAIL — `NewMachineService` signature mismatch.

- [ ] **Step 8: Implement `internal/service/machine_service.go`**

Struct holds `store domain.MachineStore`, `labKey []byte`, `sessionSecret string`, `sessionTTL time.Duration`, `nonces *nonceStore`, `now func() time.Time`.

- `InitiateAuthentication`: require IP; `nonces.Issue(ip)`; return `InitiateOutputSchema{Nonce, ExpiresAt}`.
- `Authenticate`:
  1. Empty IP → `ErrMachineIPRequired`.
  2. `nonces.Lookup(ip)`; no live nonce → `ErrInvalidCredentials`.
  3. `hex.DecodeString(payload.Signature)`; error → `ErrInvalidCredentials`.
  4. `auth.Verify(labKey, auth.CanonicalPayload(ip, nonce), signature)`; false → `ErrInvalidCredentials`.
  5. `store.GetByIP`; nil → `ErrInvalidCredentials`.
  6. `nonces.Consume(ip)`.
  7. `Touch(id, mac, ConnectedStatus)`.
  8. Issue session token via the existing `issueSessionToken` helper and return.

Keep `SyncMachines`, `ListMachines`, `issueSessionToken`, and `mapMachineToOutputSchema` otherwise unchanged. Remove `crypto/subtle` import.

- [ ] **Step 9: Run tests to verify they pass**

Run: `go test ./internal/service -run 'TestMachineService|TestNonceStore' -v`
Expected: PASS.

- [ ] **Step 10: Commit (only if authorized)**

```bash
git add internal/schemas/machine.go internal/service/nonce_store.go internal/service/nonce_store_internal_test.go internal/service/machine_service.go internal/service/machine_service_test.go
git commit -m "feat(service): add nonce challenges and HMAC machine authentication"
```

---

### Task 6: Machine handler

**Files:**
- Modify: `internal/api/handler/machine_handler.go`
- Modify: `internal/api/handler/machine_handler_test.go`
- Modify: `internal/api/handler/mock_machine_service_test.go`

**Interfaces:**
- Consumes: `schemas.InitiateAuthenticationSchema`, `schemas.InitiateOutputSchema`, `schemas.AuthenticateMachineSchema`, `schemas.MachineOutputSchema`, `schemas.AuthenticateOutputSchema` (Task 5); `utils.DecodePayload`, `utils.EncodeToJSON`, `utils.MapByError`.
- Produces: `handler.MachineServiceInterface`, `handler.NewMachineHandler(handler.MachineServiceInterface) *MachineHandler`, `(*MachineHandler).RegisterRoutes(chi.Router)`.

- [ ] **Step 1: Write the failing handler tests and mock**

Update `internal/api/handler/mock_machine_service_test.go` (package `handler_test`) to add an `initiateFn func(ctx, schemas.InitiateAuthenticationSchema) (*schemas.InitiateOutputSchema, error)` field and the matching interface method.

Update `internal/api/handler/machine_handler_test.go` (package `handler_test`). Change authenticate payloads from `Token` to `Signature`. Add init cases:

- `TestMachineHandler_Initiate_WithValidIP_ReturnsStatus200`
- `TestMachineHandler_Initiate_WithoutIP_ReturnsStatus400`
- `TestMachineHandler_Initiate_WithMalformedPayload_ReturnsStatus400`
- `TestMachineHandler_Initiate_WhenServiceFails_ReturnsStatus500`

Keep the List and Authenticate cases, asserting only the HTTP status code.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/handler -run TestMachineHandler -v`
Expected: FAIL — `InitiateAuthentication` undefined on the interface.

- [ ] **Step 3: Implement `internal/api/handler/machine_handler.go`**

```go
type MachineServiceInterface interface {
    ListMachines(ctx context.Context) ([]schemas.MachineOutputSchema, error)
    InitiateAuthentication(ctx context.Context, p schemas.InitiateAuthenticationSchema) (*schemas.InitiateOutputSchema, error)
    Authenticate(ctx context.Context, p schemas.AuthenticateMachineSchema) (*schemas.AuthenticateOutputSchema, error)
}
```

Routes:

```go
r.Route("/machines", func(r chi.Router) {
    r.Get("/", handler.List)
    r.Post("/authenticate/init", handler.Initiate)
    r.Post("/authenticate", handler.Authenticate)
})
```

`Initiate` decodes `InitiateAuthenticationSchema`, calls the service, maps `{ErrMachineIPRequired: 400}`, and returns 200 with `InitiateOutputSchema`. `Authenticate` decodes `AuthenticateMachineSchema`, calls the service, maps `{ErrMachineIPRequired: 400, ErrInvalidCredentials: 401}`, and returns 200.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/handler -run TestMachineHandler -v`
Expected: PASS.

- [ ] **Step 5: Commit (only if authorized)**

```bash
git add internal/api/handler/machine_handler.go internal/api/handler/machine_handler_test.go internal/api/handler/mock_machine_service_test.go
git commit -m "feat(api): add machine challenge routes and handler"
```

---

### Task 7: Wire router, main, and Makefile

**Files:**
- Modify: `internal/api/router.go`
- Modify: `internal/api/router_test.go`
- Modify: `Makefile`
- Modify: `cmd/orchestrator/main.go` (no signature change; confirm it compiles)

**Interfaces:**
- Consumes: `config.Config`/`config.Load` (Task 4), `store.NewMachineStore` (Task 2), `service.NewMachineService` (Task 5), `handler.NewMachineHandler` (Task 6), `auth.*` (Task 3).
- Produces: `api.NewRouter(db *sql.DB, cfg *config.Config) (http.Handler, error)`.

- [ ] **Step 1: Update the router test**

Update `internal/api/router_test.go` (package `api_test`). Replace `LabToken` with `LabKey: []byte("0123456789abcdef0123456789abcdef")`. Add an `initiateChallenge` helper that POSTs to `/machines/authenticate/init` and returns the nonce, and a `signChallenge` helper using `auth.Sign` + `auth.CanonicalPayload` + `hex.EncodeToString`. Rewrite the authenticate test to initiate a challenge, sign it, then POST to `/machines/authenticate`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api -run TestNewRouter -v`
Expected: FAIL — `LabToken` undefined / `NewMachineService` signature mismatch.

- [ ] **Step 3: Update `internal/api/router.go`**

Change the service construction to `service.NewMachineService(machineStore, cfg.LabKey, cfg.SessionSecret, cfg.SessionTTL, time.Now)` and add the `time` import. The rest of `NewRouter` is unchanged.

- [ ] **Step 4: Verify `cmd/orchestrator/main.go`**

`main.go` already calls `config.Load()` and `api.NewRouter(db, cfg)`; no change is required beyond confirming it compiles.

- [ ] **Step 5: Update the Makefile**

Add `./internal/auth` to `TEST_PACKAGES`.

- [ ] **Step 6: Run the full suite and coverage**

Run: `make tests`
Expected: PASS.

Run: `make coverage`
Expected: all listed packages pass; total coverage at or above 75%.

- [ ] **Step 7: Commit (only if authorized)**

```bash
git add internal/api/router.go internal/api/router_test.go Makefile cmd/orchestrator/main.go
git commit -m "feat(api): wire machine registry and challenge authentication at startup"
```
