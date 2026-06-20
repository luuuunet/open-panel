#!/usr/bin/env bash
# Build release packages for Linux (amd64/arm64) and Windows (amd64)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/dist"
VERSION="${VERSION:-$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)}"
GIT_COMMIT="$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-s -w -X github.com/luuuunet/owpanel/internal/version.Version=${VERSION} -X github.com/luuuunet/owpanel/internal/version.BuildDate=${BUILD_DATE} -X github.com/luuuunet/owpanel/internal/version.GitCommit=${GIT_COMMIT}"

log() { echo "[build] $*"; }

build_one() {
  local goos="$1" goarch="$2" ext="$3" name="$4"
  local dir="$OUT/$name"
  mkdir -p "$dir"
  log "Building $goos/$goarch -> $dir (version=$VERSION)"
  (cd "$ROOT/backend" && GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    go build -ldflags="$LDFLAGS" -o "$dir/owpanel$ext" ./cmd/server)
  (cd "$ROOT/backend" && GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    go build -ldflags="$LDFLAGS" -o "$dir/op$ext" ./cmd/op)
  rm -rf "$dir/web"
  cp -a "$ROOT/backend/web" "$dir/web"
  mkdir -p "$dir/data"
  cat > "$dir/README.txt" <<EOF
OWPanel $VERSION ($goos/$goarch)
1. Set OWPANEL_DATA to ./data (or use install script)
2. Run ./owpanel$ext  or use scripts/install.sh / install.ps1
Default: http://HOST:8888
First login: admin / random password in data/INITIAL_CREDENTIALS.txt (or server log)
EOF
  (cd "$OUT" && tar -czf "${name}.tar.gz" "$name")
  log "Package: $OUT/${name}.tar.gz"
}

log "Building frontend..."
(cd "$ROOT/frontend" && npm ci && npm run build)

build_one linux amd64 "" "owpanel-linux-amd64"
build_one linux arm64 "" "owpanel-linux-arm64"
build_one windows amd64 ".exe" "owpanel-windows-amd64"

log "Done. Artifacts in $OUT"
