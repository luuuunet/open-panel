#!/usr/bin/env bash
# Install Nginx via apt, falling back to nginx.org official repo on Debian/Ubuntu.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/common.sh"

require_root
detect_os

if command -v nginx >/dev/null 2>&1; then
  log "nginx already installed"
  enable_start nginx
  exit 0
fi

ensure_prereqs

case "$PKG" in
  apt)
    stop_conflicting_webservers
    if try_apt_retry nginx; then
      enable_start nginx
      log "nginx installed from default apt"
      exit 0
    fi
    log "default apt nginx failed, trying nginx.org repo …"
    ensure_codename
    rm -f /etc/apt/sources.list.d/nginx-owpanel.list
    gpg_dearmor_url "https://nginx.org/keys/nginx_signing.key" /usr/share/keyrings/nginx-archive-keyring.gpg
    distro="$OS_ID"
    [[ "$distro" == "debian" || "$distro" == "ubuntu" ]] || distro="debian"
    write_apt_repo /etc/apt/sources.list.d/nginx-owpanel.list \
      "deb [signed-by=/usr/share/keyrings/nginx-archive-keyring.gpg] https://nginx.org/packages/${distro}/ ${OS_CODENAME} nginx"
    apt_update
    apt_install_retry nginx
    enable_start nginx
    log "nginx installed from nginx.org repo"
    ;;
  dnf|yum)
    $PKG install -y nginx
    enable_start nginx
    log "nginx installed from $PKG"
    ;;
  *)
    die "unsupported package manager: $PKG"
    ;;
esac
