#!/usr/bin/env bash
# Removes everything down.sh leaves behind: the base image and the pool.
source "$(dirname "$0")/lib.sh"

"$VM_DIR/scripts/down.sh"
for vol in $(virsh vol-list "$POOL" 2>/dev/null | awk '$1 ~ /^aegis-base/ {print $1}'); do
	virsh vol-delete --pool "$POOL" "$vol" >/dev/null 2>&1 || true
done
virsh pool-destroy "$POOL" >/dev/null 2>&1 || true
virsh pool-delete "$POOL" >/dev/null 2>&1 || true
virsh pool-undefine "$POOL" >/dev/null 2>&1 || true
log "Base image and pool removed"
