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
    if try_apt_retry postgresql postgresql-contrib; then
      enable_start postgresql
      log "postgresql installed from default apt"
      exit 0
    fi
    log "default apt postgresql failed, trying PGDG repo …"
    ensure_codename
    rm -f /etc/apt/sources.list.d/pgdg-owpanel.list
    gpg_dearmor_url "https://www.postgresql.org/media/keys/ACCC4CF8.asc" /usr/share/keyrings/postgresql.gpg
    write_apt_repo /etc/apt/sources.list.d/pgdg-owpanel.list \
      "deb [arch=$(apt_arch) signed-by=/usr/share/keyrings/postgresql.gpg] https://apt.postgresql.org/pub/repos/apt ${OS_CODENAME}-pgdg main"
    apt_update
    for ver in 16 15 14; do
      if try_apt_retry "postgresql-${ver}" "postgresql-client-${ver}"; then
        enable_start postgresql
        log "postgresql ${ver} installed from PGDG"
        exit 0
      fi
    done
    apt_install_retry postgresql postgresql-contrib
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
