# US09 — Propagação de Regras para Grupos — Design

- **Issue:** [#19](https://github.com/dougdomingos/aegis/issues/19)
- **Status:** Draft, pending review
- **Date:** 2026-11-03 (revised 2026-11-04)
- **Plan:** [`docs/superpowers/plans/2026-11-03-us09-rule-propagation.md`](../plans/2026-11-03-us09-rule-propagation.md) (to be written after approval)

## 1. Context & Goal

Groups can be linked to a policy (US08) and machines can be linked to a group
(US07), but nothing reaches the machines yet. US09 makes the Orchestrator
distribute the rules of each group's policy to every machine in that group,
and makes the Orchestrator record which machines confirmed receipt and how long
each one took.

Acceptance criteria (from the issue):

1. Every rule of the policies linked to a group is distributed to each machine
   linked to that group.
2. It must be possible to see which machines confirmed receipt of the
   propagated policies, and the propagation time for each machine.

Enforcement of the received rules on the local network is US10 (#20). US09 ends
when the Sentinel has validated, durably stored, and acknowledged the package.

### 1.1 Terminology

| Term | Meaning |
| --- | --- |
| **Package** | A full snapshot of one machine's desired rules: the policy identity and every rule of that policy. |
| **Revoke** | A message telling one machine to clear its applied rules. |
| **Propagation** | One sent message. It is the unit stored in the audit table. |
| **Sequence** | A per-machine, monotonically increasing number assigned by the Orchestrator. It is the only ordering authority. |
| **Epoch** | A random identifier generated once per Orchestrator database. It lets a Sentinel detect that the Orchestrator's sequence space was reset. |
| **Policy version** | The existing per-policy counter. It is metadata carried in the package and is never used to accept or reject a message. |

## 2. Scope

**In scope:**

- A long-lived, authenticated gRPC stream from each Sentinel to the
  Orchestrator.
- A full package push on every (re)connect and after every relevant change:
  rule create/update/remove, group policy link/unlink, machine group
  link/unlink, and policy rename.
- A revoke push when a linked policy is removed, when a group with machines is
  removed, and when a group or machine loses its policy/group association.
- A per-machine sequence assigned by the Orchestrator.
- A `propagations` audit table that retains every sent message, including its
  serialized payload.
- ACK/NACK from the Sentinel, with status and timing stored by the
  Orchestrator.
- Latest propagation state exposed on the machine resource:
  `GET /machines` and `GET /machines/{id}/propagation`.
- Sentinel-side durable state at `rules_state_path`, written atomically.
- A proto generation target and the CI/e2e changes needed to make it
  executable.

**Out of scope:**

- Local enforcement of the received rules (US10, #20).
- A propagation history endpoint. History is retained for audit but the API
  exposes only the latest row per machine.
- A group-scoped propagation endpoint. Admins derive it by filtering machines
  by `GroupID`/`GroupName`.
- TLS for the gRPC stream. The existing REST channel is also plaintext; TLS is
  a separate concern (see #14).
- Automatic retry loops for failed deliveries. Reconnect and the next change
  reconcile state (see §12).
- Retention/pruning of old `propagations` rows.

### 2.1 Approaches Considered

#### 2.1.1 Delivery transport

1. **gRPC bidirectional stream (chosen).** The Sentinel dials the Orchestrator
   and keeps one ordered connection open. Packages and ACKs share it, reconnect
   is a new stream, and the Orchestrator never dials into a lab network. Cost:
   a proto toolchain, generated code, and a new server/client path.
2. **HTTP long-poll.** The Sentinel polls an endpoint that holds the request
   open until a message or a timeout. No proto tooling, but the Orchestrator
   must correlate ACKs, and a poll cycle adds latency and request churn.
3. **HTTPS push to a Sentinel endpoint.** The Orchestrator dials each machine.
   This matches the parenthetical in the US06 design, but lab machines are
   behind NAT/firewalls and the Orchestrator would need per-machine
   reachability, retries, and a listener in the Sentinel.

The design proceeds with gRPC, because the old US09 plan assumed it and because
push plus ACK fit one ordered stream. This choice is the largest fork in the
plan; it is repeated in §18 for confirmation.

#### 2.1.2 Status storage

1. **Latest row plus full history (chosen).** Every sent message is a row, so
   the audit requirement is met; reads take the highest sequence per machine,
   so the admin API stays small.
2. **Current-state table only.** Simpler reads, but it destroys the audit trail
   the issue asks for.
3. **Expose the full history through the API.** Satisfies audit visibility but
   makes the machine resource paginate an unbounded log; the issue only asks
   for the latest state and the time each machine took.

The chosen option keeps history in SQLite and exposes only the latest row.

## 3. Decisions

| Decision | Choice | Rationale |
| --- | --- | --- |
| Transport | **gRPC bidirectional stream**, Sentinel dials Orchestrator | Push and ACK share one ordered connection; reconnects are natural; the Orchestrator never needs to dial a lab machine. This supersedes the parenthetical in the US06 design that assumed a Sentinel-hosted HTTPS endpoint. |
| Authentication | Session token in gRPC `authorization` metadata, verified with `auth.ParseSessionToken` | Reuses the token already issued by US05 and the verification primitive from US06. |
| Ordering | Per-machine `sequence`, strictly increasing | Policy version is not comparable across policies; sequence is. |
| Version | Informational only | A policy switch can make the version go backwards while the new package is still the newest delivery. |
| Status reads | Latest row only | The administrator needs the current state; history stays in the audit table. |
| No row | `PENDING` | A machine that has never received a message is visibly pending, not absent. |
| Audit | Every sent message is a row, including its serialized payload | Satisfies the audit requirement; the API never exposes history or payloads. |
| Policy removed | `REVOKE` to machines in groups linked to it, resolved before deletion | `ON DELETE SET NULL` would otherwise silently leave stale rules on the machines. |
| Group removed | `REVOKE` to machines in the group, resolved before deletion | The machines are unlinked by `ON DELETE SET NULL`; the rules must still be cleared. |
| Policy renamed | New package to every affected machine | The package carries the policy name and version; otherwise the Sentinel's stored metadata goes stale. |
| Sequence reset | **Epoch** carried in every message | A wiped/recreated Orchestrator database restarts sequences; without an epoch, Sentinels would reject every new package forever. |
| Message size | 16 MiB send/receive limit on both sides | A policy with thousands of rules can exceed gRPC's 4 MiB default. |
| Delivery failure | Record `FAILED`, close the stream, let reconnect resend | No retry loop that could hammer a broken Sentinel. |
| NACK | Record `NACKED` with the reason; no automatic retry | The admin can see the reason; the next change or reconnect retries naturally. |
| Sentinel state write | Atomic temp-file + rename | A crash must never leave a half-written state file. |
| Revoke state | Persist the new sequence and clear the rules | The sequence must survive a revoke; otherwise older packages would be accepted after a restart. |
| Policy type | Carried in the package and the Sentinel state | US10 needs `WHITELIST`/`BLACKLIST` to enforce; adding it later would change the wire format and the on-disk state. |
| Reconnect safety | Registration returns a connection ID; unregister is scoped to it | A new stream can register before the old handler's deferred unregister runs; an unscoped unregister would remove the live sender. |
| Pending intent | Latest notification wins | An unlink followed by a relink must end in a package, not a stale revoke. The worker rebuilds the desired state for package intents anyway. |
| Revoke timing | Resolve machine IDs before the delete, enqueue only after the delete succeeds | A failed delete must not leave a queued revoke that clears rules that still exist. |
| Shared validator | `rules.Validate(rules.Rule) error`; `domain.Rule.Validate` keeps its `PolicyID` check and delegates the rest | The wire rule has no policy ID, so the policy check cannot move into the shared package without changing behavior. |
| Epoch acceptance | Accept any non-empty epoch different from the stored one | A stream is ordered and only one stream per machine is live, so an older-epoch message cannot follow a newer-epoch one. |
| Revoke audit metadata | Record the policy identity when the revoke comes from a policy; leave it empty for group/machine unlinks | The policy is resolved before deletion, so its metadata is still available; the group/machine cases have no single policy. |

### 3.1 Sequence vs. version, by example

The Sentinel accepts a message when the epoch changed, or when the sequence is
strictly greater than the last applied sequence. The policy version is ignored
for acceptance.

| Case | Last applied | New package | Sequence | Result |
| --- | --- | --- | --- | --- |
| Same version | P1 v3, seq 10 | P2 v3, seq 11 | 11 > 10 | accept |
| P1 has a higher version | P1 v5, seq 10 | P2 v2, seq 11 | 11 > 10 | accept |
| P1 has a lower version | P1 v2, seq 10 | P2 v5, seq 11 | 11 > 10 | accept |
| Reconnect, unchanged policy | P1 v3, seq 10 | P1 v3, seq 11 | 11 > 10 | accept |
| Stale delivery | P1 v3, seq 10 | P1 v3, seq 9 | 9 ≤ 10 | NACK `stale sequence` |

## 4. Architecture

```
Admin REST ──> Orchestrator HTTP API ──> Services ──> SQLite
                                             │
                                             │ notifier (best effort)
                                             ▼
                                    PropagationService
                                    (pending set + worker)
                                             │ gRPC stream
                                             ▼
                                        Sentinel
                                  (validate + persist + ACK/NACK)
```

The Orchestrator owns the desired state and the per-machine sequence. The
Sentinel owns the applied state and the last applied sequence. There is no
shared mutable state beyond the message itself.

### 4.1 Data flow

1. A service mutation commits to SQLite.
2. The service calls the propagation notifier with the policy, group, or
   machine ID it already has.
3. The notifier resolves the affected machine IDs and marks them pending. The
   latest intent replaces any previous intent for the same machine.
4. The worker builds the current desired snapshot, assigns the next sequence,
   inserts a `propagations` row, and sends the message over the machine's
   stream.
5. The Sentinel validates the message, persists the new state atomically, and
   sends an ACK or NACK.
6. The Orchestrator updates the row. The status API reads the latest row.

Notification is best effort. A notification that is lost is recovered by the
next change or by the snapshot sent on the next connect.

## 5. Wire Protocol

New file `proto/propagation.proto`:

```proto
syntax = "proto3";

package aegis.propagation.v1;

option go_package = "dougdomingos.com/aegis/pkg/pb;pb";

service Propagation {
  rpc Connect(stream Ack) returns (stream ServerMessage);
}

message Ack {
  int64 sequence = 1;
  AckStatus status = 2;
  string reason = 3;
}

enum AckStatus {
  ACK_STATUS_UNSPECIFIED = 0;
  ACK_STATUS_ACKED = 1;
  ACK_STATUS_NACKED = 2;
}

message ServerMessage {
  string epoch = 1;
  int64 sequence = 2;
  int64 machine_id = 3;
  oneof payload {
    PolicyPackage policy = 4;
    Revoke revoke = 5;
  }
}

message PolicyPackage {
  int64 policy_id = 1;
  string policy_name = 2;
  int32 policy_version = 3;
  repeated Rule rules = 4;
  string policy_type = 5;
}

message Revoke {}

message Rule {
  string type = 1;
  string value = 2;
  optional string protocol = 3;
  optional int32 port = 4;
}
```

- The stream is authenticated once, at connect. The server reads
  `authorization: Bearer <session-token>` from incoming metadata and derives
  the machine ID with `auth.ParseSessionToken`.
- A token for a machine that no longer exists is rejected as
  `codes.Unauthenticated`.
- The server sets `grpc.MaxRecvMsgSize(16 << 20)` and
  `grpc.MaxSendMsgSize(16 << 20)`; the client sets the equivalent
  `MaxCallRecvMsgSize`/`MaxCallSendMsgSize`.
- The server sends the initial snapshot immediately after registering the
  stream. The client never has to ask for it.
- ACKs travel on the same stream. The server ignores an ACK whose sequence does
  not belong to that machine.

### 5.1 Shared wire types

The business logic must not depend on generated proto types. A new package
`internal/common/propagation` defines the transport-neutral message:

```go
type Kind string

const (
    KindPackage Kind = "PACKAGE"
    KindRevoke  Kind = "REVOKE"
)

type Rule struct {
    Type     string
    Value    string
    Protocol *string
    Port     *int
}

type Message struct {
    Epoch         string
    Sequence      int64
    MachineID     int64
    Kind          Kind
    PolicyID      int64
    PolicyName    string
    PolicyVersion int
    PolicyType    string
    Rules         []Rule
}

type AckStatus string

const (
    AckAcked  AckStatus = "ACKED"
    AckNacked AckStatus = "NACKED"
)

type Ack struct {
    Sequence int64
    Status   AckStatus
    Reason   string
}

// Stream is the ordered, bidirectional channel a Sentinel uses to receive
// messages and send acknowledgements. It lives in internal/common/propagation
// so the runner's Connector and the client share one return type.
type Stream interface {
    Recv() (*Message, error)
    Send(ack Ack) error
    CloseSend() error
}
```

The gRPC adapter converts between these types and `pkg/pb`. The Orchestrator
service and the Sentinel runner both use this package, so neither imports
generated code directly.

## 6. Data Model

### 6.1 Migration `007_create_propagation_table.sql`

```sql
CREATE TABLE IF NOT EXISTS propagations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    machine_id INTEGER NOT NULL,
    sequence INTEGER NOT NULL,
    kind TEXT NOT NULL,
    policy_id INTEGER,
    policy_name TEXT NOT NULL DEFAULT '',
    policy_version INTEGER NOT NULL DEFAULT 0,
    policy_type TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    sent_at DATETIME,
    acked_at DATETIME,
    nack_reason TEXT NOT NULL DEFAULT '',
    payload TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(machine_id, sequence)
);
CREATE INDEX IF NOT EXISTS idx_propagations_machine_sequence
    ON propagations(machine_id, sequence DESC);
```

`machine_id` and `policy_id` deliberately have no foreign keys. The table is an
audit log: deleting a machine or policy must not erase the record of what was
sent. The status API only reads rows for machines that still exist.

`payload` holds the JSON serialization of the exact `propagation.Message` that
was sent, so the audit can reconstruct the rule set even after the policy
changes. The status API never returns it.

### 6.2 Migration `008_create_system_metadata_table.sql`

```sql
CREATE TABLE IF NOT EXISTS system_metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
```

US09 stores one key: `propagation_epoch`. The value is 32 lowercase hex
characters generated from 16 random bytes. `PropagationStore.Epoch` reads it,
inserts it with `INSERT OR IGNORE` when absent, and returns it. The epoch
changes only when the database is recreated.

### 6.3 Domain types

New file `internal/orchestrator/domain/propagation.go`:

```go
type PropagationKind string

const (
    PropagationPackage PropagationKind = "PACKAGE"
    PropagationRevoke  PropagationKind = "REVOKE"
)

type PropagationStatus string

const (
    PropagationPending PropagationStatus = "PENDING"
    PropagationSent    PropagationStatus = "SENT"
    PropagationAcked   PropagationStatus = "ACKED"
    PropagationNacked  PropagationStatus = "NACKED"
    PropagationFailed  PropagationStatus = "FAILED"
)

type Propagation struct {
    ID            int64
    MachineID     int64
    Sequence      int64
    Kind          PropagationKind
    PolicyID      *int64
    PolicyName    string
    PolicyVersion int
    PolicyType    string
    Status        PropagationStatus
    SentAt        *time.Time
    AckedAt       *time.Time
    NackReason    string
    Payload       string
    CreatedAt     time.Time
}
```

`PENDING` is the only status that can be synthesized by the API. The store
also writes it between `Create` and `MarkSent`; a crash in that window leaves
an honest "queued but not sent" row.

### 6.4 `PropagationStore`

```go
type PropagationStore interface {
    Create(ctx context.Context, propagation Propagation, encode func(sequence int64) (string, error)) (*Propagation, error)
    MarkSent(ctx context.Context, machineID, sequence int64, at time.Time) error
    MarkFailed(ctx context.Context, machineID, sequence int64, reason string) error
    Acknowledge(ctx context.Context, machineID, sequence int64, status PropagationStatus, reason string, at time.Time) error
    LatestByMachine(ctx context.Context, machineID int64) (*Propagation, error)
    ListLatest(ctx context.Context) ([]Propagation, error)
    Epoch(ctx context.Context) (string, error)
}
```

- `Create` runs in a transaction: read
  `COALESCE(MAX(sequence), 0) + 1` for the machine, call `encode(sequence)`
  to produce the exact serialized `payload`, insert the row with
  `status = PENDING`, the `payload`, and the metadata, and return the row with
  its assigned sequence and `created_at`. The encoder is a callback because the
  sequence is assigned inside the insert transaction and the wire payload must
  carry it verbatim; an encoder failure rolls the transaction back.
- `MarkSent` sets `status = SENT` and `sent_at = at`.
- `MarkFailed` sets `status = FAILED` and `nack_reason = reason`; `acked_at`
  stays `NULL`.
- `Acknowledge` sets `status`, `acked_at`, and `nack_reason` for the matching
  `(machine_id, sequence)`. A missing row is not an error: a late ACK for a
  superseded message is simply ignored.
- `LatestByMachine` orders by `sequence DESC LIMIT 1`.
- `ListLatest` joins each machine to its maximum sequence.
- Status reads select the metadata columns only; `payload` is not loaded.
- `Epoch` is idempotent and safe to call concurrently.

## 7. Orchestrator Service

### 7.1 Notifier contract

New file `internal/orchestrator/service/propagation_notifier.go`:

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

`ResolvePolicyMachines` and `ResolveGroupMachines` are read-only and run before
the delete, while the foreign keys still point at the group or policy. They
return an error, and the service aborts the delete if resolution fails: deleting
without a revoke would leave stale rules on the machines. `RevokeMachines`
enqueues the revoke after the delete commits and returns no error, because a
failed enqueue must not turn a committed delete into an HTTP failure. `policy`
carries the identity of a policy being removed, for the audit row; it is `nil`
for a group or machine unlink.

The interface is owned by the services that consume it, per AGENTS.md. The
mutation methods return no error: the mutation is already committed, and a
notification failure must never turn a successful write into an HTTP failure.
The implementation logs failures. A nil notifier is allowed in tests and
behaves as a no-op.

### 7.2 Hook placement

| Mutation | Call | When |
| --- | --- | --- |
| `RuleService.CreateRule` | `NotifyPolicy(policyID)` | after the store create |
| `RuleService.UpdateRule` | `NotifyPolicy(policyID)` | only after a real update; the existing no-op path returns before this |
| `RuleService.RemoveRule` | `NotifyPolicy(policyID)` | after the store remove |
| `GroupService.LinkMachine` | `NotifyMachine(machineID)` | after the link |
| `GroupService.UnlinkMachine` | `NotifyMachine(machineID)` | after the unlink |
| `GroupService.LinkPolicy` | `NotifyGroup(groupID)` | after the link |
| `GroupService.UnlinkPolicy` | `NotifyGroup(groupID)` | after the unlink |
| `GroupService.RemoveGroup` | `ids, err := ResolveGroupMachines(groupID)`; return on error; `RevokeMachines(ids, nil)` | resolve **before** `store.Remove`; enqueue **after** it succeeds |
| `PolicyService.UpdatePolicy` | `NotifyPolicy(policyID)` | after a real rename; the existing no-op path returns before this |
| `PolicyService.RemovePolicy` | `ids, err := ResolvePolicyMachines(policyID)`; return on error; `RevokeMachines(ids, policy)` | resolve **before** `store.Remove`; enqueue **after** it succeeds |

The two revoke paths must resolve their machine IDs before the delete, because
the foreign keys are `ON DELETE SET NULL`, but they must not enqueue until the
delete has committed. A resolution error aborts the delete, so the admin can
retry instead of silently leaving stale rules. If the store delete returns an
error, the service returns before `RevokeMachines` and nothing is queued.

### 7.3 Fan-out resolution

`PropagationService` owns the resolution so the domain services only pass the
IDs they already have:

- `NotifyPolicy(policyID)`: `GroupStore.ListByPolicy(policyID)` → for each
  group, `MachineStore.ListByGroup(group.ID)` → mark every machine pending with
  a package intent.
- `NotifyGroup(groupID)`: `MachineStore.ListByGroup(groupID)` → mark every
  machine pending with a package intent. The worker builds the current desired
  state, so a group without a policy naturally becomes a revoke.
- `NotifyMachine(machineID)`: mark that machine pending with a package intent.
  The worker builds the current desired state.
- `ResolvePolicyMachines(policyID)` / `ResolveGroupMachines(groupID)`: run the
  same resolution but only return the IDs; they do not enqueue and surface any
  store error to the caller.
- `RevokeMachines(machineIDs, policy)`: mark every listed machine with a
  revoke intent carrying the optional policy metadata.

The pending set holds one intent per machine. A notification replaces the
previous intent, so the latest one wins: unlink followed by relink ends in a
package intent, and relink followed by unlink ends in a revoke intent. A
package intent is resolved from the current desired state at send time, so it
still becomes a revoke if the machine has lost its group, policy, or rules. A
revoke intent sends a revoke directly and needs no links.

New store method:

```go
// GroupStore
ListByPolicy(ctx context.Context, policyID int64) ([]Group, error)
```

`MachineStore.ListByGroup` already exists.

### 7.4 `PropagationService`

New file `internal/orchestrator/service/propagation_service.go`:

```go
type PropagationService struct {
    // stores: machine, group, policy, rule, propagation
    // logger, now, epoch
    // pending set, wake channel, senders registry
}

func NewPropagationService(
    machines domain.MachineStore,
    groups domain.GroupStore,
    policies domain.PolicyStore,
    rules domain.RuleStore,
    store domain.PropagationStore,
    logger *slog.Logger,
    now func() time.Time,
) *PropagationService

func (s *PropagationService) Start(ctx context.Context) error
func (s *PropagationService) Register(ctx context.Context, machineID int64, sender propagation.MessageSender) (uint64, error)
func (s *PropagationService) Unregister(machineID int64, connID uint64)
func (s *PropagationService) DeliverSnapshot(ctx context.Context, machineID int64)
func (s *PropagationService) Acknowledge(ctx context.Context, machineID int64, ack propagation.Ack)
```

`MessageSender` (`Send(ctx, message) error`) is declared in
`internal/common/propagation` alongside the other transport-neutral types: the
service consumes it to deliver messages and the gRPC adapter implements it, so
the API layer never depends on the service for the contract.

`Start` loads and caches the epoch, then launches the worker. It returns an
error if the epoch cannot be loaded. It is called once, before the gRPC server
starts accepting connections.

`Register` verifies that the machine exists under the provided context, stores
the sender, and returns a fresh connection ID. `Unregister` removes the sender
only when the connection ID matches the registered one, so a reconnecting
stream cannot be unregistered by
the previous handler's `defer`. `DeliverSnapshot` marks the machine pending with
a package intent; it does not send synchronously, so all sends are serialized by
the worker. The service satisfies the `PropagationNotifier` contract of §7.1
(`var _ PropagationNotifier = (*PropagationService)(nil)`), and its dispatch
creates the audit row through `PropagationStore.Create`, passing the JSON encoder
as a callback so the stored payload carries the sequence the store assigned.

### 7.5 Snapshot building

The worker builds the message at send time, not at notification time:

- **Revoke intent:** build a `REVOKE` message. It does not need the policy or
  group, so it is safe even after the row was deleted. If the intent carries a
  policy, its ID, name, version, and type are copied into the audit row; the
  wire message stays a bare revoke.
- **Package intent:** load the machine. If it is gone, skip. Load its group; if
  there is no group, build a `REVOKE`. Load the group's policy; if there is
  none, build a `REVOKE`. Load the policy's rules; if there are none, build a
  `REVOKE` (defensive; the P0 guards prevent this). Otherwise build a package
  with the policy ID, name, version, type, and every rule.

A package carries the full rule set every time. There are no deltas.

### 7.6 Dispatch and acknowledgement

For each machine the worker processes:

1. Look up the registered sender. If there is none, skip: the machine is
   offline and no message is sent, so no audit row is created.
2. Build the message, serialize it to JSON, and insert a `PENDING` row with the
   next sequence and the serialized payload.
3. `MarkSent` with the Orchestrator's clock. If this fails, log and skip the
   send; the next change or reconnect retries.
4. Send over the stream. On error, `MarkFailed` and cancel the stream context
   so the client reconnects.
5. On ACK/NACK, `Acknowledge` updates the row with the Orchestrator's clock.

The worker is single-threaded. This serializes sequence assignment and sends
per machine, so a newer sequence can never overtake an older one on the wire.
A per-send timeout (10s) prevents one unresponsive machine from blocking the
whole fleet.

### 7.7 Status mapping

New schema `schemas.PropagationOutputSchema`:

```go
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

`duration_ms` is `acked_at - sent_at` in milliseconds when `acked_at` is set;
otherwise it is `null`. A NACK has a duration (time until the rejection); a
transport failure has no `acked_at` and therefore no duration.

No row maps to:

```json
{
  "sequence": 0,
  "kind": "",
  "status": "PENDING",
  "policy_id": null,
  "policy_name": "",
  "policy_version": 0,
  "policy_type": "",
  "sent_at": null,
  "acked_at": null,
  "duration_ms": null,
  "nack_reason": ""
}
```

### 7.8 `MachineService` changes

`MachineService` gains a `domain.PropagationStore` dependency.

- `ListMachines` lists the machines, calls `PropagationStore.ListLatest`, and
  merges the latest row into each machine's output. Machines without a row get
  the `PENDING` representation.
- New method:

```go
GetPropagation(ctx context.Context, machineID int64) (*schemas.PropagationOutputSchema, error)
```

It loads the machine first; a missing machine returns `errors.ErrMachineNotFound`.
A machine with no row returns the `PENDING` representation, not an error.

`MachineOutputSchema` gains:

```go
Propagation PropagationOutputSchema `json:"propagation"`
```

The existing machine fields keep their current JSON names (they have no tags).
The new field is explicitly tagged `propagation`; its nested fields use the
snake_case tags above.

## 8. Orchestrator HTTP API

| Method | Path | Purpose | Success | Failure |
| --- | --- | --- | --- | --- |
| GET | `/machines` | List machines with their latest propagation | `200` | — |
| GET | `/machines/{id}/propagation` | Latest propagation of one machine | `200` (including `PENDING`) | `400` invalid ID, `404` machine not found |

`MachineServiceInterface` gains `GetPropagation`. `MachineHandler` gains:

```go
func (handler *MachineHandler) GetPropagation(w http.ResponseWriter, r *http.Request)
```

It reuses the existing `parseMachineID` helper in the `handler` package and
maps `errors.ErrMachineNotFound` to `404`.

There is no `/groups/{name}/propagation` route. Admins filter the machine list
by `GroupID`/`GroupName`.

## 9. Orchestrator gRPC Server

New package `internal/orchestrator/api/grpcserver`:

```go
// propagationController declares the propagation operations the gRPC stream
// consumes. The adapter owns the interface, so it never depends on the
// concrete service.
type propagationController interface {
    Register(ctx context.Context, machineID int64, sender propagation.MessageSender) (uint64, error)
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
func (server *PropagationServer) Connect(stream pb.Propagation_ConnectServer) error
```

`Connect`:

1. Read `authorization` metadata, parse the bearer token with
   `auth.ParseSessionToken`, and derive the machine ID. Any failure returns
   `codes.Unauthenticated`.
2. Create a cancellable child context.
3. Register a `streamSender` for the machine, receiving a connection ID. If the
   machine no longer exists, return `codes.Unauthenticated`.
4. `defer` `Unregister(machineID, connID)`. The connection ID makes the
   unregister a no-op when a newer stream has already replaced this one.
5. Call `service.DeliverSnapshot(ctx, machineID)`.
6. Receive on `stream.Recv()` in a goroutine, selecting against the
   handler's context: each ACK goes to `service.Acknowledge`; `io.EOF` ends
   the stream normally; any other receive error is returned; and a cancelled
   context — an abandoned wedged send — ends the loop so returning finalizes
   the stream and the client reconnects.

`streamSender` implements `propagation.MessageSender`. It serializes `Send`
with a mutex, converts the domain message to proto, and honours the caller's
context: a send that outlives its deadline (or fails) cancels the child
context, which ends the receive loop and tears the stream down, while the
sender is marked dead so a wedged peer can neither block the propagation
worker nor accept another write.

## 10. Wiring

### 10.1 Orchestrator

`api.NewRouter` currently constructs all stores and services and returns only
an `http.Handler`. Add a runtime result so `main` can reach the propagation
service:

```go
type Runtime struct {
    HTTP        http.Handler
    Propagation *service.PropagationService
}

func NewRuntime(db *sql.DB, cfg *config.Config, logger *slog.Logger) (*Runtime, error)
func NewRouter(db *sql.DB, cfg *config.Config) (http.Handler, error) // wraps NewRuntime with a nil logger
```

`NewRuntime` builds the stores, the propagation service, the domain services
with the notifier, the handlers, and seeds the machines as before.

`cmd/orchestrator/main.go`:

- Loads `cfg.GRPCAddr` from `AEGIS_GRPC_ADDR` (default `:9090`).
- Calls `runtime.Propagation.Start(ctx)` and fails fast if it returns an error.
- Listens on `cfg.GRPCAddr`, creates a `grpc.NewServer` with the 16 MiB
  send/receive limits, registers
  `grpcserver.NewPropagationServer(runtime.Propagation, cfg.SessionSecret, logger)`,
  and serves in a goroutine.
- On shutdown, calls `GracefulStop` (bounded), cancels the propagation context,
  and shuts down the HTTP server as today.

### 10.2 Sentinel

`cmd/sentinel/main.go`:

- Constructs the client with `cfg.PropagationAddress`.
- Constructs the propagation runner with `cfg.RulesStatePath` and closes the
  client on exit.
- Registers two daemon tasks: `heartbeat` and `propagation`.

`daemon.Session` gains `MachineID int64`, populated by
`client.Authenticate` from `authenticated.Machine.ID`.

## 11. Sentinel

### 11.1 Configuration

The Sentinel JSON gains two optional fields:

```json
{
  "propagation_address": "172.30.0.10:9090",
  "rules_state_path": "/var/lib/aegis/rules.json"
}
```

- `propagation_address` defaults to `orchestrator_url`'s hostname plus port
  `9090`. An explicit value must be a valid `host:port`.
- `rules_state_path` defaults to `/var/lib/aegis/rules.json`. The parent
  directory is created with `0o755` when missing; failure to create or write it
  is a startup error.
- E2E uses `rules_state_path = /tmp/aegis-rules.json`, because the distroless
  nonroot image cannot write `/var/lib/aegis` without a mounted volume.

### 11.2 Client

`client.New` changes to accept the propagation address and returns
`(*Client, error)`:

```go
func New(baseURL *url.URL, propagationAddress string, labKey []byte, timeout time.Duration, logger *slog.Logger) (*Client, error)
```

It creates a gRPC client with insecure transport credentials and the 16 MiB
call options. The `Client` gains `Close() error` and:

```go
func (c *Client) ConnectPropagation(ctx context.Context, token string) (propagation.Stream, error)
```

`ConnectPropagation` appends `authorization: Bearer <token>` to the outgoing
metadata and opens the stream. The returned wrapper converts proto messages to
and from `internal/common/propagation`. The stream contract is the shared
`propagation.Stream` interface of §5.1, so the runner's `Connector` and the
client use the same return type.

### 11.3 State file

New package `internal/sentinel/propagation`. The state is:

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
```

A revoke persists the new epoch and sequence with `policy_id: null`,
`policy_name: ""`, `policy_version: 0`, `policy_type: ""`, and `rules: []`.
The sequence is never cleared.

`FileStore.Load` returns a zero state when the file does not exist. A corrupt
file returns an error; the runner logs a warning and starts from the zero
state, which accepts the Orchestrator's next snapshot. `FileStore.Save` writes
to a temporary file in the same directory with mode `0o600`, syncs, and renames
over the target.

### 11.4 Runner

New package `internal/sentinel/propagation`:

```go
// StateStore persists the applied propagation state. *FileStore implements
// it; the runner depends on the interface so persistence failures are
// testable.
type StateStore interface {
    Load() (State, error)
    Save(state State) error
}

type Connector interface {
    ConnectPropagation(ctx context.Context, token string) (propagation.Stream, error)
}

type SessionRenewer interface {
    Authenticate(ctx context.Context, id identity.Identity) (*daemon.Session, error)
}

type Runner struct {
    // store, connector, renewer, logger
}

func New(store StateStore, connector Connector, renewer SessionRenewer, logger *slog.Logger) *Runner
func (runner *Runner) Run(ctx context.Context, session *daemon.Session) error
```

`Run`:

1. Load the state file.
2. Loop until the context is cancelled:
   - Open a stream with the current token.
   - If the error is `codes.Unauthenticated`, renew the session with
     `renewer.Authenticate` using `session.IP`/`session.MAC`, update the token,
     and retry.
   - On any other error, log a warning and retry with exponential backoff
     (1s, 2s, 4s, … capped at 30s).
   - On a message, call `apply`, send the returned ACK, and log the outcome.
3. Return `nil` on context cancellation.

`Run` never returns a non-nil error, so a propagation failure can never kill the
daemon.

### 11.5 Acceptance and persistence

`apply` is the whole contract:

1. Reject an empty epoch, empty sequence, or `machine_id` that does not match
   `session.MachineID` with a NACK.
2. If `message.Epoch != state.Epoch`, accept and start from a zero state. This
   is the only case where a lower sequence is accepted.
3. Otherwise, if `message.Sequence <= state.LastSequence`, NACK with
   `stale sequence` and do not touch the state.
4. Build the next state:
   - `PACKAGE`: require a positive policy ID, non-empty name, positive version,
     a policy type of `WHITELIST` or `BLACKLIST`, and at least one rule.
     Validate every rule with the shared `internal/common/rules` validator. On
     failure, NACK with the validation reason.
   - `REVOKE`: clear the policy metadata and rules.
   - Any other kind: NACK with `unknown message kind`.
5. Save the next state atomically. On failure, NACK with
   `failed to persist state` and leave the old state in memory.
6. On success, replace the in-memory state and ACK.

`internal/common/rules` defines `Rule` (type, value, protocol, port) and
`Validate(rules.Rule) error`, extracted from the type/value/protocol/port
checks in `domain.Rule.Validate`. `domain.Rule.Validate` keeps its own
`PolicyID > 0` check and then delegates the rest. The Orchestrator and the
Sentinel enforce identical rule syntax, and the existing error order is
preserved. The shared validator additionally rejects unknown rule types with
`errors.ErrInvalidRuleType` (a deliberate tightening: the previous
`domain.Rule.Validate` fell through silently for unknown types); existing valid
inputs are unaffected.

### 11.6 Log records

| Record | Level | Fields |
| --- | --- | --- |
| `propagation connected` | info | machine |
| `package applied` | info | sequence, policy, version, rules |
| `revoke applied` | info | sequence |
| `propagation message rejected` | warn | sequence, reason |
| `propagation stream failed` | warn | error |
| `propagation session renewed` | info | fingerprint |
| `propagation state corrupt` | warn | error |

The session token is never logged; only `logging.Fingerprint` is used.

## 12. Reliability and Failure Modes

| Failure | Behavior |
| --- | --- |
| Orchestrator crashes after commit, before notify | The Sentinel's stream breaks and reconnects; the initial snapshot carries the committed state. |
| Orchestrator enqueue is lost | The next change or reconnect sends a fresh snapshot. |
| Machine is offline | No row and no send. On connect, the initial snapshot is sent and recorded. |
| Send fails | Row becomes `FAILED`; the stream is cancelled; reconnect sends a new snapshot. |
| Sentinel save fails | NACK `failed to persist state`; old state remains; the next change/reconnect retries. |
| Sentinel receives a stale sequence | NACK `stale sequence`; state is unchanged. |
| Sentinel restarts | Loads the last sequence; the next snapshot has a higher sequence and is accepted. |
| Sentinel state file deleted | Starts from zero; the next snapshot is accepted. |
| Orchestrator database recreated | New epoch; Sentinels accept the first snapshot regardless of sequence and reset. |
| Policy/group deleted | Machine IDs are resolved before deletion and the revoke is enqueued after it succeeds; `ON DELETE SET NULL` only runs afterward. |
| Policy renamed | Version bump plus notify; the new package carries the new name and version. |

No automatic retry loop exists. A NACK is visible in the status API and the
next change or reconnect is the recovery path. This is deliberate: a retry
loop against a permanently broken Sentinel would only generate noise.

## 13. Security

- The stream is authenticated by the existing HMAC-signed session token. The
  token is presented only in gRPC metadata and is never logged.
- The gRPC transport is plaintext in US09, matching the existing HTTP API. TLS
  for the propagation channel is a follow-up; the message contains policy data,
  not secrets, but confidentiality is still desirable.
- The state file contains rules and policy metadata, not keys. It is written
  with mode `0o600`.
- The server enforces the 16 MiB message limit to bound memory use.
- A token for a removed machine is rejected. A message whose `machine_id` does
  not match the authenticated machine is NACKed by the Sentinel.

## 14. Tooling and CI

- `proto/propagation.proto` is the source of truth.
- `make proto` runs:
  ```sh
  protoc --proto_path=proto \
    --go_out=pkg/pb --go_opt=paths=source_relative \
    --go-grpc_out=pkg/pb --go-grpc_opt=paths=source_relative \
    proto/propagation.proto
  ```
- `pkg/pb/**` remains gitignored. `make proto` is a prerequisite of `build`,
  `tests`, `coverage`, and `e2e`.
- `make init` installs `protoc-gen-go` and `protoc-gen-go-grpc`; `protoc`
  itself is a documented system prerequisite.
- CI installs `protobuf-compiler` before `make coverage`, and its path filters
  gain `proto/**` and `Makefile`.
- `e2e/Dockerfile` installs `protobuf`/`protobuf-dev` in the build stage and
  runs `make proto` before `go build`.
- `go.mod` gains `google.golang.org/grpc` and
  `google.golang.org/protobuf`.

## 15. Testing Strategy

Follows the vertical slice: store → service → API/gRPC → Sentinel.

**Store**

- `Create` assigns `1, 2, 3…` per machine independently.
- `Create` starts at `PENDING` with no `sent_at` and stores the serialized
  payload verbatim.
- `MarkSent`, `MarkFailed`, `Acknowledge` transition the right row and leave
  other rows untouched.
- `LatestByMachine` and `ListLatest` return only the highest sequence per
  machine.
- `Epoch` returns a stable 32-hex value across calls and a new value after a
  database recreation.

**Service**

- The notifier resolves policy → groups → machines, group → machines, and
  machine → itself. The latest intent wins: package then revoke ends revoked;
  revoke then package ends packaged.
- The snapshot builder sends a package when the machine has a group, policy,
  and rules; it sends a revoke when any link is missing or the policy is empty.
- `Create`/`MarkSent`/send/ACK/NACK/FAILED are exercised through the worker
  with a fake sender.
- A reconnect that registers a new sender before the old handler unregisters
  keeps the new sender; the old unregister is a no-op.
- `RemoveGroup`/`RemovePolicy` resolve machines before the delete and enqueue
  only after it succeeds; a resolution error aborts the delete, and a failed
  delete leaves the pending set untouched.
- `MachineService.ListMachines` merges the latest row; a machine without one is
  `PENDING`.
- `GetPropagation` returns `PENDING` for a machine with no row and
  `ErrMachineNotFound` for a missing machine.
- Hook tests: every mutation calls the expected notifier method; no-op updates
  and no-op renames do not; `RemoveGroup` and `RemovePolicy` resolve before the
  store delete and enqueue after it.

**HTTP handler**

- `GET /machines` includes `propagation`.
- `GET /machines/{id}/propagation` returns `200` for a known machine
  (including `PENDING`), `400` for an invalid ID, and `404` for a missing
  machine.
- Assertions check HTTP status codes only, per AGENTS.md.

**gRPC server**

- A valid token connects, receives the initial snapshot, and its ACK updates
  the row.
- A missing, malformed, or expired token returns `Unauthenticated`.
- A token for a removed machine returns `Unauthenticated`.
- An ACK for another machine's sequence is ignored.
- A reconnecting stream replaces the sender, and the old handler's unregister
  does not remove it.
- A 5 MiB package round-trips over `bufconn` to prove the message limit.

**Sentinel**

- `FileStore` round-trips, handles a missing file, resets on a corrupt file,
  and preserves the sequence across a revoke.
- `apply`: package accepted; stale sequence NACKed; epoch change accepted;
  machine mismatch NACKed; invalid package NACKed; unknown policy type NACKed;
  save failure NACKed.
- `Runner`: reconnects after a stream error, renews on `Unauthenticated`, never
  returns an error, and stops cleanly on context cancellation.
- Config: `propagation_address` default and override; invalid address;
  `rules_state_path` default and override.

**E2E**

- Start Orchestrator + Sentinel; create a policy, rules, group, link; wait for
  `package applied`; assert `GET /machines` shows `ACKED` and a non-null
  `duration_ms`.
- Change a rule; assert a new sequence and an updated package.
- Rename the policy; assert a new package with the new name and version.
- Unlink the policy; assert `revoke applied` and a `REVOKE` row.
- Delete a linked policy and a group with machines; assert revokes.
- A machine that never connects shows `PENDING`.
- Restart the Sentinel with the state file; assert the new snapshot is
  accepted and the old sequence is not.

**Coverage**

Add the new packages to `Makefile`'s `TEST_PACKAGES`:
`./internal/common/rules`, `./internal/common/propagation`,
`./internal/orchestrator/api/grpcserver`, and
`./internal/sentinel/propagation`. The 75% gate remains.

## 16. Documentation Updates

- `.env.example` gains `AEGIS_GRPC_ADDR`.
- `examples/sentinel.config.json` gains `propagation_address` and
  `rules_state_path` (with the e2e-safe `/tmp` path documented separately).
- The README documents the `protoc` prerequisite and `make proto`.
- The US06 design gets a note that the propagation channel is now a gRPC
  stream, superseding its earlier parenthetical.

## 17. Done Criteria

1. A machine in a group with a linked policy receives a package containing
   every rule of that policy, on connect and after every relevant change.
2. Renaming a linked policy pushes a new package with the new name and version.
3. Unlinking a policy, deleting a linked policy, deleting a group with
   machines, and unlinking a machine all push a revoke.
4. Every sent message is retained in `propagations` with its serialized
   payload; the API exposes only the latest row per machine.
5. `GET /machines` and `GET /machines/{id}/propagation` show `PENDING` for a
   machine that never received a message, and `ACKED`/`NACKED`/`FAILED` with
   timestamps and duration otherwise.
6. A Sentinel accepts a newer sequence even when the policy name changes or
   the policy version goes backwards, and rejects a stale sequence.
7. A Sentinel restart preserves the last sequence; a revoke preserves it too.
8. An Orchestrator database recreation changes the epoch and resets the
   Sentinel's sequence baseline.
9. Store, service, handler, gRPC, Sentinel, config, and e2e tests pass;
   coverage meets the 75% gate; `make build`, `make tests`, `make coverage`,
   and `make e2e` work on a fresh clone after `make proto`.

## 18. Open Questions

1. **Transport confirmation.** The design uses gRPC because the old US09 plan
   did and because push + ACK fit a bidirectional stream. The US06 design's
   parenthetical says propagation would be push-over-HTTPS with the Sentinel
   serving the inbound endpoint. Confirm gRPC, or switch to an HTTP long-poll
   design (no proto tooling, but more polling/ACK-correlation code).
2. **Offline status semantics.** When a machine is offline and its policy
   changes, no message is sent and no row is created, so `GET /machines` keeps
   showing the previous `ACKED` row. Should it instead show `PENDING` (or a
   stale marker) because the current desired state is unconfirmed? The answer
   changes the `ListMachines` merge logic and may require creating a pending
   row at notification time.
3. **P0 branch strategy.** The policy-version/empty-policy prerequisite is
   currently stacked on `feat/issue-19-rule-propagation`, not on its own
   branch. Keep it stacked, or split and merge it to `develop` first?
4. **Retry policy.** Confirm that a NACK/FAILED message is not retried
   automatically and that the next change or reconnect is the recovery path.
5. **TLS.** Confirm that plaintext gRPC is acceptable for US09, with TLS as a
   follow-up.

## 19. Follow-ups

- TLS for the propagation stream (and the REST API).
- Retention/pruning for old `propagations` rows.
- Local enforcement of the received rules (US10, #20).
- A live admin feed for propagation status, if polling `GET /machines` is not
  enough for the UI.
