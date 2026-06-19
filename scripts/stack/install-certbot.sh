#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/common.sh"

require_root
detect_os

if command -v certbot >/dev/null 2>&1; then
  log "certbot already installed"
  exit 0
fi

ensure_prereqs

case "$PKG" in
  apt)
    if try_apt_retry certbot python3-certbot-nginx; then
      log "certbot installed from apt"
      exit 0
    fi
    if command -v snap >/dev/null 2>&1; then
      log "trying certbot via snap …"
      snap install core 2>/dev/null || true
      snap refresh core 2>/dev/null || true
      snap install --classic certbot
      ln -sf /snap/bin/certbot /usr/bin/certbot 2>/dev/null || true
      log "certbot installed via snap"
      exit 0
    fi
    try_apt_retry python3-pip
    pip3 install --break-system-packages certbot certbot-nginx 2>/dev/null \
      || pip3 install certbot certbot-nginx
    ;;
  dnf|yum)
    $PKG install -y certbot python3-certbot-nginx 2>/dev/null \
      || $PKG install -y certbot
    ;;
  *) die "unsupported package manager: $PKG" ;;
esac
log "certbot installed"
