#!/usr/bin/env bash
# Fails when the deployed binaries do not match the current source tree.
source "$(dirname "$0")/lib.sh"

fresh="$(mktemp -d)"
trap 'rm -rf "$fresh"' EXIT

for binary in orchestrator sentinel; do
	(cd "$ROOT_DIR" && CGO_ENABLED=0 go build -buildvcs=false -o "$fresh/aegis-$binary" "./cmd/$binary")
	if ! cmp -s "$fresh/aegis-$binary" "$STATE_DIR/aegis-$binary"; then
		echo "vm: the lab is not running the current code (aegis-$binary differs); run \"make vm-up\" first" >&2
		exit 1
	fi
done
