#!/usr/bin/env bash
# Builds the Sentinel for linux/amd64 and packages it for a GitHub release.
#
# Produces in <outdir>:
#   aegis-sentinel_<version>_linux_amd64.tar.gz
#   SHA256SUMS
#
# The tarball holds the static binary, the systemd unit and an example config.
# Requires the generated gRPC code (run `make proto` first).
#
# Usage: packaging/sentinel/package.sh <version> <outdir>
set -euo pipefail

VERSION="${1:?usage: package.sh <version> <outdir>}"
OUT="$(mkdir -p "${2:?usage: package.sh <version> <outdir>}" && cd "$2" && pwd)"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
NAME="aegis-sentinel_${VERSION}_linux_amd64"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

mkdir -p "$STAGE/$NAME"

cd "$ROOT"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w" -o "$STAGE/$NAME/aegis-sentinel" ./cmd/sentinel

# Sanity check: the binary starts and prints its usage (flag -h exits 0).
"$STAGE/$NAME/aegis-sentinel" -h >/dev/null 2>&1 || {
  echo "FAIL: built binary did not start" >&2
  exit 1
}

install -m 0644 packaging/sentinel/aegis-sentinel.service "$STAGE/$NAME/"
install -m 0644 packaging/sentinel/sentinel.example.json "$STAGE/$NAME/"

tar -C "$STAGE" -czf "$OUT/$NAME.tar.gz" "$NAME"

# The installer is versioned with the release it ships in.
sed "s/@VERSION@/${VERSION}/" packaging/sentinel/install.sh > "$OUT/install.sh"
chmod 0755 "$OUT/install.sh"

cd "$OUT"
sha256sum "$NAME.tar.gz" > SHA256SUMS
echo "packaged $OUT/$NAME.tar.gz and $OUT/install.sh"
cat SHA256SUMS
