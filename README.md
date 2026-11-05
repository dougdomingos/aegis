# Aegis: Distributed Network Access Control for Academic Laboratories

## Overview
Aegis is a centralized management and distributed network filtering system designed to enforce dynamic internet access policies across computer laboratory environments during academic evaluations. Developed as part of an undergraduate capstone project (TCC) at the Federal University of Campina Grande (UFCG), Aegis addresses the scalability, maintainability, and single-point-of-failure (SPOF) limitations inherent in traditional centralized proxy solutions.

## Key Features
- **Multi-Layer Traffic Filtering:** Enforces access rules across domain levels (L7) and IP/port/protocol levels (L4).
- **Logical Group Management:** Enables administrators to group laboratory machines and apply distinct access policies per room or examination context.
- **Real-Time Policy Propagation:** Pushes rule updates instantly to target endpoints without requiring service restarts or interrupting active user sessions.
- **Autonomous Endpoint Operation (Failover):** Local agents maintain active network filtering using cached policies or fallback defaults if the central server becomes unreachable.
- **Low Resource Footprint:** Designed for optimal performance on modest client hardware within ephemeral operating system environments.
- **Audit Logging & Real-Time Monitoring:** Tracks administrative operations and provides real-time visibility into endpoint connectivity and filtering status.

## System Architecture Overview
The system decouples policy administration from packet processing by employing a two-tiered architecture:

| Component | Description & Responsibilities |
| :--- | :--- |
| **Orchestrator** | Central control plane responsible for rule storage, policy definition, logical group management, and real-time distribution of configurations via API. |
| **Sentinel** | Lightweight daemon running locally on each endpoint. Receives policies from the Orchestrator, configures local kernel network filters, and enforces traffic constraints autonomously. |

## Technical Requirements & Constraints
- **Target OS:** Linux-based distributions (optimized for ephemeral live desktop images).
- **Client Storage Footprint:** Maximum local installation footprint under 100 MB.
- **Client Resource Limits:** Runtime RAM usage limited to under 100 MB; CPU utilization capped below 5%.
- **Propagation Latency:** Policy distribution completes within 5 seconds across group endpoints.
- **Filtering Overhead:** Local request processing adds no more than 100 ms of average network latency.

## Academic Context
This software is developed as an undergraduate capstone project (Trabalho de Conclusão de Curso - TCC) under the Guardians UFCG initiative.

## Building & Running

### Prerequisites

- Go 1.26+
- `protoc` (the protocol buffers compiler) as a system package. The Go plugins
  it needs (`protoc-gen-go` and `protoc-gen-go-grpc`) are installed by
  `make init`.

### Build

```sh
make init    # install development tooling, including the protoc plugins
make proto   # generate pkg/pb from proto/propagation.proto
make build   # produce bin/aegis-orchestrator and bin/aegis-sentinel
```

The generated code under `pkg/pb` is never committed; `make tests`,
`make coverage` and `make e2e` run `make proto` as a prerequisite.

### KVM filtering tests

The VM suite runs the real binaries in KVM VMs on an isolated network and checks
that real connections are blocked or allowed for WHITELIST and BLACKLIST
policies, each with domain and IP rules. It also covers revoking a policy and
rebooting a Sentinel. It runs locally and manually, not in CI.

Prerequisites: a running `libvirtd`, membership in the `kvm` and `libvirt`
groups, `virt-install`, `genisoimage` or `xorriso`, `openssl`, `curl`, and about 6 GB of
free disk space. The test network uses `192.168.150.0/24`, which must be free.

```bash
make vm-up     # build the base image if needed, then start the lab
make vm-test   # run the suite against the lab
make vm-down   # remove VMs, network and local state, keeping the base image
make vm-clean  # also remove the base image and the storage pool
```

The suite also loads the test pages in headless Chrome as the unprivileged user.
One of those tests records a known gap: a browser using its own DNS over HTTPS
bypasses a BLACKLIST domain rule (issue #27). It shows as skipped until fixed.

`make vm-test` refuses to run if the code changed since `make vm-up`, because the
lab would be testing old binaries. Run `make vm-up` again to redeploy.

The first `make vm-up` needs internet access to download an Ubuntu 24.04 cloud
image and install packages. Ubuntu 24.04 is the base of Linux Mint 22; it stands
in for Mint, which has no cloud image. After that, the VMs have no internet
access.

### Orchestrator configuration

The Orchestrator reads environment variables, documented in `.env.example`.
`AEGIS_GRPC_ADDR` is the address the gRPC propagation server binds to
(default `:9090`).

### Sentinel configuration

The Sentinel reads a JSON file, documented in `examples/sentinel.config.json`.
Propagation is controlled by two optional fields:

- `propagation_address`: the `host:port` of the Orchestrator's gRPC propagation
  server. Defaults to the `orchestrator_url` host with port `9090`.
- `rules_state_path`: the file where the Sentinel durably stores the last
  applied rules, written atomically. Defaults to `/var/lib/aegis/rules.json`.

Local filtering is controlled by four more optional fields:

- `filtering_enabled`: the string `"true"` or `"false"`. Defaults to `"true"`.
  When `"false"`, the Sentinel rejects every propagated policy because it cannot
  enforce it.
- `dns_listen_address`: the `host:port` the embedded DNS server binds to.
  Defaults to `127.0.0.1:53`.
- `dns_upstream_servers`: IP addresses of the upstream DNS servers the embedded
  server forwards allowed queries to. Required on the spec's image, because
  `resolv.conf` points at the Sentinel itself, so no system resolver can be
  used as the default. The first entry is also used to resolve the Orchestrator
  before the embedded server is serving.
- `nft_binary_path`: the `nft` executable used to apply the rulesets. Defaults
  to `nft`, resolved from `PATH`.
