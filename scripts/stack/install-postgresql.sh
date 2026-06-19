#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/common.sh"

require_root
detect_os

if systemctl is-active --quiet postgresql 2>/dev/null; then
  log "postgresql already running"
  exit 0
fi

ensure_prereqs

case "$PKG" in
  apt)
    if try_apt postgresql postgresql-contrib; then
      enable_start postgresql
      log "postgresql installed from default apt"
      exit 0
    fi
    log "default apt postgresql failed, trying PGDG repo …"
    ensure_codename
    install -d -m 0755 /usr/share/keyrings
    curl -fsSL --connect-timeout 30 --max-time 120 --retry 3 \
      https://www.postgresql.org/media/keys/ACCC4CF8.asc \
      | gpg --dearmor -o /usr/share/keyrings/postgresql.gpg
    cat > /etc/apt/sources.list.d/pgdg-owpanel.list <<EOF
deb [signed-by=/usr/share/keyrings/postgresql.gpg] https://apt.postgresql.org/pub/repos/apt ${OS_CODENAME}-pgdg main
EOF
    apt_update
    for ver in 16 15 14; do
      if try_apt "postgresql-${ver}" "postgresql-client-${ver}"; then
        enable_start postgresql
        log "postgresql ${ver} installed from PGDG"
        exit 0
      fi
    done
    try_apt postgresql postgresql-contrib
    enable_start postgresql
    ;;
  dnf|yum)
    $PKG install -y postgresql-server postgresql
    if command -v postgresql-setup >/dev/null 2>&1; then
      postgresql-setup --initdb || true
    fi
    enable_start postgresql
    ;;
  *) die "unsupported package manager: $PKG" ;;
esac
log "postgresql installed"
