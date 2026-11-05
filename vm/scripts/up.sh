#!/usr/bin/env bash
# Provisions the whole topology from scratch on the isolated network.
source "$(dirname "$0")/lib.sh"

"$VM_DIR/scripts/down.sh"
"$VM_DIR/scripts/build-base.sh"

mkdir -p "$STATE_DIR"
ssh-keygen -q -t ed25519 -N '' -f "$SSH_KEY"
public_key="$(cat "$SSH_KEY.pub")"

virsh net-define "$TEMPLATES/network.xml" >/dev/null
virsh net-start "$NETWORK" >/dev/null

# create_vm boots one overlay VM. Extra addresses follow the primary one.
create_vm() {
	local name=$1 nameserver=$2
	shift 2
	local dir="$STATE_DIR/vms/$name" addresses="" ip
	mkdir -p "$dir"
	for ip in "$@"; do addresses+="\"$ip/24\", "; done

	sed "s|@SSH_KEY@|$public_key|" "$TEMPLATES/cloud-init/node.yaml" >"$dir/user-data"
	printf 'instance-id: %s-%s\nlocal-hostname: %s\n' "$name" "$(date +%s)" "$name" >"$dir/meta-data"
	cat >"$dir/network-config" <<NET
version: 2
ethernets:
  lab0:
    match: {name: "en*"}
    addresses: [${addresses%, }]
    nameservers: {addresses: [$nameserver]}
NET

	virsh vol-create-as "$POOL" "$name.qcow2" 8G --format qcow2 \
		--backing-vol "$BASE_VOL" --backing-vol-format qcow2 >/dev/null
	virt-install --name "$name" --memory 1024 --vcpus 1 --osinfo ubuntu24.04 --import \
		--disk "vol=$POOL/$name.qcow2,bus=virtio" --network "network=$NETWORK,model=virtio" \
		--graphics none --noautoconsole \
		--cloud-init "user-data=$dir/user-data,meta-data=$dir/meta-data,network-config=$dir/network-config" >/dev/null
}

create_vm aegis-orchestrator "$SERVICES_IP" "$ORCHESTRATOR_IP"
create_vm aegis-services "$SERVICES_IP" "$SERVICES_IP" 192.168.150.31 192.168.150.32
create_vm aegis-sentinel 127.0.0.1 "$SENTINEL_IP"

for ip in "$ORCHESTRATOR_IP" "$SERVICES_IP" "$SENTINEL_IP"; do
	log "Waiting for $ip"
	for attempt in $(seq 1 60); do
		vm_ssh "$ip" 'cloud-init status --wait >/dev/null 2>&1; true' 2>/dev/null && break
		if [[ $attempt == 60 ]]; then
			echo "$ip did not become reachable over SSH" >&2
			exit 1
		fi
		sleep 5
	done
done

"$VM_DIR/scripts/deploy.sh"

# virt-install's first boot runs with on_reboot=destroy, so a later reboot would
# power the VM off. A full stop and start applies the persistent definition.
log "Restarting the VMs on their final definition"
for vm in "${VMS[@]}"; do virsh shutdown "$vm" >/dev/null; done
for vm in "${VMS[@]}"; do
	for attempt in $(seq 1 60); do
		[[ "$(virsh domstate "$vm")" == "shut off" ]] && break
		if [[ $attempt == 60 ]]; then
			echo "$vm did not shut down" >&2
			exit 1
		fi
		sleep 2
	done
	virsh start "$vm" >/dev/null
done
for ip in "$ORCHESTRATOR_IP" "$SERVICES_IP" "$SENTINEL_IP"; do
	for attempt in $(seq 1 60); do
		vm_ssh "$ip" true 2>/dev/null && break
		if [[ $attempt == 60 ]]; then
			echo "$ip did not come back after the restart" >&2
			exit 1
		fi
		sleep 2
	done
done
log "Lab ready. Run: make vm-test"
