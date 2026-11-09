# Propagation metrics at /metrics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Record propagation latency and outcomes per group and expose them at `GET /metrics` for Prometheus.

**Architecture:** The service records through a consumer-owned interface. A new `metrics` package implements it with `client_golang` and provides the HTTP handler. The router wires both.

**Tech Stack:** Go 1.26, chi, `github.com/prometheus/client_golang v1.24.1`.

**Spec:** `docs/superpowers/specs/2026-11-08-metrics-endpoint-design.md`

## Global Constraints

- Follow `AGENTS.md`: doc comment on every symbol starting with its name; external test packages; names `Test<Type>_<Method>_<Scenario>_<ExpectedResult>`; two-step test writing; helpers at the end of the file; handler tests assert status codes only.
- The `service` package must not import `client_golang`.
- Labels: `group` and `status` only. Never a machine label.
- Status strings: `acked`, `nacked`, `failed`. Fallback group label: `unassigned`.
- Add `./internal/orchestrator/metrics` to `TEST_PACKAGES` in the `Makefile`.
- Do not commit. Leave changes for developer review (`AGENTS.md` 6.1).

## Review Focus

- An ack for a machine that has no group records under `unassigned` and does not panic (Task 2).
- An ack for a superseded sequence (latest row has a higher sequence) records nothing in the histogram (Task 2).
- A REVOKE propagation ack records nothing (Task 2).
- A failed send increments `failed` and never observes the histogram (Task 2).
- A nil metrics value leaves `Acknowledge` and `dispatch` working unchanged (Task 2).

---

### Task 1: `metrics` package

**Files:**
- Create: `internal/orchestrator/metrics/propagation_metrics.go`
- Create: `internal/orchestrator/metrics/propagation_metrics_test.go`
- Modify: `go.mod` (make `client_golang` direct via `go mod tidy`), `Makefile` (`TEST_PACKAGES`)

**Interfaces:**
- Consumes: `github.com/prometheus/client_golang/prometheus` and `.../promhttp`.
- Produces:
  - `type PropagationMetrics struct` with unexported registry and instruments.
  - `func NewPropagationMetrics() *PropagationMetrics`.
  - `func (m *PropagationMetrics) RecordPropagation(group, status string, latency time.Duration)`.
  - `func (m *PropagationMetrics) Handler() http.Handler`.

- [ ] **Step 1: Write the test skeleton** (package `metrics_test`):
  - `TestPropagationMetrics_RecordPropagation_WithAckedStatus_ObservesHistogramAndCounter`
  - `TestPropagationMetrics_RecordPropagation_WithFailedStatus_IncrementsOnlyCounter`
  - `TestPropagationMetrics_Handler_AfterRecording_ExposesSeriesPerGroup`

- [ ] **Step 2: Fill in the bodies.** Scrape through `httptest.NewRecorder` and `m.Handler().ServeHTTP`, then assert on the body text. Test 1: record `("LCC1", "acked", 120*time.Millisecond)`; the body contains `aegis_propagation_duration_seconds_count{group="LCC1"} 1` and `aegis_propagations_total{group="LCC1",status="acked"} 1`. Test 2: record `("LCC1", "failed", 0)`; the body contains `aegis_propagations_total{group="LCC1",status="failed"} 1` and does not contain `aegis_propagation_duration_seconds_count{group="LCC1"}`. Test 3: record for `LCC1` and `LCC2`; both group labels appear.

- [ ] **Step 3: Run to confirm failure.** Run `go test ./internal/orchestrator/metrics -v`. Expected: build failure.

- [ ] **Step 4: Implement** with a private `prometheus.NewRegistry()`, a `HistogramVec` (`aegis_propagation_duration_seconds`, label `group`, buckets from the spec) and a `CounterVec` (`aegis_propagations_total`, labels `group`, `status`). Observe the histogram only when status is `acked`. Run `go mod tidy`.

- [ ] **Step 5: Run to confirm pass.** Run `go test ./internal/orchestrator/metrics -v`. Expected: PASS.

### Task 2: Record from `PropagationService`

**Files:**
- Create: `internal/orchestrator/service/propagation_metrics.go`
- Modify: `internal/orchestrator/service/propagation_service.go` (struct, `Acknowledge` at lines 241-260, `dispatch` after `MarkFailed` near line 366)
- Modify: `internal/orchestrator/service/propagation_service_test.go`
- Create: `internal/orchestrator/service/mock_propagation_metrics_test.go`

