#!/usr/bin/env bash
# Builds the cached base image: the only step that needs internet access.
source "$(dirname "$0")/lib.sh"

ensure_pool
virsh pool-refresh "$POOL" >/dev/null

# The base volume only exists once a build finished, so its presence is proof.
if virsh vol-info --pool "$POOL" "$BASE_VOL" >/dev/null 2>&1; then
	log "Base image already cached"
	exit 0
fi

# A leftover builder or work volume means an earlier build was interrupted.
remove_domain "$BUILDER"
virsh vol-delete --pool "$POOL" "$WORK_VOL" >/dev/null 2>&1 || true

mkdir -p "$STATE_DIR"
image="$STATE_DIR/cloud-image.img"
log "Downloading the Ubuntu 24.04 cloud image"
curl -fL --retry 3 -o "$image" "$IMAGE_URL"

virsh vol-create-as "$POOL" "$WORK_VOL" "$(stat -c %s "$image")" --format qcow2 >/dev/null
virsh vol-upload --pool "$POOL" "$WORK_VOL" "$image"
rm -f "$image"
virsh pool-refresh "$POOL" >/dev/null
virsh vol-resize --pool "$POOL" "$WORK_VOL" 8G >/dev/null

log "Installing packages in the builder VM"
virt-install --name "$BUILDER" --memory 2048 --vcpus 2 --osinfo ubuntu24.04 --import \
	--disk "vol=$POOL/$WORK_VOL,bus=virtio" --network user,model=virtio \
	--graphics none --noautoconsole --noreboot \
	--cloud-init "user-data=$TEMPLATES/cloud-init/base.yaml" >/dev/null

for _ in $(seq 1 180); do
	[[ "$(virsh domstate "$BUILDER")" == "shut off" ]] && break
	sleep 5
done
if [[ "$(virsh domstate "$BUILDER")" != "shut off" ]]; then
	echo "builder VM did not power off within 15 minutes: package installation probably failed" >&2
	exit 1
fi

remove_domain "$BUILDER"
virsh vol-clone --pool "$POOL" "$WORK_VOL" "$BASE_VOL" >/dev/null
virsh vol-delete --pool "$POOL" "$WORK_VOL" >/dev/null
log "Base image ready"
