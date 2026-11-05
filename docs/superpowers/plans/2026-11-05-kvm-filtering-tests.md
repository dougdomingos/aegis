# KVM Filtering Test Suite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prove in real KVM VMs that Aegis blocks and allows real traffic according to policy.

**Architecture:** Bash scripts manage three VMs on an isolated libvirt network. Go tests behind the `vm` build tag configure policies through the Orchestrator API and run curl over SSH as an unprivileged user on the Sentinel VM.

**Tech Stack:** libvirt (`virsh`, `virt-install`), Ubuntu 24.04 cloud image, cloud-init, Go `testing`, system `ssh` client.

**Spec:** `docs/superpowers/specs/2026-11-05-kvm-filtering-tests-design.md`

## Global Constraints
- Libvirt URI `qemu:///system`; every resource name starts with `aegis-`; pool `aegis-vm`.
- Network 192.168.150.0/24, host .1, Orchestrator .10, Sentinel .20, services .30/.31/.32.
- Binaries built with `CGO_ENABLED=0` after `make proto`.
- Test names follow `Test<Type>_<Method>_<Scenario>_<ExpectedResult>`; every symbol has a doc comment; helpers go at the end of the file; plain `t.Errorf`/`t.Fatalf` (AGENTS.md).
- No Go dependencies added. `TEST_PACKAGES` unchanged.
- **Do not commit:** AGENTS.md forbids automatic commits, so changes stay in the working tree for developer review. This overrides the skill's commit steps.

## Review Focus
- `vm-up` run twice: the second run rebuilds cleanly or reuses state, never fails halfway. Task 1.
- `vm-down` with nothing provisioned, or after a failed `vm-up`: exits 0 and removes what exists. Task 1.
- `vm-test` without `vm-up`: fails immediately with a message naming `make vm-up`, not a timeout. Task 3.
- Scenario failing midway: the next scenario still starts with no policy, because cleanup runs from `t.Cleanup`. Task 3.
- Reboot while the Orchestrator is reachable but the Sentinel starts first: the unit restarts until authenticated (`Restart=on-failure`). Task 2.

---

### Task 1: VM lifecycle

**Files:**
- Create: `e2e/vm/scripts/lib.sh`, `build-base.sh`, `up.sh`, `down.sh`, `clean.sh`
- Create: `e2e/vm/templates/network.xml`, `e2e/vm/templates/cloud-init/{base,node}.yaml`
- Modify: `Makefile`, `.gitignore` (add `e2e/vm/.state/`)

**Interfaces:**
- Produces: `make vm-up`, `vm-down`, `vm-clean`; SSH access as `aegis` (passwordless sudo) with the key `e2e/vm/.state/id_ed25519` on all VMs.

- [ ] **Step 1:** Write `build-base.sh`: download the Ubuntu 24.04 cloud image, boot a builder on QEMU user networking with `base.yaml` (installs nftables, curl, dnsmasq, python3, dnsutils), wait for shutdown, keep the volume `aegis-base.qcow2`. Skip if it already exists.
- [ ] **Step 2:** Write `up.sh`: run `down.sh` first so reruns are clean, generate the SSH key, define the isolated network (no `<forward>`), create overlays and start the three VMs with static IPs through cloud-init `network-config`, wait for SSH.
- [ ] **Step 3:** Write `down.sh` (tolerates missing resources) and `clean.sh`; add Makefile targets with `##` help text.
- [ ] **Step 4:** Verify. Run `make vm-up && make vm-up`. Expected: both succeed; from the Sentinel, `ping -c1 192.168.150.10` succeeds and `ping -c1 -W2 1.1.1.1` fails.
- [ ] **Step 5:** Verify. Run `make vm-down && make vm-down`. Expected: both exit 0; `virsh -c qemu:///system list --all` and `net-list --all` show no `aegis-` entries; `vol-list aegis-vm` shows only the base image.

### Task 2: Deploy services

**Files:**
- Create: `e2e/vm/scripts/deploy.sh`, `e2e/vm/templates/aegis-orchestrator.service`, `aegis-sentinel.service`, `aegis-http@.service`, `dnsmasq.conf`, `sentinel.config.json`, `machines.csv`
- Modify: `e2e/vm/scripts/up.sh` (call `deploy.sh` last)

**Interfaces:**
- Consumes: Task 1 SSH access.
- Produces: Orchestrator API at `http://192.168.150.10:8080`; Sentinel running; `student` user on the Sentinel; targets on .31 and .32.

