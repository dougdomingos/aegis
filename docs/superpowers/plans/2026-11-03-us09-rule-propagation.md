# US09 Rule Propagation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Distribute each group's linked policy to every machine in that group over an authenticated gRPC stream, acknowledge it durably on the Sentinel, and expose the latest propagation state through the machine API.

**Architecture:** The Orchestrator owns the desired state and assigns a per-machine sequence. A single-threaded propagation worker builds full snapshots, writes an audit row, and sends them over each machine's stream; the Sentinel validates, persists the state atomically, and ACKs or NACKs on the same stream. The HTTP API reads only the latest audit row per machine.

**Tech Stack:** Go 1.26.5, chi, SQLite, gRPC (`google.golang.org/grpc`), protobuf (`google.golang.org/protobuf`), `log/slog`.

**Spec:** `docs/superpowers/specs/2026-11-03-us09-rule-propagation-design.md`

## Global Constraints

- Module path is `dougdomingos.com/aegis`; Go version is `1.26.5`.
- Dependency flow is `API handler → service → store → SQLite`; interfaces are owned by their consumers.
- Every exported symbol carries a doc comment starting with its name. Do not add doc comments to methods that implement an interface.
- Tests live beside their targets in external packages (`service_test`, `store_test`, `handler_test`), follow `Test<Type>_<Method>_<Scenario>_<ExpectedResult>`, use Arrange-Act-Assert, and use `t.Errorf`/`t.Fatalf`.
- Table-driven tests use `map[string]struct{...}` and `t.Run`.
- **Do not commit automatically.** `AGENTS.md` overrides the skill's commit step. Every task ends by staging its files for developer review; the suggested message is included for the developer.
- `pkg/pb/**` is gitignored. Generated code is produced by `make proto` and is never committed.
- Message size limit is 16 MiB on both send and receive sides.
- Sequence is the only ordering authority. Policy version is metadata and never gates acceptance.
- The API exposes the latest propagation row only. Every sent message remains in `propagations` with its serialized payload.
- No automatic retry loop. A NACK/FAILED is recovered by the next change or reconnect.
- TLS is out of scope for this plan; see `TLS-FOLLOWUP.md`.
- Coverage gate is 75% across `Makefile`'s `TEST_PACKAGES`.

### Design details resolved during planning

These are the only intentional deviations from the design document; Task 18 updates the design text to match.

1. `domain.PropagationStore.Create` takes an encoder callback (`func(sequence int64) (string, error)`) because the sequence is assigned inside the insert transaction and the exact wire payload must contain it.
2. `propagation.Stream` lives in `internal/common/propagation`, not `client`, so the Sentinel runner's `Connector` interface and `client.ConnectPropagation` share one return type.
3. The Sentinel runner accepts a `StateStore` interface instead of `*FileStore`, so persistence failures are testable.
4. The gRPC server consumes a consumer-owned `propagationController` interface instead of the concrete `*service.PropagationService`.
5. The shared `rules.Validate` adds a `default` branch returning `errors.ErrInvalidRuleType`. `domain.Rule.Validate` currently falls through silently for unknown types; this is a deliberate tightening that makes the Orchestrator and Sentinel reject the same inputs. Existing valid inputs are unaffected.

## Review Focus

1. **A machine with no group, no policy, or an empty policy must receive a `REVOKE`, never an empty package.** Covered by Task 6's snapshot-builder tests.
2. **A stale or equal sequence must be NACKed with the state unchanged; an epoch change must accept even a lower sequence.** Covered by Task 15's `apply` tests.
3. **A reconnect that registers before the old handler unregisters must keep the new sender.** Covered by Task 6's connection-ID test.
4. **An ACK for another machine or a superseded sequence must not corrupt any row.** Covered by Task 6's `Acknowledge` test and Task 10's stream test.
5. **Deleting a linked policy or a group with machines must resolve machine IDs before the delete and enqueue the revoke only after it succeeds.** Covered by Task 7's ordering tests.

Additional failure pinned because it is likely in real deployments: **a package larger than gRPC's 4 MiB default must round-trip.** Covered by Task 10's 5 MiB `bufconn` test.

---

### Task 1: Shared Rule Validator

**Files:**
- Create: `internal/common/rules/rules.go`
- Create: `internal/common/rules/rules_test.go`
- Modify: `internal/common/errors/rule.go`
- Modify: `internal/orchestrator/domain/rule.go`

**Interfaces:**
- Consumes: `internal/common/errors`.
- Produces: `rules.Rule{Type string; Value string; Protocol *string; Port *int}` and `rules.Validate(rule Rule) error`; `errors.ErrInvalidRuleType`.
- `domain.Rule.Validate` keeps the `PolicyID > 0` check and delegates the rest.

- [ ] **Step 1: Write the failing tests**

Create `internal/common/rules/rules_test.go` in `package rules_test` with:

```go
func TestRules_Validate_WithValidDomainRule_ReturnsNil(t *testing.T)
func TestRules_Validate_WithValidIPRule_ReturnsNil(t *testing.T)
func TestRules_Validate_WithMissingType_ReturnsErrRuleTypeRequired(t *testing.T)
func TestRules_Validate_WithMissingValue_ReturnsErrRuleValueRequired(t *testing.T)
func TestRules_Validate_WithDomainCarryingProtocol_ReturnsErrInvalidFieldForRuleType(t *testing.T)
func TestRules_Validate_WithMalformedDomain_ReturnsErrMalformedRuleValue(t *testing.T)
func TestRules_Validate_WithMalformedIP_ReturnsErrMalformedRuleValue(t *testing.T)
func TestRules_Validate_WithPortOutOfRange_ReturnsErrInvalidRulePortValue(t *testing.T)
func TestRules_Validate_WithUnknownType_ReturnsErrInvalidRuleType(t *testing.T)
```

Use `rules.Rule{Type: "DOMAIN", Value: "example.com"}` and `rules.Rule{Type: "IP", Value: "127.0.0.1", Protocol: ptr("TCP"), Port: ptr(443)}`. Assert with `errors.Is`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/common/rules/...`
Expected: FAIL with `undefined: rules.Rule` / `undefined: rules.Validate`.

- [ ] **Step 3: Add `ErrInvalidRuleType` and implement the shared validator**

In `internal/common/errors/rule.go` add:

```go
// ErrInvalidRuleType is returned when a rule declares a type that is not
// supported by the system (DOMAIN or IP).
ErrInvalidRuleType = errors.New("rule type must be DOMAIN or IP")
```

Create `internal/common/rules/rules.go` with the exact validation order from `domain.Rule.Validate`: type required, value required, then the `DOMAIN`/`IP` switch. `DOMAIN` rejects `Protocol` or `Port` and checks the same domain regex. `IP` uses `net.ParseIP` and checks `Port` in `[0, 65535]`. The `default` branch returns `errors.ErrInvalidRuleType`.

In `internal/orchestrator/domain/rule.go` replace the body of `Validate` after the `PolicyID` check with:

```go
return rules.Validate(rules.Rule{
    Type:     string(r.Type),
    Value:    r.Value,
    Protocol: r.Protocol,
    Port:     r.Port,
})
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/common/rules/... ./internal/orchestrator/domain/...`
Expected: PASS. Existing domain rule tests must still pass; add `TestRule_Validate_WithoutPolicyID_ReturnsErrRulePolicyRequired` if it is missing.

- [ ] **Step 5: Stage the task for review (do not commit)**

```bash
git add internal/common/rules internal/common/errors/rule.go internal/orchestrator/domain/rule.go
```

Suggested message: `feat(common): extract shared rule validator`

---

### Task 2: Shared Propagation Wire Types

**Files:**
- Create: `internal/common/propagation/propagation.go`
- Create: `internal/common/propagation/propagation_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `propagation.Kind`, `KindPackage`, `KindRevoke`; `propagation.Rule`; `propagation.Message`; `propagation.AckStatus`, `AckAcked`, `AckNacked`; `propagation.Ack`; `propagation.Stream`; `propagation.PolicyTypeWhitelist`, `propagation.PolicyTypeBlacklist`.

- [ ] **Step 1: Write the failing tests**

Create `internal/common/propagation/propagation_test.go` in `package propagation_test`:

```go
func TestPropagation_Message_JSONRoundTrip_PreservesPackage(t *testing.T)
func TestPropagation_Message_JSONRoundTrip_PreservesRevoke(t *testing.T)
func TestPropagation_Kind_Constants_HaveWireValues(t *testing.T)
func TestPropagation_AckStatus_Constants_HaveWireValues(t *testing.T)
func TestPropagation_PolicyType_Constants_HaveWireValues(t *testing.T)
```

The package round-trip asserts `Epoch`, `Sequence`, `MachineID`, `Kind`, `PolicyID`, `PolicyName`, `PolicyVersion`, `PolicyType`, and a rule with protocol and port. The revoke round-trip asserts an empty policy and empty rules. Pin `KindPackage == "PACKAGE"`, `KindRevoke == "REVOKE"`, `AckAcked == "ACKED"`, `AckNacked == "NACKED"`, `PolicyTypeWhitelist == "WHITELIST"`, `PolicyTypeBlacklist == "BLACKLIST"`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/common/propagation/...`
Expected: FAIL with undefined symbols.

- [ ] **Step 3: Implement the transport-neutral types**

Create `internal/common/propagation/propagation.go` with:

```go
type Kind string
const (
    KindPackage Kind = "PACKAGE"
    KindRevoke  Kind = "REVOKE"
)

type Rule struct {
    Type     string  `json:"type"`
    Value    string  `json:"value"`
    Protocol *string `json:"protocol,omitempty"`
    Port     *int    `json:"port,omitempty"`
}

