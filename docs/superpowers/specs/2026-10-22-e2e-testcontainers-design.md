# E2E Test Suite - TestContainers Design

- **Date:** 2026-10-22
- **Status:** Design approved by the human partner; pending written-spec review
- **Related:** docs/superpowers/specs/2026-10-22-sentinel-daemon-design.md section 11, follow-ups

## 1. Context And Goal

The repository already ships an end-to-end suite (e2e/, build tag e2e,
invoked via make e2e) that builds the real Orchestrator and Sentinel binaries
and runs them as host processes. Its current coverage is the Sentinel
authentication failure surface: valid-key happy path, wrong lab key, unreachable
Orchestrator, missing configuration.

The Sentinel daemon design spec explicitly deferred the parts of the system that
were not yet safe to exercise this way, the main one being:

> An end-to-end check that the Sentinel flips the machine to CONNECTED in the
> Orchestrator, which is the only test that proves both halves agree on the wire
> contract.

This initiative migrates the suite to Testcontainers and closes that gap.

## 2. Goal (As Agreed With The Human Partner)

Replace the host-process execution model with containers, so the suite:

1. Runs the Orchestrator and Sentinel as isolated containers on a shared
   Docker network - a faithful topology for a two-tier, per-endpoint system.
2. Verifies the full authentication loop end-to-end against the real binaries,
   including the machine status transition to CONNECTED.
3. Verifies failure behavior (wrong lab key) inside the same topology.
4. Remains the single source of truth for E2E execution (make e2e semantics).

## 3. Scope

In scope:

- New multi-sentinel fleet scenario: Orchestrator + N Sentinel containers.
- Assertions: machine status transitions, authentication outcomes, process exit
  codes, emitted log records.
- A Dockerfile that produces an image containing both binaries.
- Makefile e2e target semantics and CI wiring.

Out of scope:

- The rule-propagation channel (not implemented yet).
- A general-purpose daemon lifecycle/signal suite (deferred).
- Local missing-config and unreachable-Orchestrator scenarios that depend on
  host files/network paths (deferred; the wrong-key scenario covers the
  authentication-failure axis inside the container model).

## 4. Decisions

| Decision | Choice | Rationale |
| --- | --- | --- |
| Approach | Approach 1: single in-repo image, both binaries | Only one artifact to maintain; different entrypoints per container |
| Dockerfile | New, multi-stage; context = repository root, path e2e/Dockerfile | Keeps the build context aligned with the module layout |
| Runtime base | Builder: pinned Go image; Runtime: minimal image with no shell | Go binaries are static; minimal runtime reduces surface area for a test image |
| Network | User-defined bridge network with a fixed IPAM subnet (planned, e.g. 172.30.0.0/24) | IP-based machine identity makes static, planned IPs the honest lab simulation; deterministic machines.csv |
| IP plan | Orchestrator planned IP (172.30.0.10), sentinel N planned IPs (172.30.0.11+) | Enables writing the accepted-machines registry before any container starts (startup-only SyncMachines) |
| Secrets and config | Lab key, machines registry, Sentinel JSON injected into containers | No host paths leak into containers |
| Status assertions | GET /machines via published host port against the Orchestrator container | Proves the CONNECTED flip that the spec called out as the missing wire-contract check |

## 5. Architecture

```
e2e/                      # package (build tag: e2e)
  Dockerfile              # multi-stage, builds aegis-orchestrator + aegis-sentinel
  harness_test.go         # container lifecycle, network, file injection, wait/assert helpers
  fleet_e2e_test.go       # fleet scenarios

Docker (per run)
  +-----------------------+        +-----------------------+
  | aegis-orchestrator   |<------->| aegis-sentinel-1     |
  | 172.30.0.10          |  auth  | 172.30.0.11          |
  +-----------------------+        +-----------------------+
       (port published to host)          ...
```

1. Create the user-defined network with the fixed subnet.
2. Write the accepted-machines registry (machines.csv) using the planned
   static IPs of the sentinel fleet.
3. Start the Orchestrator container (registry file injected, port published).
4. Start Sentinel containers (planned static IPs, Sentinel JSON injected, pointing
   at the Orchestrator).
5. Assert via GET /machines plus process behavior / logs.
6. Tear down containers.

## 6. Testing Strategy

Scenario 1 - Fleet happy path with graceful stop (recommended scope):

- Orchestrator + 3 Sentinels on the network.
- All Sentinels authenticate; Orchestrator transitions each machine to
  CONNECTED.
- daemon running / daemon stopping records emitted; first signal -> exit 0.

Scenario 2 - Wrong lab key rejected inside the fleet:

- Sentinel container configured with a wrong lab key.
- Expected: exit code 1 and log record authentication rejected by the orchestrator.

## 7. Done Criteria

1. make e2e runs a Testcontainers suite requiring Docker.
2. The suite exercises the real binaries inside containers.
3. The fleet scenario proves the Sentinel -> Orchestrator wire contract
   (CONNECTED transition) end to end.
4. The wrong-key failure scenario passes inside the container topology.
5. The suite passes locally (Docker available) and in CI (e2e job added).

## 8. Follow-ups

- Add the daemon lifecycle/signal suite (second-signal forced exit, etc.).
- Add unreachable-Orchestrator and missing-config scenarios under the container
  model (or keep host-process variants where host paths matter).
- Introduce the push-based rule-propagation E2E when implemented.

---

Next step per the approved path: submit this design for your written-spec review,
then we proceed to the implementation plan. No source files have been written yet.
