#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/common.sh"

require_root
detect_os

if command -v redis-server >/dev/null 2>&1; then
  enable_start_any redis-server redis && exit 0
fi

ensure_prereqs

case "$PKG" in
  apt)
    if try_apt_retry redis-server; then
      :
    elif try_apt_retry redis; then
      log "installed redis meta package"
    else
      die "could not install redis from apt"
    fi
    enable_start_any redis-server redis
    ;;
  dnf|yum)
    $PKG install -y redis
    enable_start_any redis redis-server
    ;;
  *) die "unsupported package manager: $PKG" ;;
esac
log "redis installed"
