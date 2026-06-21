#!/usr/bin/env bash
# OWPanel stack fallback entrypoint — used by appstore when apt install fails.
# Usage: fallback.sh nginx|redis|postgresql|docker|php83|…
set -euo pipefail

# Fix broken third-party apt lists before any install (e.g. MongoDB noble repo blocks all apt updates).
owpanel_apt_emergency_sanitize() {
  if ! command -v apt-get >/dev/null 2>&1; then
    return 0
  fi
  export DEBIAN_FRONTEND=noninteractive
  shopt -s nullglob
  local f
  for f in /etc/apt/sources.list.d/*.list; do
    if grep -q 'repo.mongodb.org' "$f" 2>/dev/null; then
      if grep -qE '/ubuntu (noble|mantic)/' "$f" 2>/dev/null; then
        sed -i 's|/ubuntu noble/mongodb-org/|/ubuntu jammy/mongodb-org/|g;s|/ubuntu mantic/mongodb-org/|/ubuntu jammy/mongodb-org/|g' "$f" 2>/dev/null || rm -f "$f"
      fi
    fi
  done
  shopt -u nullglob
  if apt-get update -qq 2>/tmp/owpanel-apt-pre.err; then
    return 0
  fi
  if grep -qE 'mongodb.org|Release file' /tmp/owpanel-apt-pre.err 2>/dev/null; then
    rm -f /etc/apt/sources.list.d/mongodb-org-*.list
    apt-get update -qq || true
  fi
}
owpanel_apt_emergency_sanitize

COMPONENT="${1:-}"
[[ -n "$COMPONENT" ]] || { echo "usage: $0 <component>" >&2; exit 1; }

STACK_FILES=(
  common.sh
  fallback.sh
  install-nginx.sh
  install-mariadb.sh
  install-php.sh
  install-redis.sh
  install-postgresql.sh
  install-mongodb.sh
  install-apache.sh
  install-openresty.sh
  install-docker.sh
  install-certbot.sh
  install-generic.sh
)

if [[ -f "${BASH_SOURCE[0]:-}" ]]; then
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
else
  SCRIPT_DIR="/tmp/owpanel-stack-$$"
  mkdir -p "$SCRIPT_DIR"
  BASE="${OWPANEL_STACK_BASE:-https://raw.githubusercontent.com/luuuunet/owpanel/main/scripts/stack}"
  for f in "${STACK_FILES[@]}"; do
    [[ "$f" == "fallback.sh" ]] && continue
    curl -fsSL --connect-timeout 30 --max-time 120 --retry 3 \
      "${BASE}/${f}" -o "${SCRIPT_DIR}/${f}"
    chmod +x "${SCRIPT_DIR}/${f}"
  done
fi

case "$COMPONENT" in
  nginx) exec bash "$SCRIPT_DIR/install-nginx.sh" ;;
  mariadb|mysql) exec bash "$SCRIPT_DIR/install-mariadb.sh" ;;
  php*)
    ver="${COMPONENT#php}"
    if [[ ${#ver} -ge 2 ]]; then
      export PHP_VERSION="${ver:0:1}.${ver:1}"
    fi
    exec bash "$SCRIPT_DIR/install-php.sh"
    ;;
  redis) exec bash "$SCRIPT_DIR/install-redis.sh" ;;
  postgresql) exec bash "$SCRIPT_DIR/install-postgresql.sh" ;;
  mongodb) exec bash "$SCRIPT_DIR/install-mongodb.sh" ;;
  apache) exec bash "$SCRIPT_DIR/install-apache.sh" ;;
  openresty) exec bash "$SCRIPT_DIR/install-openresty.sh" ;;
  docker) exec bash "$SCRIPT_DIR/install-docker.sh" ;;
  certbot) exec bash "$SCRIPT_DIR/install-certbot.sh" ;;
  memcached|fail2ban|supervisor|pureftpd|postfix|dovecot)
    export GENERIC_KEY="$COMPONENT"
    exec bash "$SCRIPT_DIR/install-generic.sh"
    ;;
  *)
    echo "unknown component: $COMPONENT" >&2
    exit 1
    ;;
esac
