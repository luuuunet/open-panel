#!/usr/bin/env bash
# Shared helpers for OWPanel stack fallback installers (idempotent, noninteractive).
set -euo pipefail

log() { echo "[owpanel-stack] $*"; }
die() { echo "[owpanel-stack] ERROR: $*" >&2; exit 1; }

require_root() {
  [[ "$(id -u)" -eq 0 ]] || die "must run as root"
}

detect_os() {
  [[ -f /etc/os-release ]] || die "missing /etc/os-release"
  # shellcheck disable=SC1091
  . /etc/os-release
  OS_ID="${ID:-unknown}"
  OS_ID_LIKE="${ID_LIKE:-}"
  OS_CODENAME="${VERSION_CODENAME:-}"
  OS_PRETTY="${PRETTY_NAME:-$OS_ID}"
  PKG=""
  case "$OS_ID" in
    ubuntu|debian) PKG="apt" ;;
    centos|rhel|rocky|almalinux|fedora|ol)
      if command -v dnf >/dev/null 2>&1; then PKG="dnf"; else PKG="yum"; fi
      ;;
    *)
      if echo "$OS_ID_LIKE" | grep -qE 'debian|ubuntu'; then PKG="apt"
      elif echo "$OS_ID_LIKE" | grep -qE 'rhel|fedora|centos'; then
        if command -v dnf >/dev/null 2>&1; then PKG="dnf"; else PKG="yum"; fi
      fi
      ;;
  esac
  [[ -n "$PKG" ]] || die "unsupported OS: $OS_PRETTY"
}

apt_update() {
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
}

apt_install() {
  export DEBIAN_FRONTEND=noninteractive
  apt-get install -y -qq \
    -o Dpkg::Options::=--force-confdef \
    -o Dpkg::Options::=--force-confold \
    "$@"
}

ensure_codename() {
  if [[ -n "${OS_CODENAME:-}" ]]; then
    return 0
  fi
  if command -v lsb_release >/dev/null 2>&1; then
    OS_CODENAME="$(lsb_release -cs 2>/dev/null || true)"
  fi
  [[ -n "${OS_CODENAME:-}" ]] || die "cannot detect distro codename"
}

ensure_prereqs() {
  case "$PKG" in
    apt)
      apt_update
      apt_install ca-certificates curl gnupg lsb-release apt-transport-https
      ;;
    dnf|yum)
      $PKG install -y curl ca-certificates gnupg2
      ;;
  esac
}

service_active() {
  local svc="$1"
  systemctl is-active --quiet "$svc" 2>/dev/null
}

enable_start() {
  local svc="$1"
  systemctl enable "$svc" >/dev/null 2>&1 || true
  systemctl start "$svc"
}

enable_start_any() {
  local svc
  for svc in "$@"; do
    if systemctl list-unit-files "${svc}.service" >/dev/null 2>&1; then
      enable_start "$svc"
      return 0
    fi
  done
  return 1
}

try_apt() {
  apt_install "$@" 2>/dev/null
}

install_docker_official_script() {
  command -v curl >/dev/null 2>&1 || die "curl required for Docker official install"
  log "installing Docker via get.docker.com script …"
  curl -fsSL --connect-timeout 30 --max-time 600 --retry 3 https://get.docker.com | sh
  systemctl enable docker >/dev/null 2>&1 || true
  systemctl start docker
}

tune_mariadb_lowmem() {
  local ram_mb
  ram_mb="$(awk '/MemTotal/{printf "%d", $2/1024}' /proc/meminfo 2>/dev/null || echo 0)"
  [[ "${ram_mb:-0}" -lt 2048 ]] || return 0
  local dbconf="/etc/mysql/mariadb.conf.d/99-owpanel-lowmem.cnf"
  mkdir -p "$(dirname "$dbconf")"
  cat >"$dbconf" <<'EOF'
[mysqld]
innodb_buffer_pool_size = 64M
max_connections = 50
performance_schema = OFF
EOF
  log "applied low-memory MariaDB config (${ram_mb}MB RAM)"
}
