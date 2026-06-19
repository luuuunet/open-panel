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
    if try_apt_retry mongodb; then
      enable_start mongod
      log "mongodb installed from default apt"
      exit 0
    fi
    log "default apt mongodb unavailable, trying MongoDB official repo …"
    rm -f /etc/apt/sources.list.d/mongodb-org-7.0.list
    setup_mongodb_repo 7.0
    if ! try_apt_retry mongodb-org; then
      if [[ "$OS_ID" == "ubuntu" && "$OS_CODENAME" != "jammy" ]]; then
        log "retrying MongoDB repo with jammy suite …"
        gpg_dearmor_url "https://pgp.mongodb.com/server-7.0.asc" /usr/share/keyrings/mongodb-server-7.0.gpg
        write_apt_repo /etc/apt/sources.list.d/mongodb-org-7.0.list \
          "deb [arch=$(apt_arch) signed-by=/usr/share/keyrings/mongodb-server-7.0.gpg] https://repo.mongodb.org/apt/ubuntu jammy/mongodb-org/7.0 multiverse"
        apt_update
        try_apt_retry mongodb-org || die "mongodb install failed"
      else
        die "mongodb install failed"
      fi
    fi
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
