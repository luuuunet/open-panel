#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/common.sh"

require_root
detect_os

if systemctl is-active --quiet mongod 2>/dev/null; then
  log "mongodb already running"
  exit 0
fi

ensure_prereqs

install_mongodb_org() {
  local ver="${1:-7.0}"
  rm -f "/etc/apt/sources.list.d/mongodb-org-${ver}.list"
  setup_mongodb_repo "$ver"
  try_apt_retry mongodb-org
}

case "$PKG" in
  apt)
    log "installing MongoDB from official repository …"
    if install_mongodb_org 7.0; then
      enable_start mongod
      log "mongodb 7.0 installed from official repo"
      exit 0
    fi
    log "mongodb-org 7.0 unavailable, trying 6.0 …"
    rm -f /etc/apt/sources.list.d/mongodb-org-7.0.list
    if install_mongodb_org 6.0; then
      enable_start mongod
      log "mongodb 6.0 installed from official repo"
      exit 0
    fi
    die "mongodb install failed (official repo)"
    ;;
  dnf|yum)
    cat > /etc/yum.repos.d/mongodb-org-7.0.repo <<'EOF'
[mongodb-org-7.0]
name=MongoDB Repository
baseurl=https://repo.mongodb.org/yum/redhat/$releasever/mongodb-org/7.0/x86_64/
gpgcheck=1
enabled=1
gpgkey=https://pgp.mongodb.com/server-7.0.asc
EOF
    $PKG install -y mongodb-org
    enable_start mongod
    ;;
  *) die "unsupported package manager: $PKG" ;;
esac
log "mongodb installed"
