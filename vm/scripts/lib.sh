#!/usr/bin/env bash
# Shared settings and helpers of the Aegis VM test suite.
set -euo pipefail

export LIBVIRT_DEFAULT_URI=qemu:///system

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
VM_DIR="$ROOT_DIR/vm"
TEMPLATES="$VM_DIR/templates"
STATE_DIR="$VM_DIR/.state"
SSH_KEY="$STATE_DIR/id_ed25519"

POOL=aegis-vm
POOL_DIR=/var/lib/libvirt/images/aegis-vm
NETWORK=aegis-net
BASE_VOL=aegis-base.qcow2
WORK_VOL=aegis-base-building.qcow2
BUILDER=aegis-builder
IMAGE_URL=https://cloud-images.ubuntu.com/releases/24.04/release/ubuntu-24.04-server-cloudimg-amd64.img

ORCHESTRATOR_IP=192.168.150.10
SENTINEL_IP=192.168.150.20
SERVICES_IP=192.168.150.30
VMS=(aegis-orchestrator aegis-sentinel aegis-services)

log() { echo "==> $*"; }

# vm_ssh runs a command on a VM as the passwordless-sudo aegis user.
vm_ssh() {
	local ip=$1
	shift
	ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
		-o LogLevel=ERROR -o ConnectTimeout=5 "aegis@$ip" "$@"
}

# vm_scp copies a local file to a path in the aegis user's home on a VM.
vm_scp() {
	scp -q -i "$SSH_KEY" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
		-o LogLevel=ERROR "$2" "aegis@$1:$3"
}

# ensure_pool creates and starts the dedicated storage pool when needed.
ensure_pool() {
	if ! virsh pool-info "$POOL" >/dev/null 2>&1; then
		virsh pool-define-as "$POOL" dir --target "$POOL_DIR" >/dev/null
		virsh pool-build "$POOL" >/dev/null
		virsh pool-autostart "$POOL" >/dev/null
	fi
	if [[ "$(virsh pool-info "$POOL" | awk '/^State:/ {print $2}')" != running ]]; then
		virsh pool-start "$POOL" >/dev/null
	fi
}

# remove_domain destroys and undefines a VM, tolerating its absence.
remove_domain() {
	virsh destroy "$1" >/dev/null 2>&1 || true
	virsh undefine "$1" --nvram >/dev/null 2>&1 || virsh undefine "$1" >/dev/null 2>&1 || true
}