**Interfaces:**
- Consumes: `domain.PropagationStore.LatestByMachine`, `domain.MachineStore.GetByID`, `domain.GroupStore.GetByID`, `domain.PropagationKind` (PACKAGE constant in `domain/propagation.go`).
- Produces:
  - `type PropagationMetrics interface { RecordPropagation(group, status string, latency time.Duration) }` (documented on the interface).
  - `func (service *PropagationService) SetMetrics(metrics PropagationMetrics)`.
  - `MockPropagationMetrics` in the test package, storing calls as `[]MetricCall{Group, Status string; Latency time.Duration}` with a `Calls() []MetricCall` accessor.

- [ ] **Step 1: Write the test skeleton** in `propagation_service_test.go`, reusing its existing arrange helpers (read them first; extend one to optionally call `SetMetrics`):
  - `TestPropagationService_Acknowledge_WithAckedPackage_RecordsAckedWithLatency`
  - `TestPropagationService_Acknowledge_WithNackedPackage_RecordsNacked`
  - `TestPropagationService_Acknowledge_WithSupersededSequence_RecordsNothing`
  - `TestPropagationService_Acknowledge_WithRevokeKind_RecordsNothing`
  - `TestPropagationService_Acknowledge_WithMachineWithoutGroup_RecordsUnassigned`
  - `TestPropagationService_Acknowledge_WithoutMetrics_StillAppliesAck`
  - `TestPropagationService_Dispatch_WithFailedSend_RecordsFailed`

- [ ] **Step 2: Fill in the bodies.** Seed a group, a machine in it, and a PACKAGE row with `SentAt = now - 150ms` through the mock store. Assert exact `MetricCall` values: `{Group: "<group name>", Status: "acked", Latency: 150 * time.Millisecond}`; for nacked `Status: "nacked"`; superseded and revoke cases assert zero calls; unassigned asserts `Group: "unassigned"`; the failed-send case uses a sender whose `Send` returns an error and asserts `{Status: "failed"}`.

- [ ] **Step 3: Run to confirm failure.** Run `go test ./internal/orchestrator/service -run "TestPropagationService_(Acknowledge|Dispatch)" -v`. Expected: build failure on `SetMetrics`.

- [ ] **Step 4: Implement.** Add the interface and `SetMetrics`. In `Acknowledge`, after the store write succeeds: `LatestByMachine`; return silently on error or when the latest sequence differs or kind is not PACKAGE; compute latency from `SentAt` and the same `service.now()` value used for the store write (call `now` once and reuse it); call `RecordPropagation(groupName, "acked"|"nacked", latency)`. In `dispatch`, after `MarkFailed` runs for a package message, call `RecordPropagation(groupName, "failed", 0)`. Add a private helper `groupNameOf(ctx, machineID int64) string` that returns `"unassigned"` for no machine, no group or any error. Every call is guarded by `service.metrics != nil`.

- [ ] **Step 5: Run to confirm pass.** Run `go test ./internal/orchestrator/service -v`. Expected: PASS, including all existing tests.

### Task 3: `/metrics` route and wiring

**Files:**
- Create: `internal/orchestrator/api/handler/metrics_handler.go` and `metrics_handler_test.go`
- Modify: `internal/orchestrator/api/router.go` (`NewRuntime`)
- Modify: `internal/orchestrator/api/router_test.go`

**Interfaces:**
- Consumes: `metrics.PropagationMetrics.Handler() http.Handler`, `service.PropagationService.SetMetrics`.
- Produces: `func NewMetricsHandler(h http.Handler) *MetricsHandler` and `func (handler *MetricsHandler) RegisterRoutes(r chi.Router)` registering `GET /metrics`.

- [ ] **Step 1: Write the test skeleton:**
  - `TestMetricsHandler_RegisterRoutes_WithGetRequest_ReturnsStatus200` (wrap a stub `http.HandlerFunc` that writes 200)
  - `TestMetricsHandler_RegisterRoutes_WithPostRequest_ReturnsNon200Status`
  - In `router_test.go`: `TestNewRouter_WithMetricsRequest_ReturnsStatus200`

- [ ] **Step 2: Fill in the bodies.** Handler tests use `performRequest` and `assertStatusCode`. The router test calls `GET /metrics` on the built router and expects status 200.

- [ ] **Step 3: Run to confirm failure.** Run `go test ./internal/orchestrator/api/... -run "Metrics" -v`. Expected: build failure.

- [ ] **Step 4: Implement and wire.** In `NewRuntime`, create `propagationMetrics := metrics.NewPropagationMetrics()`, call `propagationService.SetMetrics(propagationMetrics)`, and add `handler.NewMetricsHandler(propagationMetrics.Handler())` to `registerHandlers`. Vector metrics print nothing until a first label set is recorded, so the router test must not expect series names on a fresh router.

- [ ] **Step 5: Verify.** Run `make tests` and `make coverage`. Expected: PASS and total coverage at or above 75%. Then start the Orchestrator locally with the required `AEGIS_*` variables and run `curl -s localhost:8080/metrics | head`. Expected: Prometheus text.