- [ ] **Step 1:** Write `deploy.sh`: build both binaries, generate lab key and session secret into `e2e/vm/.state/`, copy binaries and configs by scp, enable the units. Orchestrator env: `AEGIS_LAB_KEY`, `AEGIS_SESSION_SECRET`, `AEGIS_MACHINES`, `AEGIS_DB`, `AEGIS_GRPC_ADDR=:9090`.
- [ ] **Step 2:** Write the Sentinel config: `dns_upstream_servers: ["192.168.150.30"]`, `propagation_address: "192.168.150.10:9090"`, and a heartbeat of `5s`. Set `/etc/resolv.conf` to `nameserver 127.0.0.1`. The unit uses `Restart=on-failure`, `RestartSec=2`, `After=network-online.target`.
- [ ] **Step 3:** Verify. Run `make vm-up`, then `curl -s http://192.168.150.10:8080/machines`. Expected: the Sentinel is listed as CONNECTED.
- [ ] **Step 4:** Verify. As `student` on the Sentinel, run `curl -s --max-time 5 allowed.test` and `curl -s --max-time 5 blocked.test`. Expected: both exit 0.

### Task 3: Test harness

**Files:**
- Create: `e2e/vm/harness_test.go` (package `vm`, `//go:build vm`)

**Interfaces:**
- Consumes: `e2e/vm/.state/id_ed25519`; API routes as used by `e2e/containers/harness_test.go`.
- Produces:
  - `newLab(t *testing.T) *lab`
  - `(*lab) curlAsStudent(t, target string) int`: returns the curl exit code
  - `(*lab) sudo(t, command string) (int, string)`
  - `(*lab) applyPolicy(t, policyType, ruleType, value string) int64`: creates the policy and group, links them, waits for ACK, registers cleanup, returns the ACKed sequence
  - `(*lab) revoke(t)`, `(*lab) reboot(t)`, `(*lab) waitConnected(t)`

- [ ] **Step 1:** Write `TestMain` so it exits non-zero with `run "make vm-up" first` when `/machines` is unreachable within 3s. Verify: with no VMs, `go test -tags vm ./e2e/vm/...` prints that message immediately.
- [ ] **Step 2:** Implement the API helpers, porting `doJSON`, `createPolicy`, `createGroup`, `linkMachine`, `linkPolicy`, `unlinkPolicy`, `waitForPropagation` and `machineByIP` from `e2e/containers/harness_test.go` and `createIPRule` from `e2e/containers/filtering_e2e_test.go`. They live in another package's test files and cannot be imported.
- [ ] **Step 3:** Implement the SSH helpers with `os/exec` and `ssh -i ... -o StrictHostKeyChecking=no`. `curlAsStudent` runs `sudo -u student curl -s -o /dev/null --max-time 5 <target>`. `reboot` uses `virsh reboot aegis-sentinel`, then waits for SSH and `waitConnected`.
- [ ] **Step 4:** Implement `t.Cleanup` in `applyPolicy`: unlink if still linked, delete the group and policy, wait until `inet aegis` is absent.

### Task 4: Filtering matrix

**Files:**
- Create: `e2e/vm/filtering_vm_test.go` (`//go:build vm`)
- Modify: `Makefile` (`vm-test`: `go test -tags vm -count=1 -timeout 30m -v ./e2e/vm/...`), `README.md`

**Interfaces:**
- Consumes: Task 3 helpers.

- [ ] **Step 1:** Write `TestFiltering_Policy_WithEachTypeAndRule_EnforcesTraffic` as a table of the spec's four rows: `map[string]struct{ policyType, ruleType, value string; reachable []string; blocked map[string]int }`, with subtests run serially.
- [ ] **Step 2:** Each subtest runs in order: baseline (all targets exit 0, `t.Fatalf` otherwise), `applyPolicy`, assert the row, `reboot`, assert the row, `revoke`, assert `nft list table inet aegis` fails and all targets exit 0. Errors print the target, wanted code and actual code.
- [ ] **Step 3:** Verify. Run `make vm-up && make vm-test`. Expected: PASS for all four subtests.
- [ ] **Step 4:** Negative control: `TestFiltering_Assertions_WithFilterRemoved_DetectBrokenFiltering` deletes `inet aegis` under an applied policy and expects the blocked target to become reachable. (`filtering_enabled: "false"` rejects the policy, so it would fail before reaching any traffic check.)
- [ ] **Step 5:** Add the README section: prerequisites (`libvirtd`, `kvm` and `libvirt` groups, `virt-install`, `genisoimage`, disk space), the targets, and the base image cache exception.
- [ ] **Step 6:** Verify. Run `make vm-down`, then `make tests` and `go build ./...`. Expected: no `aegis-` VMs or network remain; tests and build pass.
