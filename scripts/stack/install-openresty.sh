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
stop_conflicting_webservers

case "$PKG" in
  apt)
    if try_apt_retry openresty; then
      enable_start openresty
      log "openresty installed from default apt"
      exit 0
    fi
    log "default apt openresty unavailable, trying OpenResty official repo …"
    ensure_codename
    rm -f /etc/apt/sources.list.d/openresty-owpanel.list /etc/apt/sources.list.d/openresty.list
    gpg_dearmor_url "https://openresty.org/package/pubkey.gpg" /usr/share/keyrings/openresty.gpg
    distro="$OS_ID"
    [[ "$distro" == "debian" || "$distro" == "ubuntu" ]] || distro="ubuntu"
    write_apt_repo /etc/apt/sources.list.d/openresty-owpanel.list \
      "deb [arch=$(apt_arch) signed-by=/usr/share/keyrings/openresty.gpg] https://openresty.org/package/${distro} ${OS_CODENAME} main"
    apt_update
    apt_install_retry openresty
    enable_start openresty
    log "openresty installed from official repo (${OS_CODENAME})"
    ;;
  dnf|yum)
    $PKG install -y openresty 2>/dev/null || $PKG install -y nginx
    enable_start_any openresty nginx
    ;;
  *) die "unsupported package manager: $PKG" ;;
esac
log "openresty installed"