type Message struct {
    Epoch         string `json:"epoch"`
    Sequence      int64  `json:"sequence"`
    MachineID     int64  `json:"machine_id"`
    Kind          Kind   `json:"kind"`
    PolicyID      int64  `json:"policy_id,omitempty"`
    PolicyName    string `json:"policy_name,omitempty"`
    PolicyVersion int    `json:"policy_version,omitempty"`
    PolicyType    string `json:"policy_type,omitempty"`
    Rules         []Rule `json:"rules,omitempty"`
}

type AckStatus string
const (
    AckAcked  AckStatus = "ACKED"
    AckNacked AckStatus = "NACKED"
)

// Policy type wire values shared by the Orchestrator and the Sentinel.
const (
    PolicyTypeWhitelist = "WHITELIST"
    PolicyTypeBlacklist = "BLACKLIST"
)

type Ack struct {
    Sequence int64     `json:"sequence"`
    Status   AckStatus `json:"status"`
    Reason   string    `json:"reason,omitempty"`
}

// Stream is the ordered, bidirectional channel a Sentinel uses to receive
// messages and send acknowledgements.
type Stream interface {
    Recv() (*Message, error)
    Send(ack Ack) error
    CloseSend() error
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/common/propagation/...`
Expected: PASS.

- [ ] **Step 5: Stage the task for review (do not commit)**

```bash
git add internal/common/propagation
```

Suggested message: `feat(common): add transport-neutral propagation types`

---

### Task 3: Proto Contract and Generation Tooling

**Files:**
- Create: `proto/propagation.proto`
- Modify: `Makefile`
- Modify: `go.mod`, `go.sum`
- Modify: `.github/workflows/ci.yml`
- Modify: `e2e/Dockerfile`

**Interfaces:**
- Produces: generated package `dougdomingos.com/aegis/pkg/pb` with `PropagationClient`, `PropagationServer`, `Propagation_ConnectClient`, `Propagation_ConnectServer`, `Ack`, `AckStatus`, `ServerMessage`, `PolicyPackage`, `Revoke`, `Rule`.
- Produces: `make proto`.

- [ ] **Step 1: Write the proto contract**

Create `proto/propagation.proto` with exactly the schema from design §5, including `option go_package = "dougdomingos.com/aegis/pkg/pb;pb";`. Keep the `oneof payload` and the `optional` protocol/port fields.

- [ ] **Step 2: Add the generation target and prerequisites**

In `Makefile`:
- Add `proto` to `.PHONY`.
- Add a `proto` target that creates `pkg/pb` and runs:

```sh
protoc --proto_path=proto \
  --go_out=pkg/pb --go_opt=paths=source_relative \
  --go-grpc_out=pkg/pb --go-grpc_opt=paths=source_relative \
  proto/propagation.proto
```

- Make `build`, `tests`, `coverage`, `test-report`, and `e2e` depend on `proto`.
- Add `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest` and `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest` to `init`.

- [ ] **Step 3: Generate and add the gRPC dependencies**

Run: `make proto`
Expected: `pkg/pb/propagation.pb.go` and `pkg/pb/propagation_grpc.pb.go` exist.

Run: `go get google.golang.org/grpc@latest google.golang.org/protobuf@latest`
Run: `go mod tidy`
Expected: `go.mod` lists both modules as direct requirements.

- [ ] **Step 4: Update CI and the e2e build image**

In `.github/workflows/ci.yml`:
- Add `proto/**` and `Makefile` to both `push.paths` and `pull_request.paths`.
- Add this step after the `setup-go` step in **both** the `test` and `e2e` jobs, before `make coverage` / `make e2e`:

```yaml
      - name: Install protobuf toolchain
        run: |
          sudo apt-get update
          sudo apt-get install -y protobuf-compiler
          go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
          go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
          echo "$(go env GOPATH)/bin" >> "$GITHUB_PATH"
```

In `e2e/Dockerfile`, in the build stage:
- After `COPY go.mod go.sum ./` and `RUN go mod download`, install the compiler and both plugins:

```dockerfile
RUN apk add --no-cache protobuf protobuf-dev make
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest \
 && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
ENV PATH="/root/go/bin:${PATH}"
```

- After `COPY . .`, run `RUN make proto` before the `go build` commands.

- [ ] **Step 5: Verify the generated package builds**

Run: `go build ./pkg/pb`
Expected: PASS.

- [ ] **Step 6: Stage the task for review (do not commit)**

```bash
git add proto Makefile go.mod go.sum .github/workflows/ci.yml e2e/Dockerfile
```

Suggested message: `build: add propagation proto and generation tooling`

---

### Task 4: Propagation Migrations, Domain, and Store

**Files:**
- Create: `internal/orchestrator/infra/migrations/sql/007_create_propagation_table.sql`
- Create: `internal/orchestrator/infra/migrations/sql/008_create_system_metadata_table.sql`
- Create: `internal/orchestrator/domain/propagation.go`
- Create: `internal/orchestrator/store/propagation_store.go`
- Create: `internal/orchestrator/store/propagation_store_test.go`

**Interfaces:**
- Produces: `domain.PropagationKind`, `PropagationPackage`, `PropagationRevoke`; `domain.PropagationStatus`, `PropagationPending`, `PropagationSent`, `PropagationAcked`, `PropagationNacked`, `PropagationFailed`; `domain.Propagation`; `domain.PropagationStore`.
- Produces: `store.NewPropagationStore(db *sql.DB) *PropagationStore`.
- `domain.PropagationStore.Create` is:

```go
Create(ctx context.Context, propagation Propagation, encode func(sequence int64) (string, error)) (*Propagation, error)
```

- Other methods: `MarkSent(ctx, machineID, sequence int64, at time.Time) error`, `MarkFailed(ctx, machineID, sequence int64, reason string) error`, `Acknowledge(ctx, machineID, sequence int64, status PropagationStatus, reason string, at time.Time) error`, `LatestByMachine(ctx, machineID int64) (*Propagation, error)`, `ListLatest(ctx) ([]Propagation, error)`, `Epoch(ctx) (string, error)`.

- [ ] **Step 1: Write the failing store tests**

Create `internal/orchestrator/store/propagation_store_test.go` in `package store_test`, using `arrangeStoreTest(t, store.NewPropagationStore)`:

```go
func TestPropagationStore_Create_WithMultipleMachines_AssignsIndependentSequences(t *testing.T)
func TestPropagationStore_Create_WithNewRow_StartsPendingAndStoresPayload(t *testing.T)
func TestPropagationStore_Create_WhenEncoderFails_RollsBack(t *testing.T)
func TestPropagationStore_MarkSent_UpdatesOnlyTargetRow(t *testing.T)
func TestPropagationStore_MarkFailed_SetsReasonAndLeavesAckedAtNil(t *testing.T)
func TestPropagationStore_Acknowledge_WithMatchingRow_SetsStatusAndTimestamps(t *testing.T)
func TestPropagationStore_Acknowledge_WithMissingRow_ReturnsNil(t *testing.T)
func TestPropagationStore_LatestByMachine_ReturnsHighestSequence(t *testing.T)
func TestPropagationStore_ListLatest_ReturnsOnlyHighestSequencePerMachine(t *testing.T)
func TestPropagationStore_Epoch_ReturnsStableThirtyTwoHexCharacters(t *testing.T)
func TestPropagationStore_Epoch_AfterMetadataDeletion_ReturnsNewValue(t *testing.T)
```

The `Create` payload encoder must be `func(sequence int64) (string, error) { return fmt.Sprintf("payload-%d", sequence), nil }`; assert the stored `Payload == "payload-1"` and the returned `Sequence == 1`. For the rollback case, return an error from the encoder and assert no rows exist.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/orchestrator/store/... -run Propagation`
Expected: FAIL with undefined symbols.

- [ ] **Step 3: Add the migrations**

Create `007_create_propagation_table.sql` exactly as design §6.1, including the unique `(machine_id, sequence)` constraint and the descending index. Create `008_create_system_metadata_table.sql` exactly as design §6.2.

- [ ] **Step 4: Add the domain types and store implementation**

Create `internal/orchestrator/domain/propagation.go` with the types and interface from design §6.3/§6.4, using the callback signature above.

Implement `internal/orchestrator/store/propagation_store.go`:
- `Create` starts a transaction, reads `COALESCE(MAX(sequence), 0) + 1` for the machine, calls `encode(sequence)`, inserts the row with `status = 'PENDING'`, `payload`, and the metadata, then reads the inserted row back inside the transaction.
- `MarkSent` sets `status = 'SENT'`, `sent_at = ?`.
- `MarkFailed` sets `status = 'FAILED'`, `nack_reason = ?`; do not touch `acked_at`.
- `Acknowledge` updates `status`, `acked_at`, `nack_reason` by `(machine_id, sequence)`; zero rows affected is not an error.
- `LatestByMachine` orders by `sequence DESC LIMIT 1`.
- `ListLatest` joins each machine to its maximum sequence.
- Status reads do not select `payload`.
- `Epoch` selects `propagation_epoch`; when absent, generates 16 random bytes with `crypto/rand`, hex-encodes them, inserts with `INSERT OR IGNORE`, and re-selects.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/orchestrator/store/... -run Propagation`
Expected: PASS.

- [ ] **Step 6: Stage the task for review (do not commit)**

```bash
git add internal/orchestrator/infra/migrations/sql/007_create_propagation_table.sql \
  internal/orchestrator/infra/migrations/sql/008_create_system_metadata_table.sql \
  internal/orchestrator/domain/propagation.go \
  internal/orchestrator/store/propagation_store.go \
  internal/orchestrator/store/propagation_store_test.go
```

Suggested message: `feat(store): add propagation audit table and store`

---

### Task 5: Group Store `ListByPolicy`

**Files:**
- Modify: `internal/orchestrator/domain/group.go`
- Modify: `internal/orchestrator/store/group_store.go`
- Modify: `internal/orchestrator/store/group_store_test.go`
- Modify: `internal/orchestrator/service/mock_group_store_test.go`

**Interfaces:**
- Produces: `domain.GroupStore.ListByPolicy(ctx context.Context, policyID int64) ([]Group, error)` and its `store.GroupStore` implementation.

- [ ] **Step 1: Write the failing test**

Add to `internal/orchestrator/store/group_store_test.go`:

```go
func TestGroupStore_ListByPolicy_WithLinkedGroups_ReturnsOnlyLinkedGroups(t *testing.T)
```

Seed two groups, link one to a policy, and assert the result contains exactly the linked group with its `PolicyName` populated.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/orchestrator/store/... -run ListByPolicy`
Expected: FAIL with `ListByPolicy undefined`.

- [ ] **Step 3: Implement the query**

Add to `domain.GroupStore`:

```go
// ListByPolicy retrieves every group linked to the policy whose ID matches
// the provided argument.
ListByPolicy(ctx context.Context, policyID int64) ([]Group, error)
```

In `store/group_store.go` add:

```sql
SELECT g.id, g.name, g.created_at, g.policy_id, p.name
FROM groups g LEFT JOIN policies p ON p.id = g.policy_id
WHERE g.policy_id = ?
```

Return `store.executor.QueryMany(ctx, query, policyID)`.

- [ ] **Step 4: Add `ListByPolicy` to the service test mock**

`MockGroupStore` in `internal/orchestrator/service/mock_group_store_test.go` implements `domain.GroupStore`, so it must gain the new method or the service tests stop compiling. Add:

```go
// ListByPolicy returns the groups linked to the provided policy.
func (mock *MockGroupStore) ListByPolicy(ctx context.Context, policyID int64) ([]domain.Group, error) {
    if mock.StoreErr != nil {
        return nil, mock.StoreErr
    }

    linked := make([]domain.Group, 0)
    for _, group := range mock.groups {
        if group.PolicyID != nil && *group.PolicyID == policyID {
            linked = append(linked, *group)
        }
    }

    return linked, nil
}
```

Use the mock's existing map and error field names; adjust the snippet to match the struct.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/orchestrator/store/... -run ListByPolicy`
Run: `go test ./internal/orchestrator/service/...`
Expected: PASS.

- [ ] **Step 6: Stage the task for review (do not commit)**

```bash
git add internal/orchestrator/domain/group.go internal/orchestrator/store/group_store.go internal/orchestrator/store/group_store_test.go internal/orchestrator/service/mock_group_store_test.go
```

Suggested message: `feat(store): list groups by linked policy`

---

### Task 6: Propagation Service

**Files:**
- Create: `internal/orchestrator/service/propagation_notifier.go`
- Create: `internal/orchestrator/service/propagation_service.go`
- Create: `internal/orchestrator/service/propagation_service_test.go`
- Create: `internal/orchestrator/service/mock_propagation_store_test.go`

**Interfaces:**
- Consumes: `domain.MachineStore` (`GetByID`), `domain.GroupStore` (`ListByPolicy`, `GetByID`), `domain.PolicyStore` (`GetByID`), `domain.RuleStore` (`List` with `domain.RuleFilter{PolicyID: ...}`), `domain.PropagationStore` (Task 4), `domain.GroupStore.ListByPolicy` (Task 5), `propagation.Message`/`Ack`/`Kind` (Task 2).
- Produces: `service.PropagationNotifier`; `service.MessageSender`; `service.NewPropagationService(...)`; `Start`, `Register`, `Unregister`, `DeliverSnapshot`, `Acknowledge`, and the notifier methods.

- [ ] **Step 1: Write the notifier contract**

Create `internal/orchestrator/service/propagation_notifier.go`:

```go
type PropagationNotifier interface {
    NotifyPolicy(ctx context.Context, policyID int64)
    NotifyGroup(ctx context.Context, groupID int64)
    NotifyMachine(ctx context.Context, machineID int64)
    ResolvePolicyMachines(ctx context.Context, policyID int64) ([]int64, error)
    ResolveGroupMachines(ctx context.Context, groupID int64) ([]int64, error)
    RevokeMachines(ctx context.Context, machineIDs []int64, policy *domain.Policy)
}
```

Add the doc comment explaining that mutation methods are best effort and the resolve methods are read-only and may fail.

- [ ] **Step 2: Write the failing service tests and the propagation store mock**

Create `internal/orchestrator/service/mock_propagation_store_test.go` in `package service_test` with an in-memory implementation of `domain.PropagationStore`. It must expose `EpochValue string`, `EpochErr error`, `CreateErr error`, `Rows map[int64][]domain.Propagation`, and a `CreateCalls int` counter. `Create` assigns the next sequence per machine, calls `encode`, appends the row with `PropagationPending`, and returns it.

Create `internal/orchestrator/service/propagation_service_test.go` with:

```go
func TestPropagationService_Start_WithEpochError_ReturnsErrorAndDoesNotLaunch(t *testing.T)
func TestPropagationService_NotifyPolicy_WithLinkedGroups_EnqueuesEveryMachine(t *testing.T)
func TestPropagationService_NotifyGroup_WithMachines_EnqueuesEveryMachine(t *testing.T)
func TestPropagationService_NotifyMachine_WithMachine_EnqueuesOnlyThatMachine(t *testing.T)
func TestPropagationService_LatestIntentWins_PackageThenRevoke(t *testing.T)
func TestPropagationService_LatestIntentWins_RevokeThenPackage(t *testing.T)
func TestPropagationService_DeliverSnapshot_WithCompleteLinks_SendsPackage(t *testing.T)
func TestPropagationService_DeliverSnapshot_WithoutGroup_SendsRevoke(t *testing.T)
func TestPropagationService_DeliverSnapshot_WithoutPolicy_SendsRevoke(t *testing.T)
func TestPropagationService_DeliverSnapshot_WithEmptyRules_SendsRevoke(t *testing.T)
func TestPropagationService_DeliverSnapshot_WhenSendSucceeds_MarksSent(t *testing.T)
func TestPropagationService_DeliverSnapshot_WithoutSender_DoesNotCreateRow(t *testing.T)
func TestPropagationService_DeliverSnapshot_WhenMarkSentFails_DoesNotSend(t *testing.T)
func TestPropagationService_DeliverSnapshot_WhenSendFails_MarksFailed(t *testing.T)
func TestPropagationService_Register_AfterReconnect_OldUnregisterIsNoop(t *testing.T)
func TestPropagationService_Acknowledge_WithForeignSequence_LeavesRowsUntouched(t *testing.T)
func TestPropagationService_RevokeMachines_WithPolicy_RecordsPolicyMetadata(t *testing.T)
```

Use a `fakeSender` with a buffered `sent chan propagation.Message` and an injectable `Err`. Use `waitForSend(t, ch)` and `waitForCondition(t, func() bool, timeout)` helpers. The snapshot tests call `DeliverSnapshot` and assert what the sender receives; do not test the unexported `buildMessage` or `dispatch` directly. For the intent tests, register the sender and seed stores before calling `Start`, enqueue both intents, then `Start`; do not call `DeliverSnapshot`, so the pending intent is the only send. For `Start_WithEpochError`, inject `EpochErr` and assert that a subsequently enqueued intent is never sent.

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/orchestrator/service/... -run PropagationService`
Expected: FAIL with undefined symbols.

- [ ] **Step 4: Implement `PropagationService`**

Create `internal/orchestrator/service/propagation_service.go` with the constructor from design §7.4, plus:

```go
type MessageSender interface {
    Send(ctx context.Context, message propagation.Message) error
}

type propagationIntent struct {
    kind   propagation.Kind
    policy *domain.Policy
}

type registeredSender struct {
    id     uint64
    sender MessageSender
}

// builtMessage couples the wire message with the policy metadata recorded in
// the audit row. For a revoke, message.Policy* is empty while the audit fields
// may still carry the policy being removed.
type builtMessage struct {
    message       propagation.Message
    policyID      int64
    policyName    string
    policyVersion int
    policyType    string
}
```

Struct fields: the five stores, `logger`, `now`, `epoch`, `mu sync.Mutex`, `pending map[int64]propagationIntent`, `senders map[int64]registeredSender`, `nextConnID uint64`, `wake chan struct{}` (capacity 1). Use a discard logger when `logger == nil`.

Implement:
- `Start(ctx)`: `epoch, err := store.Epoch(ctx)`; on error return it without launching the worker or storing the epoch; otherwise store the epoch, launch `go s.run(ctx)`, and return nil.
- `Register(machineID, sender)`: `machines.GetByID`; nil → `ErrMachineNotFound`; lock, increment `nextConnID`, store the sender, return the ID.
- `Unregister(machineID, connID)`: delete only when the stored ID matches.
- `DeliverSnapshot(ctx, machineID)`: enqueue a package intent.
- `NotifyPolicy`: `groups.ListByPolicy`, then `machines.ListByGroup` for each group, enqueue package intents. Log resolution errors.
- `NotifyGroup`: `machines.ListByGroup`, enqueue package intents.
- `NotifyMachine`: enqueue a package intent.
- `ResolvePolicyMachines` / `ResolveGroupMachines`: the same resolution without enqueueing; return the error.
- `RevokeMachines`: enqueue a revoke intent carrying the optional policy copy.
- `enqueue`: lock, replace `pending[machineID]`, non-blocking send on `wake`.
- `run`: select on `ctx.Done()` and `wake`; on wake, take and clear a snapshot of `pending`, then process each machine in turn.
- `dispatch`: look up the sender; if absent, drop the intent without creating a row. Call `buildMessage`; a nil result means the machine was removed, so drop the intent. Build a `domain.Propagation` carrying the machine ID, kind, audit metadata (`built.policyID`, `built.policyName`, `built.policyVersion`, `built.policyType`), and call `store.Create(ctx, row, func(sequence int64) (string, error) { built.message.Sequence = sequence; payload, err := json.Marshal(built.message); return string(payload), err })`. If `Create` fails, log and skip the send. Call `MarkSent(machineID, created.Sequence, now)`; if it fails, log and skip the send (never send without a `SENT` row). Then `sender.Send` with a 10-second context. On send error, call `MarkFailed(machineID, created.Sequence, err.Error())`.
- `buildMessage`: return `(*builtMessage, error)`. For a revoke, return a bare `REVOKE` message and copy the intent's policy metadata into the audit fields. For a package, `machines.GetByID` (gone → `nil, nil`); when `machine.GroupID == nil` → revoke; `groups.GetByID(*machine.GroupID)` (missing → revoke); when `group.PolicyID == nil` → revoke; `policies.GetByID(*group.PolicyID)` (missing → revoke); `rules.List(ctx, domain.RuleFilter{PolicyID: policy.ID})` (empty → revoke). Otherwise return a `PACKAGE` with policy ID, name, version, type, every rule mapped to `propagation.Rule`, and the same policy metadata in the audit fields.
- `Acknowledge`: map `AckAcked`/`AckNacked` to the domain status; ignore any other status; call `store.Acknowledge`.

Add `var _ PropagationNotifier = (*PropagationService)(nil)`.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/orchestrator/service/... -run PropagationService`
Expected: PASS.

- [ ] **Step 6: Stage the task for review (do not commit)**

```bash
git add internal/orchestrator/service/propagation_notifier.go \
  internal/orchestrator/service/propagation_service.go \
  internal/orchestrator/service/propagation_service_test.go \
  internal/orchestrator/service/mock_propagation_store_test.go
```

Suggested message: `feat(service): add propagation fan-out and worker`

---

### Task 7: Domain Service Hooks

**Files:**
- Modify: `internal/orchestrator/service/group_service.go`
- Modify: `internal/orchestrator/service/policy_service.go`
- Modify: `internal/orchestrator/service/rule_service.go`
- Modify: `internal/orchestrator/api/router.go` (temporary nil notifier until Task 11)
- Modify: `internal/orchestrator/service/group_service_test.go`
- Modify: `internal/orchestrator/service/policy_service_test.go`
- Modify: `internal/orchestrator/service/rule_service_test.go`
- Create: `internal/orchestrator/service/mock_propagation_notifier_test.go`

**Interfaces:**
- Consumes: `service.PropagationNotifier` (Task 6).
- Produces: `NewGroupService(store, machineStore, policyStore, ruleStore, notifier, stalenessGrace, now)`, `NewPolicyService(store, notifier)`, `NewRuleService(store, policies, groups, notifier)`.

- [ ] **Step 1: Write the mock notifier and failing hook tests**

Create `internal/orchestrator/service/mock_propagation_notifier_test.go` with a `MockPropagationNotifier` that records calls in order and allows injected `ResolvePolicyMachinesErr`/`ResolveGroupMachinesErr` and return IDs.

Add tests:

```go
func TestRuleService_CreateRule_WithNotifier_NotifiesPolicy(t *testing.T)
func TestRuleService_UpdateRule_WhenChanged_NotifiesPolicy(t *testing.T)
func TestRuleService_UpdateRule_WhenUnchanged_DoesNotNotify(t *testing.T)
func TestRuleService_RemoveRule_WithNotifier_NotifiesPolicy(t *testing.T)
func TestGroupService_LinkMachine_WithNotifier_NotifiesMachine(t *testing.T)
func TestGroupService_UnlinkMachine_WithNotifier_NotifiesMachine(t *testing.T)
func TestGroupService_LinkPolicy_WithNotifier_NotifiesGroup(t *testing.T)
func TestGroupService_UnlinkPolicy_WithNotifier_NotifiesGroup(t *testing.T)
func TestGroupService_RemoveGroup_ResolvesBeforeDeleteAndRevokesAfter(t *testing.T)
func TestGroupService_RemoveGroup_WhenResolveFails_DoesNotDelete(t *testing.T)
func TestGroupService_RemoveGroup_WhenDeleteFails_DoesNotRevoke(t *testing.T)
func TestPolicyService_UpdatePolicy_WhenRenamed_NotifiesPolicy(t *testing.T)
func TestPolicyService_UpdatePolicy_WhenUnchanged_DoesNotNotify(t *testing.T)
func TestPolicyService_RemovePolicy_ResolvesBeforeDeleteAndRevokesAfter(t *testing.T)
func TestPolicyService_RemovePolicy_WhenResolveFails_DoesNotDelete(t *testing.T)
func TestPolicyService_RemovePolicy_WhenDeleteFails_DoesNotRevoke(t *testing.T)
```

For ordering tests, record `"resolve"`, `"remove"`, and `"revoke"` into one slice from the notifier and the mock store, then assert the slice equals `["resolve", "remove", "revoke"]`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/orchestrator/service/... -run 'Notifies|RemoveGroup|RemovePolicy|UpdatePolicy'`
Expected: FAIL because the constructors do not accept a notifier.

- [ ] **Step 3: Add the notifier dependency and hooks**

In each service, add a `notifier PropagationNotifier` field and the constructor parameter. Allow `nil` and guard every call with `if service.notifier == nil { return }`.

Hook placement:
- `RuleService.CreateRule` after `store.Create`: `NotifyPolicy(p.PolicyID)`.
- `RuleService.UpdateRule` only after `store.Update`: `NotifyPolicy(p.PolicyID)`. The `IsEqual` path returns before this.
- `RuleService.RemoveRule` after `store.Remove`: `NotifyPolicy(rule.PolicyID)`.
- `GroupService.LinkMachine` after `LinkToGroup`: `NotifyMachine(machineID)`.
- `GroupService.UnlinkMachine` after `UnlinkFromGroup`: `NotifyMachine(machineID)`.
- `GroupService.LinkPolicy` after `LinkToPolicy`: `NotifyGroup(group.ID)`.
- `GroupService.UnlinkPolicy` after `UnlinkFromPolicy`: `NotifyGroup(group.ID)`.
- `GroupService.RemoveGroup`: replace the `Exists` check with `GetByName` so the ID is available. Call `ResolveGroupMachines(group.ID)` before `store.Remove`; return on error. Call `store.Remove`; return on error. Then call `RevokeMachines(ids, nil)`.
- `PolicyService.UpdatePolicy` only after `store.Update`: `NotifyPolicy(policy.ID)`. The same-name path returns before this.
- `PolicyService.RemovePolicy`: call `ResolvePolicyMachines(policy.ID)` before `store.Remove`; return on error. Call `store.Remove`; return on error. Then call `RevokeMachines(ids, policy)`.

Update `api/router.go` to pass `nil` as the notifier to the three constructors so the repository still builds. Task 11 replaces it with the real service.

Update the existing arrange helpers to pass `nil` or the mock notifier, and keep all existing tests passing.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/orchestrator/service/... ./internal/orchestrator/api/...`
Expected: PASS.

- [ ] **Step 5: Stage the task for review (do not commit)**

```bash
git add internal/orchestrator/service/group_service.go \
  internal/orchestrator/service/policy_service.go \
  internal/orchestrator/service/rule_service.go \
  internal/orchestrator/service/*_test.go \
  internal/orchestrator/api/router.go
```

Suggested message: `feat(service): notify propagation on policy, group, and rule changes`

---

### Task 8: Machine Propagation Schema and Service

**Files:**
- Modify: `internal/common/schemas/machine.go`
- Modify: `internal/orchestrator/service/machine_service.go`
- Modify: `internal/orchestrator/service/machine_service_test.go`
- Modify: `internal/orchestrator/api/router.go` (pass the propagation store)

**Interfaces:**
- Consumes: `domain.PropagationStore` (Task 4).
- Produces: `schemas.PropagationOutputSchema`; `schemas.MachineOutputSchema.Propagation`; `NewMachineService(machineStore, propagationStore, labKey, sessionSecret, sessionTTL, stalenessGrace, now)`; `MachineService.GetPropagation(ctx, machineID int64) (*schemas.PropagationOutputSchema, error)`.

- [ ] **Step 1: Write the failing service tests**

Add to `internal/orchestrator/service/machine_service_test.go`:

```go
func TestMachineService_ListMachines_WithLatestPropagation_MergesIntoOutput(t *testing.T)
func TestMachineService_ListMachines_WithoutPropagationRow_ReportsPending(t *testing.T)
func TestMachineService_ListMachines_WhenPropagationStoreFails_ReturnsError(t *testing.T)
func TestMachineService_GetPropagation_WithAckedRow_ReturnsTimestampsAndDuration(t *testing.T)
func TestMachineService_GetPropagation_WithoutRow_ReturnsPending(t *testing.T)
func TestMachineService_GetPropagation_WithMissingMachine_ReturnsErrMachineNotFound(t *testing.T)
func TestMachineService_GetPropagation_WhenStoreFails_ReturnsError(t *testing.T)
```

Seed rows with `MockPropagationStore.Create` and an encoder that returns `""`. For the ACKED case, use `sent_at = 2026-01-01T00:00:00Z` and `acked_at = 2026-01-01T00:00:02.500Z`; assert `DurationMS == 2500`. For PENDING, assert `Sequence == 0`, `Kind == ""`, `Status == "PENDING"`, `PolicyID == nil`, and all timestamps/duration nil.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/orchestrator/service/... -run 'ListMachines|GetPropagation'`
Expected: FAIL with undefined `GetPropagation` and constructor mismatch.

- [ ] **Step 3: Add the schema**

In `internal/common/schemas/machine.go` add:

```go
// PropagationOutputSchema declares the latest propagation state of a machine.
type PropagationOutputSchema struct {
    Sequence      int64      `json:"sequence"`
    Kind          string     `json:"kind"`
    Status        string     `json:"status"`
    PolicyID      *int64     `json:"policy_id"`
    PolicyName    string     `json:"policy_name"`
    PolicyVersion int        `json:"policy_version"`
    PolicyType    string     `json:"policy_type"`
    SentAt        *time.Time `json:"sent_at"`
    AckedAt       *time.Time `json:"acked_at"`
    DurationMS    *int64     `json:"duration_ms"`
    NackReason    string     `json:"nack_reason"`
}
```

Add `Propagation PropagationOutputSchema `json:"propagation"`` to `MachineOutputSchema`. Leave the existing fields unchanged (they have no tags).

- [ ] **Step 4: Implement the merge and `GetPropagation`**

In `MachineService`:
- Add `propagations domain.PropagationStore` and the constructor parameter after `machineStore`.
- `ListMachines` calls `propagations.ListLatest`, builds `map[int64]domain.Propagation`, and attaches the mapped schema or the PENDING value to each machine.
- `GetPropagation` loads the machine with `store.GetByID`; nil → `machineErrors.ErrMachineNotFound`. Then `propagations.LatestByMachine`; no row → PENDING. Otherwise map the row.
- `mapPropagationToOutputSchema` copies metadata and sets `DurationMS` only when both `SentAt` and `AckedAt` are non-nil: `ms := p.AckedAt.Sub(*p.SentAt).Milliseconds()`.
- `pendingPropagationOutput` returns the exact design §7.7 JSON shape.

Update `api/router.go` to create `store.NewPropagationStore(db)` and pass it to `NewMachineService`.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/orchestrator/service/... ./internal/orchestrator/api/...`
Expected: PASS.

- [ ] **Step 6: Stage the task for review (do not commit)**

```bash
git add internal/common/schemas/machine.go \
  internal/orchestrator/service/machine_service.go \
  internal/orchestrator/service/machine_service_test.go \
  internal/orchestrator/api/router.go
```

Suggested message: `feat(service): expose latest propagation on machines`

---

### Task 9: HTTP Propagation Endpoint

**Files:**
- Modify: `internal/orchestrator/api/handler/machine_handler.go`
- Modify: `internal/orchestrator/api/handler/machine_handler_test.go`
- Modify: `internal/orchestrator/api/handler/mock_machine_service_test.go`

**Interfaces:**
- Consumes: `MachineService.GetPropagation` (Task 8).
- Produces: `GET /machines/{id}/propagation`; `MachineServiceInterface.GetPropagation`.

- [ ] **Step 1: Write the failing handler tests**

Add to `internal/orchestrator/api/handler/machine_handler_test.go`:

```go
func TestMachineHandler_GetPropagation_WithKnownMachine_ReturnsOK(t *testing.T)
func TestMachineHandler_GetPropagation_WithPendingMachine_ReturnsOK(t *testing.T)
func TestMachineHandler_GetPropagation_WithInvalidID_ReturnsBadRequest(t *testing.T)
func TestMachineHandler_GetPropagation_WithMissingMachine_ReturnsNotFound(t *testing.T)
func TestMachineHandler_GetPropagation_WhenServiceFails_ReturnsInternalServerError(t *testing.T)
```

Add `getPropagationFn` to `mockMachineService` and implement `GetPropagation`. Per `AGENTS.md`, assert only HTTP status codes.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/orchestrator/api/handler/... -run GetPropagation`
Expected: FAIL with `GetPropagation undefined`.

- [ ] **Step 3: Implement the handler and route**

Add to `MachineServiceInterface`:

```go
GetPropagation(ctx context.Context, machineID int64) (*schemas.PropagationOutputSchema, error)
```

In `RegisterRoutes`, add `r.Get("/{id}/propagation", handler.GetPropagation)` after the list route.

Implement `GetPropagation`:

```go
func (handler *MachineHandler) GetPropagation(w http.ResponseWriter, r *http.Request) {
    id, ok := parseMachineID(w, r, "id")
    if !ok {
        return
    }

    res, err := handler.service.GetPropagation(r.Context(), id)
    if err != nil {
        utils.MapByError(w, err, map[error]int{
            errors.ErrMachineNotFound: http.StatusNotFound,
        })
        return
    }

    utils.EncodeToJSON(w, http.StatusOK, res)
}
```

`parseMachineID` already exists in `group_handler.go` in the same package.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/orchestrator/api/handler/...`
Expected: PASS.

- [ ] **Step 5: Stage the task for review (do not commit)**

```bash
git add internal/orchestrator/api/handler/machine_handler.go \
  internal/orchestrator/api/handler/machine_handler_test.go \
  internal/orchestrator/api/handler/mock_machine_service_test.go
```

Suggested message: `feat(api): add machine propagation endpoint`

---

### Task 10: gRPC Propagation Server

**Files:**
- Create: `internal/orchestrator/api/grpcserver/server.go`
- Create: `internal/orchestrator/api/grpcserver/sender.go`
- Create: `internal/orchestrator/api/grpcserver/convert.go`
- Create: `internal/orchestrator/api/grpcserver/server_test.go`
- Create: `internal/orchestrator/api/grpcserver/convert_internal_test.go`

**Interfaces:**
- Consumes: `service.MessageSender`, `propagation.Message`/`Ack` (Tasks 2 and 6), generated `pkg/pb` (Task 3), `auth.ParseSessionToken`.
- Produces: `grpcserver.NewPropagationServer(service propagationController, sessionSecret string, logger *slog.Logger) *PropagationServer`; `Connect(stream pb.Propagation_ConnectServer) error`.

- [ ] **Step 1: Write the failing server tests**

Create `internal/orchestrator/api/grpcserver/server_test.go` in `package grpcserver_test`. Use `bufconn`, a real `grpc.NewServer` with `grpc.MaxRecvMsgSize(16<<20)` and `grpc.MaxSendMsgSize(16<<20)`, and a `mockController` implementing the consumer-owned interface. Tests:

```go
func TestPropagationServer_Connect_WithValidToken_RegistersAndDeliversSnapshot(t *testing.T)
func TestPropagationServer_Connect_WithMissingToken_ReturnsUnauthenticated(t *testing.T)
func TestPropagationServer_Connect_WithMalformedToken_ReturnsUnauthenticated(t *testing.T)
func TestPropagationServer_Connect_WithRemovedMachine_ReturnsUnauthenticated(t *testing.T)
func TestPropagationServer_Connect_WithAck_ForwardsAck(t *testing.T)
func TestPropagationServer_Connect_WhenStreamEnds_Unregisters(t *testing.T)
func TestPropagationServer_Connect_WithFiveMiBMessage_RoundTrips(t *testing.T)
```

Generate tokens with `auth.SignSessionToken(42, sessionSecret, time.Hour, time.Now())`. The mock's `DeliverSnapshot` should call the registered sender's `Send` with a message; the client asserts it receives it. The 5 MiB test sends one rule whose `Value` is `strings.Repeat("a", 5<<20)`.

Also create `internal/orchestrator/api/grpcserver/convert_internal_test.go` in `package grpcserver` (internal, because the conversion helpers are unexported) with:

```go
func TestConvert_ToProtoMessage_WithPackage_MapsPolicyAndRules(t *testing.T)
func TestConvert_ToProtoMessage_WithRevoke_MapsRevoke(t *testing.T)
func TestConvert_FromProtoAck_WithKnownStatuses_MapsToDomain(t *testing.T)
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/orchestrator/api/grpcserver/...`
Expected: FAIL with undefined symbols.

- [ ] **Step 3: Implement the consumer-owned interface and server**

In `server.go`:

```go
type propagationController interface {
    Register(machineID int64, sender service.MessageSender) (uint64, error)
    Unregister(machineID int64, connID uint64)
    DeliverSnapshot(ctx context.Context, machineID int64)
    Acknowledge(ctx context.Context, machineID int64, ack propagation.Ack)
}

type PropagationServer struct {
    pb.UnimplementedPropagationServer
    service       propagationController
    sessionSecret string
    logger        *slog.Logger
}

func NewPropagationServer(service propagationController, sessionSecret string, logger *slog.Logger) *PropagationServer
```

`Connect`:
1. Read `authorization` from `metadata.FromIncomingContext`; accept a case-insensitive `Bearer ` prefix.
2. `auth.ParseSessionToken(token, server.sessionSecret, time.Now())`; any failure → `status.Error(codes.Unauthenticated, "invalid session token")`.
3. Create a cancellable child context.
4. `sender := newStreamSender(stream, cancel)`; `connID, err := service.Register(machineID, sender)`. `errors.Is(err, machineErrors.ErrMachineNotFound)` → Unauthenticated; any other error → Internal.
5. `defer service.Unregister(machineID, connID)`.
6. `service.DeliverSnapshot(ctx, machineID)`.
7. Loop `stream.Recv()`: `io.EOF` → nil; other errors → return the error; each ACK → `service.Acknowledge(ctx, machineID, fromProtoAck(ack))`.

In `sender.go`, `streamSender` holds the stream, a `sync.Mutex`, and `cancel`. `Send` locks, converts, sends, and on error calls `cancel()` before returning the error.

In `convert.go`, map `propagation.Message` to `pb.ServerMessage` with the `oneof` payload and map `pb.Ack` to `propagation.Ack`. Unknown kinds return an error; unknown ACK statuses map to the zero value.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/orchestrator/api/grpcserver/...`
Expected: PASS, including the 5 MiB round-trip.

- [ ] **Step 5: Stage the task for review (do not commit)**

```bash
git add internal/orchestrator/api/grpcserver
```

Suggested message: `feat(api): add authenticated gRPC propagation server`

---

### Task 11: Orchestrator Config and Wiring

**Files:**
- Modify: `internal/orchestrator/config/config.go`
- Modify: `internal/orchestrator/config/config_test.go`
- Modify: `internal/orchestrator/api/router.go`
- Modify: `cmd/orchestrator/main.go`
- Modify: `.env.example`

**Interfaces:**
- Consumes: `service.PropagationService` (Task 6), hooks (Task 7), machine service (Task 8), `grpcserver` (Task 10).
- Produces: `config.Config.GRPCAddr`; `api.Runtime{HTTP http.Handler; Propagation *service.PropagationService}`; `api.NewRuntime(db, cfg, logger) (*Runtime, error)`.

- [ ] **Step 1: Write the failing config tests**

Add to `internal/orchestrator/config/config_test.go`:

```go
func TestConfig_Load_WithoutGRPCAddr_DefaultsToPort9090(t *testing.T)
func TestConfig_Load_WithGRPCAddr_ReturnsConfiguredAddress(t *testing.T)
func TestConfig_Load_WithInvalidGRPCAddr_ReturnsError(t *testing.T)
```

Use `t.Setenv` for `AEGIS_GRPC_ADDR`. Invalid values: `"9090"` (no colon), `":abc"`, `":70000"`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/orchestrator/config/... -run GRPC`
Expected: FAIL with `Config.GRPCAddr undefined`.

- [ ] **Step 3: Add `GRPCAddr` to the config**

Add `envGRPCAddr = "AEGIS_GRPC_ADDR"` and `defaultGRPCAddr = ":9090"`. Add `GRPCAddr string` to `Config`. Implement `parseGRPCAddr` with `net.SplitHostPort`, a non-empty port, and an integer port in `[1, 65535]`. Empty input uses the default. Call it from `Load`.

- [ ] **Step 4: Add `Runtime` and wire the real notifier**

Refactor `api/router.go`:

```go
type Runtime struct {
    HTTP        http.Handler
    Propagation *service.PropagationService
}

// NewRuntime builds the HTTP router and the propagation service that shares
// the same stores and notifier.
func NewRuntime(db *sql.DB, cfg *config.Config, logger *slog.Logger) (*Runtime, error) {
    router := chi.NewRouter()
    router.Use(middleware.Logger)

    machineStore := store.NewMachineStore(db)
    policyStore := store.NewPolicyStore(db)
    groupStore := store.NewGroupStore(db)
    ruleStore := store.NewRuleStore(db)
    propagationStore := store.NewPropagationStore(db)

    propagationService := service.NewPropagationService(
        machineStore, groupStore, policyStore, ruleStore, propagationStore, logger, time.Now)

    machineService := service.NewMachineService(
        machineStore, propagationStore, cfg.LabKey, cfg.SessionSecret, cfg.SessionTTL, cfg.StalenessGrace, time.Now)

    if err := seedMachines(machineService, cfg); err != nil {
        return nil, err
    }

    registerHandlers(router,
        handler.NewGroupHandler(service.NewGroupService(
            groupStore, machineStore, policyStore, ruleStore, propagationService, cfg.StalenessGrace, time.Now)),
        handler.NewPolicyHandler(service.NewPolicyService(policyStore, propagationService)),
        handler.NewRuleHandler(service.NewRuleService(ruleStore, policyStore, groupStore, propagationService)),
        handler.NewMachineHandler(machineService),
    )

    return &Runtime{HTTP: router, Propagation: propagationService}, nil
}

func NewRouter(db *sql.DB, cfg *config.Config) (http.Handler, error) {
    runtime, err := NewRuntime(db, cfg, nil)
    if err != nil {
        return nil, err
    }
    return runtime.HTTP, nil
}
```

Keep `seedMachines` and `registerHandlers` unchanged. Add the `log/slog`, `chi`, and `middleware` imports; `chi` and `middleware` are already imported by the current file.

- [ ] **Step 5: Start the gRPC server in `cmd/orchestrator/main.go`**

- Keep the existing HTTP server setup; replace the `api.NewRouter` call with:
  `runtime, err := api.NewRuntime(db, cfg, logger)`.
- Create a `*slog.Logger` for the new components: `logger := slog.New(slog.NewTextHandler(os.Stderr, nil))`.
- `propagationCtx, cancelPropagation := context.WithCancel(context.Background())`; `defer cancelPropagation()`.
- `if err := runtime.Propagation.Start(propagationCtx); err != nil { log.Fatalf("failed to start propagation: %v", err) }`.
- `grpcServer := grpc.NewServer(grpc.MaxRecvMsgSize(16<<20), grpc.MaxSendMsgSize(16<<20))`.
- `pb.RegisterPropagationServer(grpcServer, grpcserver.NewPropagationServer(runtime.Propagation, cfg.SessionSecret, logger))`.
- `grpcListener, err := net.Listen("tcp", cfg.GRPCAddr)`; fail fast on error.
- Serve in a goroutine: `go func() { if err := grpcServer.Serve(grpcListener); err != nil { ... } }()`. Record the serve error in the same channel/select used for the HTTP server so either server failing triggers shutdown.
- On shutdown: cancel the propagation context, call `GracefulStop` in a goroutine, wait up to `shutdownTimeout`, then call `Stop`; then shut down HTTP as today.
- Use `runtime.HTTP` in place of the handler returned by `api.NewRouter`.

Add to `.env.example`:

```sh
# AEGIS_GRPC_ADDR is the address the gRPC propagation server binds to.
# Defaults to :9090.
AEGIS_GRPC_ADDR=:9090
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/orchestrator/config/... ./internal/orchestrator/api/... ./cmd/orchestrator/...`
Expected: PASS.

- [ ] **Step 7: Stage the task for review (do not commit)**

```bash
git add internal/orchestrator/config internal/orchestrator/api/router.go cmd/orchestrator/main.go .env.example
```

Suggested message: `feat(orchestrator): wire propagation service and gRPC server`

---

### Task 12: Sentinel Configuration

**Files:**
- Modify: `internal/sentinel/config/config.go`
- Modify: `internal/sentinel/config/config_test.go`
- Modify: `examples/sentinel.config.json`

**Interfaces:**
- Produces: `config.Config.PropagationAddress string`; `config.Config.RulesStatePath string`.

- [ ] **Step 1: Write the failing config tests**

Add to `internal/sentinel/config/config_test.go`:

```go
func TestConfig_Load_PropagationAddress_UnsetField_DefaultsToOrchestratorHostPort9090(t *testing.T)
func TestConfig_Load_PropagationAddress_ValidValue_ReturnsConfiguredAddress(t *testing.T)
func TestConfig_Load_PropagationAddress_InvalidValues_ReturnError(t *testing.T)
func TestConfig_Load_RulesStatePath_UnsetField_DefaultsToVarLibAegis(t *testing.T)
func TestConfig_Load_RulesStatePath_ValidValue_ReturnsConfiguredPath(t *testing.T)
```

Pin the default `http://10.0.0.5:8080` → `10.0.0.5:9090`. Invalid addresses: `"10.0.0.5"`, `"10.0.0.5:abc"`, `":9090"`. The default rules path is `/var/lib/aegis/rules.json`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/sentinel/config/... -run 'PropagationAddress|RulesStatePath'`
Expected: FAIL with undefined fields.

- [ ] **Step 3: Implement the fields and parsers**

Add to `document`:

```go
PropagationAddress string `json:"propagation_address"`
RulesStatePath     string `json:"rules_state_path"`
```

Add to `Config`:

```go
PropagationAddress string
RulesStatePath     string
```

Constants: `defaultPropagationPort = "9090"` and `defaultRulesStatePath = "/var/lib/aegis/rules.json"`.

`parsePropagationAddress(raw string, orchestratorURL *url.URL) (string, error)`:
- Empty → `net.JoinHostPort(orchestratorURL.Hostname(), defaultPropagationPort)`.
- Otherwise trim, `net.SplitHostPort`, require a non-empty host, and an integer port in `[1, 65535]`.

`parseRulesStatePath(raw string) (string, error)`:
- Empty → default; otherwise trimmed.

Call both from `Load`.

Update `examples/sentinel.config.json` with `"propagation_address": "10.0.0.5:9090"` and `"rules_state_path": "/var/lib/aegis/rules.json"`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/sentinel/config/...`
Expected: PASS.

- [ ] **Step 5: Stage the task for review (do not commit)**

```bash
git add internal/sentinel/config examples/sentinel.config.json
```

Suggested message: `feat(sentinel): configure propagation address and state path`

---

### Task 13: Sentinel Propagation Client

**Files:**
- Modify: `internal/sentinel/client/client.go`
- Create: `internal/sentinel/client/convert.go`
- Modify: `internal/sentinel/client/client_test.go`
- Modify: `internal/sentinel/daemon/daemon.go`
- Modify: `cmd/sentinel/main.go` (adapt to the new `client.New` signature)

**Interfaces:**
- Consumes: `propagation.Message`/`Ack`/`Stream` (Task 2), generated `pkg/pb` (Task 3), `config.PropagationAddress` (Task 12).
- Produces: `client.New(baseURL *url.URL, propagationAddress string, labKey []byte, timeout time.Duration, logger *slog.Logger) (*Client, error)`; `(*Client).ConnectPropagation(ctx, token) (propagation.Stream, error)`; `(*Client).Close() error`; `daemon.Session.MachineID`.

- [ ] **Step 1: Write the failing client tests**

Add to `internal/sentinel/client/client_test.go`:

```go
func TestClient_New_WithInvalidPropagationAddress_ReturnsError(t *testing.T)
func TestClient_Authenticate_WithValidChallenge_PopulatesMachineID(t *testing.T)
func TestClient_ConnectPropagation_WithToken_SendsAuthorizationMetadata(t *testing.T)
func TestClient_ConnectPropagation_WhenServerSendsPackage_ReturnsWireMessage(t *testing.T)
func TestClient_ConnectPropagation_WhenServerSendsRevoke_ReturnsRevokeMessage(t *testing.T)
func TestClient_ConnectPropagation_WhenAckSent_ConvertsStatus(t *testing.T)
func TestClient_Close_ClosesConnection(t *testing.T)
```

For the gRPC tests, start a real `grpc.NewServer` on `127.0.0.1:0` with a fake `pb.PropagationServer`, then build the client with that address. The fake records incoming `authorization` metadata and the received ACK.

Update `newTestClient` to the new signature and register `t.Cleanup(func() { _ = cl.Close() })`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/sentinel/client/...`
Expected: FAIL with the old `client.New` signature.

- [ ] **Step 3: Add the gRPC client and session machine ID**

In `client.go`:
- Add fields `connection *grpc.ClientConn` and `propagation pb.PropagationClient`.
- `New` validates `propagationAddress` with `net.SplitHostPort`, creates the connection with `grpc.NewClient(propagationAddress, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(16<<20), grpc.MaxCallSendMsgSize(16<<20)))`, and returns an error on failure.
- `Close` closes the connection.
- `ConnectPropagation` appends `authorization: Bearer <token>` to outgoing metadata, opens `pb.NewPropagationClient(...).Connect(ctx)`, and returns a wrapper implementing `propagation.Stream`.
- Put the wire conversions in `convert.go`: `toProtoAck(propagation.Ack) *pb.Ack`, `fromProtoServerMessage(*pb.ServerMessage) (*propagation.Message, error)`, and `fromProtoRules`. The wrapper's `Recv` uses `fromProtoServerMessage`; `Send` uses `toProtoAck`; `CloseSend` delegates.

In `daemon.go`, add `MachineID int64` to `Session` with a doc comment. In `client.Authenticate`, set `MachineID: authenticated.Machine.ID`.

In `cmd/sentinel/main.go`, adapt to the new signature:

```go
orchestrator, err := client.New(cfg.OrchestratorURL, cfg.PropagationAddress, cfg.LabKey, cfg.RequestTimeout, logger)
if err != nil {
    logger.Error("failed to initialize client", "error", err)
    return 1
}
defer orchestrator.Close()
```

Do not wire the propagation task yet; Task 16 does that.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/sentinel/client/... ./internal/sentinel/daemon/...`
Expected: PASS.

- [ ] **Step 5: Stage the task for review (do not commit)**

```bash
git add internal/sentinel/client internal/sentinel/daemon/daemon.go cmd/sentinel/main.go
```

Suggested message: `feat(sentinel): add authenticated propagation gRPC client`

---

### Task 14: Sentinel State File

**Files:**
- Create: `internal/sentinel/propagation/state.go`
- Create: `internal/sentinel/propagation/state_test.go`

**Interfaces:**
- Consumes: `propagation.Rule` (Task 2).
- Produces: `propagation.State`; `propagation.NewFileStore(path string) (*FileStore, error)`; `(*FileStore).Load() (State, error)`; `(*FileStore).Save(State) error`.

- [ ] **Step 1: Write the failing state tests**

Create `internal/sentinel/propagation/state_test.go` in `package propagation_test`:

```go
func TestFileStore_Load_WhenFileMissing_ReturnsZeroState(t *testing.T)
func TestFileStore_SaveAndLoad_WithPackage_RoundTrips(t *testing.T)
func TestFileStore_SaveAndLoad_WithRevoke_PreservesSequenceAndClearsRules(t *testing.T)
func TestFileStore_Load_WithCorruptFile_ReturnsError(t *testing.T)
func TestFileStore_Save_WhenParentMissing_CreatesDirectory(t *testing.T)
func TestFileStore_Save_WritesFileWithMode0600(t *testing.T)
func TestFileStore_Save_LeavesNoTemporaryFile(t *testing.T)
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/sentinel/propagation/... -run FileStore`
Expected: FAIL with undefined symbols.

- [ ] **Step 3: Implement the state and file store**

Create `state.go`:

```go
type State struct {
    Epoch         string             `json:"epoch"`
    LastSequence  int64              `json:"last_sequence"`
    PolicyID      *int64             `json:"policy_id"`
    PolicyName    string             `json:"policy_name"`
    PolicyVersion int                `json:"policy_version"`
    PolicyType    string             `json:"policy_type"`
    Rules         []propagation.Rule `json:"rules"`
}

type FileStore struct {
    path string
}

func NewFileStore(path string) (*FileStore, error)
func (store *FileStore) Load() (State, error)
func (store *FileStore) Save(state State) error
```

`NewFileStore` trims the path, rejects empty, and creates the parent with `os.MkdirAll(filepath.Dir(path), 0o755)`.

`Load` returns a zero state with `Rules: []propagation.Rule{}` when the file does not exist. A malformed JSON returns an error.

`Save` writes to `os.CreateTemp(dir, ".rules-*")`, `Chmod(0o600)`, encodes, `Sync`, `Close`, and `Rename` over the target. Remove the temporary file on any error.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/sentinel/propagation/... -run FileStore`
Expected: PASS.

- [ ] **Step 5: Stage the task for review (do not commit)**

```bash
git add internal/sentinel/propagation/state.go internal/sentinel/propagation/state_test.go
```

Suggested message: `feat(sentinel): persist applied rules atomically`

---

### Task 15: Sentinel Runner and Acceptance

**Files:**
- Create: `internal/sentinel/propagation/runner.go`
- Create: `internal/sentinel/propagation/apply.go`
- Create: `internal/sentinel/propagation/runner_test.go`
- Create: `internal/sentinel/propagation/apply_internal_test.go`

**Interfaces:**
- Consumes: `rules.Validate` (Task 1), `propagation.Message`/`Ack`/`Stream` (Task 2), `State`/`FileStore` (Task 14), `daemon.Session.MachineID` (Task 13).
- Produces: `propagation.StateStore`; `propagation.Connector`; `propagation.SessionRenewer`; `propagation.New(store StateStore, connector Connector, renewer SessionRenewer, logger *slog.Logger) *Runner`; `(*Runner).Run(ctx context.Context, session *daemon.Session) error`.

- [ ] **Step 1: Write the failing apply tests**

Create `internal/sentinel/propagation/apply_internal_test.go` in `package propagation` (internal, matching the existing `nonce_store_internal_test.go` precedent) with a `fakeStateStore` that can fail `Save`. Tests:

```go
func TestRunner_Apply_WithValidPackage_PersistsAndAcks(t *testing.T)
func TestRunner_Apply_WithStaleSequence_NacksAndLeavesStateUnchanged(t *testing.T)
func TestRunner_Apply_WithEqualSequence_Nacks(t *testing.T)
func TestRunner_Apply_WithEpochChange_AcceptsLowerSequence(t *testing.T)
func TestRunner_Apply_WithMachineMismatch_Nacks(t *testing.T)
func TestRunner_Apply_WithEmptyEpoch_Nacks(t *testing.T)
func TestRunner_Apply_WithEmptySequence_Nacks(t *testing.T)
func TestRunner_Apply_WithInvalidPolicyType_Nacks(t *testing.T)
func TestRunner_Apply_WithEmptyRules_Nacks(t *testing.T)
func TestRunner_Apply_WithInvalidRule_Nacks(t *testing.T)
func TestRunner_Apply_WithUnknownKind_Nacks(t *testing.T)
func TestRunner_Apply_WhenSaveFails_NacksAndKeepsOldState(t *testing.T)
func TestRunner_Apply_WithRevoke_PreservesSequenceAndClearsRules(t *testing.T)
```

Pin the NACK reasons exactly:

| Condition | Reason |
| --- | --- |
| empty epoch | `empty epoch` |
| zero sequence | `empty sequence` |
| machine mismatch | `machine mismatch` |
| sequence ≤ last | `stale sequence` |
| unknown kind | `unknown message kind` |
| save failure | `failed to persist state` |
| policy ID ≤ 0 | `invalid policy id` |
| empty policy name | `empty policy name` |
| version ≤ 0 | `invalid policy version` |
| type not WHITELIST/BLACKLIST | `invalid policy type` |
| zero rules | `empty policy` |
| invalid rule | the `rules.Validate` error text |

- [ ] **Step 2: Write the failing runner tests**

Create `runner_test.go` in `package propagation_test` with a `fakeConnector`, `fakeStream`, and `fakeRenewer`:

```go
func TestRunner_Run_WithPackageMessage_AppliesAndAcks(t *testing.T)
func TestRunner_Run_WhenStreamFails_Reconnects(t *testing.T)
func TestRunner_Run_WhenUnauthenticated_RenewsSessionAndRetries(t *testing.T)
func TestRunner_Run_WhenContextCancelled_ReturnsNil(t *testing.T)
func TestRunner_Run_WhenStateFileCorrupt_StartsFromZeroState(t *testing.T)
```

The fake stream returns queued messages and then blocks until the test closes it. Assert the ACK status and that the state file contains the applied sequence. For renewal, assert the connector receives the renewed token.

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/sentinel/propagation/... -run 'Runner|Apply'`
Expected: FAIL with undefined symbols.

- [ ] **Step 4: Implement `apply` and the runner**

In `apply.go`:

```go
func (runner *Runner) apply(session *daemon.Session, state State, message *propagation.Message) (State, propagation.Ack)
```

Follow design §11.5 in order. On any validation failure return the unchanged state and `Ack{Sequence: message.Sequence, Status: AckNacked, Reason: reason}`. On success build the next state, call `runner.store.Save(next)`, and return the next state with `AckAcked`. A revoke sets `PolicyID: nil`, empty metadata, `Rules: []propagation.Rule{}`, and keeps the new epoch/sequence.

In `runner.go`:

```go
type StateStore interface {
    Load() (State, error)
    Save(State) error
}

type Connector interface {
    ConnectPropagation(ctx context.Context, token string) (propagation.Stream, error)
}

type SessionRenewer interface {
    Authenticate(ctx context.Context, id identity.Identity) (*daemon.Session, error)
}

func New(store StateStore, connector Connector, renewer SessionRenewer, logger *slog.Logger) *Runner
```

`Run`:
1. Copy the session (`current := *session`) so the heartbeat task's session is not mutated.
2. `Load` the state; on error log `propagation state corrupt` and use a zero state.
3. Loop until `ctx.Done()`:
   - `ConnectPropagation(ctx, current.Token)`.
   - On `status.Code(err) == codes.Unauthenticated` (import `google.golang.org/grpc/codes` and `google.golang.org/grpc/status`): call `renewer.Authenticate(ctx, identity.Identity{IP: current.IP, MAC: current.MAC})`; on success replace `current`, log `propagation session renewed` with `logging.Fingerprint(current.Token)`, and retry immediately; on failure log and back off.
   - On any other error: log `propagation stream failed`, wait with exponential backoff (1s, 2s, 4s, capped at 30s), and retry.
   - On success: log `propagation connected` with the machine name, reset the backoff, and consume the stream.
   - `consume` loops `Recv`, calls `apply`, sends the ACK, and logs `package applied`/`revoke applied` or `propagation message rejected`. A send or receive error returns to the reconnect loop.
4. Return `nil` when the context is cancelled. `Run` never returns a non-nil error.

In `apply`, compare `message.PolicyType` against `propagation.PolicyTypeWhitelist` and `propagation.PolicyTypeBlacklist` (Task 2) instead of literal strings.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/sentinel/propagation/...`
Expected: PASS.

- [ ] **Step 6: Stage the task for review (do not commit)**

```bash
git add internal/sentinel/propagation/runner.go \
  internal/sentinel/propagation/apply.go \
  internal/sentinel/propagation/runner_test.go \
  internal/sentinel/propagation/apply_internal_test.go
```

Suggested message: `feat(sentinel): apply and acknowledge propagated rules`

---

### Task 16: Sentinel Main Wiring

**Files:**
- Modify: `cmd/sentinel/main.go`

**Interfaces:**
- Consumes: `config.PropagationAddress`/`RulesStatePath` (Task 12), `client.ConnectPropagation` (Task 13), `propagation.NewFileStore`/`New` (Tasks 14 and 15).
- Produces: a Sentinel that runs `heartbeat` and `propagation` tasks.

- [ ] **Step 1: Wire the runner**

In `cmd/sentinel/main.go`:
- Import the sentinel propagation package (alias it if needed to avoid confusion with the common package).
- After creating the client, create the state store:

```go
stateStore, err := propagation.NewFileStore(cfg.RulesStatePath)
if err != nil {
    logger.Error("failed to initialize propagation state store", "error", err)
    return 1
}

propagationRunner := propagation.New(stateStore, orchestrator, orchestrator, logger)
```

- Add a second daemon task:

```go
daemon.New(logger, detector, orchestrator,
    daemon.Task{Name: "heartbeat", Run: heartbeats.Run},
    daemon.Task{Name: "propagation", Run: propagationRunner.Run},
)
```

- Add `propagation_address` and `rules_state_path` to the `configuration loaded` log record.
- Keep `defer orchestrator.Close()`.

- [ ] **Step 2: Verify the Sentinel builds and the existing tests pass**

Run: `go build ./cmd/sentinel`
Run: `go test ./internal/sentinel/...`
Expected: PASS.

- [ ] **Step 3: Stage the task for review (do not commit)**

```bash
git add cmd/sentinel/main.go
```

Suggested message: `feat(sentinel): run propagation alongside heartbeat`

---

### Task 17: End-to-End Propagation Suite

**Files:**
- Modify: `e2e/harness_test.go`
- Create: `e2e/propagation_e2e_test.go`

**Interfaces:**
- Consumes: all previous tasks.
- Produces: e2e coverage of package delivery, ACK, rename, revoke, PENDING, and restart.

- [ ] **Step 1: Extend the harness**

In `e2e/harness_test.go`:
- Add `orchestratorGRPCPort = "9090/tcp"` and pass `AEGIS_GRPC_ADDR=:9090` in `startOrchestrator`'s default env.
- In `startSentinel`, add `"propagation_address": "172.30.0.10:9090"` and `"rules_state_path": "/tmp/aegis-rules.json"` to the config JSON.
- Add a `sentinelHandle.restart(t)` that calls `h.container.Start(context.Background())` after a stop and re-registers cleanup.
- Add REST helpers: `orchestratorEndpoint`, `doJSON`, `createPolicy`, `createRule`, `createGroup`, `linkMachine`, `linkPolicy`, `unlinkPolicy`, `renamePolicy`, `deletePolicy`, `deleteGroup`, `machineByIP`, `propagationOf`, and `waitForPropagation`.
- Add `readContainerFile(t, path)` using `container.CopyFileFromContainer` for the state-file assertion.

Use `schemas.PolicyOutputSchema`, `schemas.RuleOutputSchema`, `schemas.GroupOutputSchema`, and `schemas.PropagationOutputSchema` for decoding. Poll with a 60-second timeout and a 250 ms interval.

- [ ] **Step 2: Write the e2e scenarios**

Create `e2e/propagation_e2e_test.go` with:

```go
func TestPropagationE2E_LinkedPolicy_AppliesPackageAndReportsAcked(t *testing.T)
func TestPropagationE2E_RuleChange_PushesNewSequence(t *testing.T)
func TestPropagationE2E_PolicyRename_PushesNewNameAndVersion(t *testing.T)
func TestPropagationE2E_UnlinkPolicy_PushesRevoke(t *testing.T)
func TestPropagationE2E_DeleteLinkedPolicy_PushesRevoke(t *testing.T)
func TestPropagationE2E_DeleteGroupWithMachines_PushesRevoke(t *testing.T)
func TestPropagationE2E_MachineWithoutPolicy_ReportsPending(t *testing.T)
func TestPropagationE2E_SentinelRestart_PreservesSequence(t *testing.T)
```

Scenario details:
- **Package/ACK:** one Sentinel, one machine; create policy with two rules, group, link machine, link policy. Wait for the Sentinel log `package applied`; poll `GET /machines` until `propagation.status == "ACKED"`; assert `kind == "PACKAGE"`, `sequence > 0`, `duration_ms != nil`, `policy_name`, and `policy_version`.
- **Rule change:** after the first ACK, update one rule; poll until the sequence increases and status is `ACKED`.
- **Rename:** rename the policy; poll until `policy_name` is the new name and `policy_version` increased.
- **Unlink policy:** unlink; wait for `revoke applied`; poll until `kind == "REVOKE"` and status `ACKED`.
- **Delete linked policy:** create and link a second policy; delete it; assert a `REVOKE` row is ACKED and the machine's stored rules are cleared.
- **Delete group with machines:** delete the group; assert a `REVOKE` row is ACKED.
- **PENDING:** start the Orchestrator with two registered machines and start a Sentinel only for the first one. The second machine never connects, so no propagation row exists; `GET /machines` reports `propagation.status == "PENDING"`, `sequence == 0`, and `policy_id == null` for it. Do not use a connected machine with no group: on connect it receives a `REVOKE`, which is a real row.
- **Restart:** apply a package, read `/tmp/aegis-rules.json` and assert `last_sequence > 0`; stop and restart the Sentinel; update a rule; poll for a higher sequence with `ACKED`; read the state file again and assert `last_sequence` increased.

- [ ] **Step 3: Run the e2e suite**

Run: `make e2e`
Expected: PASS. This requires Docker and can take several minutes.

- [ ] **Step 4: Stage the task for review (do not commit)**

```bash
git add e2e/harness_test.go e2e/propagation_e2e_test.go
```

Suggested message: `test(e2e): cover rule propagation and acknowledgement`

---

### Task 18: Documentation, Coverage, and Final Verification

**Files:**
- Modify: `Makefile` (`TEST_PACKAGES`)
- Modify: `README.md`
- Modify: `examples/sentinel.config.json` (if Task 12 left anything undocumented)
- Modify: `docs/superpowers/specs/2026-11-03-us09-rule-propagation-design.md`
- Modify: `docs/superpowers/specs/2026-10-28-us06-sentinel-heartbeat-design.md`

**Interfaces:**
- Consumes: every task.
- Produces: updated docs, coverage inclusion, and a verified branch.

- [ ] **Step 1: Add the new packages to coverage**

In `Makefile`, append to `TEST_PACKAGES`:

```make
./internal/common/rules \
./internal/common/propagation \
./internal/orchestrator/api/grpcserver \
./internal/sentinel/propagation
```

- [ ] **Step 2: Update the documentation**

- `README.md`: document `protoc` as a system prerequisite, `make proto`, `AEGIS_GRPC_ADDR`, and the two new Sentinel configuration fields.
- `.env.example`: confirm `AEGIS_GRPC_ADDR` is present (Task 11).
- `examples/sentinel.config.json`: confirm both new fields are present (Task 12).
- `docs/superpowers/specs/2026-10-28-us06-sentinel-heartbeat-design.md`: add a short note that the propagation channel is now an authenticated gRPC stream, superseding the earlier HTTPS-push parenthetical (the line that says propagation is push-over-HTTPS with the Sentinel serving the inbound endpoint).
- US09 design: update §6.4, §7.4, §11.2, and §11.4 to match the callback `Create`, `propagation.Stream`, and `StateStore` signatures resolved during planning.

- [ ] **Step 3: Run the full verification**

Run:

```sh
make proto
make build
make tests
make coverage
```

Expected: all commands succeed; `go tool cover -func=coverage.out` reports a total at or above `75.0%`.

Run `make e2e` if Docker is available.
Expected: PASS.

- [ ] **Step 4: Stage the task for review (do not commit)**

```bash
git add Makefile README.md examples/sentinel.config.json docs/superpowers/specs
```

Suggested message: `docs: document propagation setup and coverage`

---

## Final Review Checklist

- [ ] Every task leaves `go build ./...` and `go test ./...` passing (except `e2e`, which needs the `e2e` tag and Docker).
- [ ] No generated `pkg/pb` files are staged.
- [ ] No task commits automatically; all changes are staged for the developer.
- [ ] The 75% coverage gate is met with the new packages included.
- [ ] The design document matches the callback `Create`, `propagation.Stream`, `StateStore`, and gRPC controller interface.
- [ ] `TLS-FOLLOWUP.md` remains the single source for the deferred TLS work.
