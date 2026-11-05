#!/usr/bin/env bash
# Removes everything down.sh leaves behind: the base image and the pool.
source "$(dirname "$0")/lib.sh"

"$VM_DIR/scripts/down.sh"
virsh vol-delete --pool "$POOL" "$BASE_VOL" >/dev/null 2>&1 || true
virsh pool-destroy "$POOL" >/dev/null 2>&1 || true
virsh pool-delete "$POOL" >/dev/null 2>&1 || true
virsh pool-undefine "$POOL" >/dev/null 2>&1 || true
log "Base image and pool removed"
