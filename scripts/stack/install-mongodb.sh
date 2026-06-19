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

case "$PKG" in
  apt)
    if try_apt mongodb; then
      enable_start mongod
      log "mongodb installed from default apt"
      exit 0
    fi
    log "default apt mongodb unavailable, trying MongoDB official repo …"
    ensure_codename
    install -d -m 0755 /usr/share/keyrings
    curl -fsSL --connect-timeout 30 --max-time 120 --retry 3 \
      https://pgp.mongodb.com/server-7.0.asc \
      | gpg --dearmor -o /usr/share/keyrings/mongodb-server-7.0.gpg
    mongo_codename="$OS_CODENAME"
    case "$OS_ID" in
      ubuntu)
        case "$OS_CODENAME" in
          noble|mantic) mongo_codename="jammy" ;;
        esac
        ;;
      debian)
        case "$OS_CODENAME" in
          bookworm|trixie) mongo_codename="bookworm" ;;
        esac
        ;;
    esac
    cat > /etc/apt/sources.list.d/mongodb-org-7.0.list <<EOF
deb [signed-by=/usr/share/keyrings/mongodb-server-7.0.gpg] https://repo.mongodb.org/apt/${OS_ID} ${mongo_codename}/mongodb-org/7.0 multiverse
EOF
    apt_update
    try_apt mongodb-org
    enable_start mongod
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
