# KVM test suite for local filtering (Issue #25): design

## Intent
The Testcontainers e2e suite checks that nftables rules and DNS answers exist, but never that a real connection is blocked or allowed. This suite runs the real binaries in KVM VMs and proves that traffic follows the configured policy.

## Goals
- Prove blocking and allowing for every combination: WHITELIST and BLACKLIST, each with DOMAIN and IP rules.
- Prove REVOKE restores traffic.
- Prove a rebooted Sentinel re-authenticates and reapplies its rules with no manual step.
- Provide `make vm-up`, `vm-test` and `vm-down`, with README instructions.

## Non-goals
- CI: the suite runs locally and manually, as CI would need a cloud cluster (user decision).
- IPv6 and DHCP: the lab uses fixed IPs and IPv6 is not a concern (user decision).
- Testing conntrack or boot ordering on their own. Allowed `curl` requests and the reboot test exercise them indirectly.

## Anti-goals
- A suite that passes because the network is broken. Blocked results only count if the target was reachable beforehand.
- A suite that could not fail. A permanent test removes the nftables table under an applied policy and shows that traffic then passes.

## Topology (my call: deviates from the issue)
Three VMs on an isolated libvirt network with static IPv4 addresses:

| VM | IP | Role |
|---|---|---|
| `aegis-orchestrator` | 192.168.150.10 | Orchestrator: HTTP 8080, gRPC 9090 |
| `aegis-sentinel` | 192.168.150.20 | Sentinel under systemd, `resolv.conf` -> 127.0.0.1, unprivileged `student` user |
| `aegis-services` | 192.168.150.30 (DNS), .31 (IP A), .32 (IP B) | dnsmasq: `allowed.test` -> .31, `blocked.test` -> .32; HTTP on .31 and .32 |

The issue proposes a separate client VM. The Sentinel filters only its own outgoing traffic (`hook output`), so a separate client would not be filtered. The `student` user on the Sentinel VM takes that role, matching a student at a lab machine.

The host reaches VMs at 192.168.150.1 for SSH and API calls. Inbound SSH keeps working under WHITELIST because the chain accepts established traffic.

## Images (my call)
- Ubuntu 24.04 cloud image, the base of Linux Mint 22 (user decision; the lab runs Mint, which has no cloud image).
- A builder VM on QEMU user networking (it does not depend on the host's forwarding firewall, which blocked libvirt's NAT on a Docker host) installs nftables, curl, dnsmasq and python3, then shuts down, producing a cached base image. Every other VM is an overlay of it on the isolated network, without internet.
- Everything lives in a libvirt storage pool `aegis-vm` on `qemu:///system`.
- `vm-down` removes VMs, network and overlays, leaving only the base image cache. `vm-clean` also removes the cache and pool. Criterion 5 of the issue ("no leftovers") is met this way, with the cache as a stated exception.

## Test matrix
Each scenario runs curl over SSH as `student` with `--max-time 5`. Expected curl exit codes: 0 reachable, 6 DNS refused, 28 dropped.

| Scenario | Rule | Exit 0 | Blocked |
|---|---|---|---|
| WHITELIST + DOMAIN | `allowed.test` | `allowed.test` | `blocked.test` (6), `http://192.168.150.32` (28) |
| WHITELIST + IP | `192.168.150.31` | `http://192.168.150.31` | `http://192.168.150.32` (28) |
| BLACKLIST + DOMAIN | `blocked.test` | `allowed.test` | `blocked.test` (6) |
| BLACKLIST + IP | `192.168.150.32` | `http://192.168.150.31` | `http://192.168.150.32` (28) |

For each scenario, in order:
1. **Baseline:** all targets reachable with no policy.
2. **Enforce:** link the policy, wait for ACK, check the row above.
3. **Reboot:** reboot through libvirt; the Machine returns to CONNECTED and the row holds again.
4. **Revoke:** unlink, wait for ACK; `inet aegis` is gone and all targets are reachable.

Scenarios run serially and clean up their policy and group.

Known limit: a BLACKLIST DOMAIN rule blocks by name only, so direct access by IP still works. The suite does not assert this as a requirement.

Design choice: a rebooted Sentinel enforces no rules until it authenticates and receives its policy. Exam machines are ephemeral, so no rules persist across boots. The suite's reboots therefore keep the Orchestrator reachable and do not test an unreachable one.

## Left to the builder
Script internals, cloud-init details, the HTTP server (`python3 -m http.server` is enough), and polling intervals.
