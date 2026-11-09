# Propagation metrics at /metrics: design

Status: approved scope, not implemented. Replaces R4 and R5 in `orchestrator-api-proposal.md`.

## Intent

The Monitor panel needs P95 latency, the lab with the highest mean latency, and per-lab mean and P95. The user prefers that the Orchestrator emits raw measurements and Prometheus derives the metrics, which also gives history over time. The admin UI will query Prometheus through its own nginx. This spec covers only the Orchestrator side: emitting the measurements and exposing them.

## Goals

- `GET /metrics` serves Prometheus text format, with no authentication.
- A latency histogram of package propagations that were acknowledged, labelled by group name.
- A counter of package propagation outcomes, labelled by group name and status.
- Enough to answer in PromQL: overall P95, mean per group, highest mean group, P95 per group.

## Non-goals

- No "mean time to full propagation" metric. Propagations carry no group or batch ID, so a policy reaching a whole lab is not derivable today. That tile keeps its placeholder; a later change decides how to group propagations.
- No machine-state gauges (online, failed, offline). The client counts those from `GET /groups/{name}/machines`.
- No Go runtime or process collectors beyond what `client_golang` registers by default on a new registry (none are added on purpose).
- No Prometheus container or nginx proxy rules in this change. Those belong to the deployment and client tasks.

## Anti-goals

- No per-machine labels. Series count must stay proportional to the number of groups.
- Metrics code must never fail or slow a propagation. Recording is in-memory and cannot return an error.
- The service layer must not import Prometheus.

## Constraints

- Dependency: `github.com/prometheus/client_golang v1.24.1` is already in `go.mod` as an indirect dependency. Make it direct.
- Layering: `service` declares a small consumer-owned interface. A new package `internal/orchestrator/metrics` implements it with Prometheus. `api` wires the two.
- `/metrics` is served on the same listener as the admin API (`AEGIS_ADDR`, default `:8080`). The deployment must not publish that port outside the host and nginx must not proxy `/metrics`.

## Decisions

1. **Interface.** In `service/propagation_metrics.go`:
   `type PropagationMetrics interface { RecordPropagation(group, status string, latency time.Duration) }`. Status values are the strings `acked`, `nacked` and `failed`. `latency` matters only for `acked` and is ignored otherwise.
2. **Instruments** (in `metrics` package, own `prometheus.Registry`, not the global default):
   - `aegis_propagation_duration_seconds`, histogram, label `group`, buckets `0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30`. Chosen around the observed 117 ms P95 and the 10 s send timeout. A builder may adjust buckets.
   - `aegis_propagations_total`, counter, labels `group` and `status`.
3. **What is recorded.** Only propagations of kind PACKAGE. Revokes are skipped so teardown does not distort latency.
4. **Where.** `PropagationService.Acknowledge` records `acked` or `nacked` after the store write succeeds. `dispatch` records `failed` after `MarkFailed`. The service gains `SetMetrics(PropagationMetrics)`, which keeps `NewPropagationService`'s many callers unchanged. A nil metrics value disables recording.
5. **Latency source.** `store.Acknowledge` returns only an error. After a successful write the service reads `LatestByMachine`; if the latest row has the same sequence, kind PACKAGE and a non-nil `SentAt`, latency is `ackTime - SentAt`, where `ackTime` is the same `service.now()` value passed to the store. A late ack for a superseded message is not recorded in the histogram. Accepted: the counter still counts it only when the row is the latest, so both stay consistent.
6. **Group name.** Resolved with `machines.GetByID` then `groups.GetByID`. A machine without a group, or a lookup error, records the group `unassigned`.
7. **Handler.** `metrics.Handler()` returns an `http.Handler` built with `promhttp.HandlerFor(registry, promhttp.HandlerOpts{})`. A small `MetricsHandler` in the handler package registers it at `GET /metrics`.

## Left to the builder

- Exact bucket list.
- Help strings.
- Whether `metrics` exposes the registry or only the handler.
