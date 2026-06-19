#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/common.sh"

require_root
detect_os

if command -v openresty >/dev/null 2>&1; then
  enable_start openresty 2>/dev/null || enable_start nginx
  exit 0
fi

ensure_prereqs

case "$PKG" in
  apt)
    if try_apt openresty; then
      enable_start openresty
      log "openresty installed from default apt"
      exit 0
    fi
    log "trying OpenResty official repo …"
    ensure_codename
    install -d -m 0755 /usr/share/keyrings
    curl -fsSL --connect-timeout 30 --max-time 120 --retry 3 \
      https://openresty.org/package/pubkey.gpg \
      | gpg --dearmor -o /usr/share/keyrings/openresty.gpg
    distro="$OS_ID"
    [[ "$distro" == "debian" || "$distro" == "ubuntu" ]] || distro="debian"
    cat > /etc/apt/sources.list.d/openresty-owpanel.list <<EOF
deb [signed-by=/usr/share/keyrings/openresty.gpg] http://openresty.org/package/${distro} ${OS_CODENAME} openresty
EOF
    apt_update
    try_apt openresty
    enable_start openresty
    ;;
  dnf|yum)
    $PKG install -y openresty 2>/dev/null || $PKG install -y nginx
    enable_start_any openresty nginx
    ;;
  *) die "unsupported package manager: $PKG" ;;
esac
log "openresty installed"
