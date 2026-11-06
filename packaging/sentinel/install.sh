#!/usr/bin/env bash
# Installs the Aegis Sentinel from a GitHub release.
#
# Published as a release asset. The @VERSION@ placeholder is replaced at
# packaging time so the script always installs the release it belongs to:
#
#   curl -fsSL https://github.com/dougdomingos/aegis/releases/latest/download/install.sh | sudo bash
#
# The tarball is verified against the SHA256SUMS file of the same release
# before anything is installed. Re-running the script upgrades the binary and
# unit, and keeps the existing configuration.
set -euo pipefail

VERSION="@VERSION@"
REPO="${AEGIS_REPO:-dougdomingos/aegis}"
BASE_URL="${AEGIS_RELEASE_BASE:-https://github.com/${REPO}/releases/download/v${VERSION}}"

BIN_DIR="${AEGIS_BIN_DIR:-/usr/local/bin}"
CONF_DIR="${AEGIS_CONF_DIR:-/etc/aegis}"
UNIT_DIR="${AEGIS_UNIT_DIR:-/etc/systemd/system}"

NAME="aegis-sentinel_${VERSION}_linux_amd64"
TARBALL="${NAME}.tar.gz"
PLACEHOLDER="ORCHESTRATOR_HOST"

# die prints an error to stderr and aborts.
die() {
  echo "error: $*" >&2
  exit 1
}

# require_command aborts when the named command is not on PATH.
require_command() {
  command -v "$1" >/dev/null 2>&1 || die "'$1' is required but not installed"
}

# preflight checks the platform and the tools the installer depends on.
preflight() {
  [ "$(uname -s)" = "Linux" ] || die "the Sentinel only runs on Linux"
  [ "$(uname -m)" = "x86_64" ] || die "only x86_64 (amd64) is published, found $(uname -m)"
  [ "$(id -u)" -eq 0 ] || die "run as root, e.g. with: curl ... | sudo bash"

  for cmd in curl tar sha256sum install nft; do
    require_command "$cmd"
  done
}

# download fetches the tarball and its checksum list into the work directory.
download() {
  echo "downloading ${TARBALL} (v${VERSION})..."
  curl -fsSL -o "${WORKDIR}/${TARBALL}" "${BASE_URL}/${TARBALL}"
  curl -fsSL -o "${WORKDIR}/SHA256SUMS" "${BASE_URL}/SHA256SUMS"
}

# verify checks the tarball against its entry in SHA256SUMS.
verify() {
  echo "verifying checksum..."
  local line
  line="$(grep " ${TARBALL}\$" "${WORKDIR}/SHA256SUMS" || true)"
  [ -n "$line" ] || die "${TARBALL} is not listed in SHA256SUMS"

  (cd "$WORKDIR" && printf '%s\n' "$line" | sha256sum -c --quiet -) \
    || die "checksum mismatch for ${TARBALL}; nothing was installed"
}

# install_files copies the binary, the unit and the example config into place.
install_files() {
  tar -C "$WORKDIR" -xzf "${WORKDIR}/${TARBALL}"

  install -d -m 0755 "$BIN_DIR" "$UNIT_DIR"
  install -m 0755 "${WORKDIR}/${NAME}/aegis-sentinel" "${BIN_DIR}/aegis-sentinel"
  install -m 0644 "${WORKDIR}/${NAME}/aegis-sentinel.service" "${UNIT_DIR}/aegis-sentinel.service"

  install -d -m 0755 "$CONF_DIR"
  if [ ! -e "${CONF_DIR}/sentinel.json" ]; then
    install -m 0640 "${WORKDIR}/${NAME}/sentinel.example.json" "${CONF_DIR}/sentinel.json"
    echo "created ${CONF_DIR}/sentinel.json from the example"
  else
    echo "keeping existing ${CONF_DIR}/sentinel.json"
  fi
}

# activate enables the service when the configuration has been filled in,
# and explains the remaining steps otherwise.
activate() {
  systemctl daemon-reload

  if grep -q "$PLACEHOLDER\|UPSTREAM_DNS_IP" "${CONF_DIR}/sentinel.json"; then
    cat <<EOF

Installed Aegis Sentinel v${VERSION}. The service is not started yet.
Finish the setup:
  1. Edit ${CONF_DIR}/sentinel.json (replace the placeholder hosts).
  2. Place the shared lab key at the lab_key_path in that file.
  3. Start the service: sudo systemctl enable --now aegis-sentinel
EOF
    return
  fi

  systemctl enable aegis-sentinel >/dev/null
  if systemctl is-active --quiet aegis-sentinel; then
    systemctl restart aegis-sentinel
    echo "restarted aegis-sentinel with v${VERSION}"
  else
    systemctl start aegis-sentinel
    echo "started aegis-sentinel v${VERSION}"
  fi
}

# main runs the installation steps in order.
main() {
  preflight

  WORKDIR="$(mktemp -d)"
  trap 'rm -rf "$WORKDIR"' EXIT

  download
  verify
  install_files
  activate

  echo "done: aegis-sentinel v${VERSION} installed to ${BIN_DIR}/aegis-sentinel"
}

main "$@"
