#!/usr/bin/env bash
# Removes VMs, network, overlays and local state. Keeps the cached base image.
source "$(dirname "$0")/lib.sh"

for vm in "${VMS[@]}" "$BUILDER"; do
	remove_domain "$vm"
	virsh vol-delete --pool "$POOL" "$vm.qcow2" >/dev/null 2>&1 || true
done

virsh vol-delete --pool "$POOL" "$WORK_VOL" >/dev/null 2>&1 || true
virsh net-destroy "$NETWORK" >/dev/null 2>&1 || true
virsh net-undefine "$NETWORK" >/dev/null 2>&1 || true
rm -rf "$STATE_DIR"
log "Lab removed"
