#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/common.sh"

require_root
detect_os

if command -v apache2 >/dev/null 2>&1 || command -v httpd >/dev/null 2>&1; then
  enable_start_any apache2 httpd && exit 0
fi

ensure_prereqs

case "$PKG" in
  apt)
    stop_conflicting_webservers
    try_apt_retry apache2 libapache2-mod-fcgid || try_apt_retry apache2
    a2enmod rewrite proxy proxy_http headers ssl 2>/dev/null || true
    enable_start apache2
    ;;
  dnf|yum)
    $PKG install -y httpd mod_ssl
    enable_start httpd
    ;;
  *) die "unsupported package manager: $PKG" ;;
esac
log "apache installed"
