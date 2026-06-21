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

apt_sanitize_known_bad_repos() {
  local f changed=0
  shopt -s nullglob
  for f in /etc/apt/sources.list.d/*.list; do
    if grep -q 'repo.mongodb.org' "$f" 2>/dev/null; then
      if grep -qE '/ubuntu (noble|mantic)/' "$f" 2>/dev/null; then
        log "fixing MongoDB apt repo → jammy in $(basename "$f")"
        sed -i 's|/ubuntu noble/mongodb-org/|/ubuntu jammy/mongodb-org/|g;s|/ubuntu mantic/mongodb-org/|/ubuntu jammy/mongodb-org/|g' "$f" 2>/dev/null || rm -f "$f"
        changed=1
      fi
    fi
  done
  shopt -u nullglob
  [[ "$changed" == "1" ]]
}

apt_update() {
  export DEBIAN_FRONTEND=noninteractive
  apt_sanitize_known_bad_repos || true
  if apt-get update -qq 2>/tmp/owpanel-apt.err; then
    return 0
  fi
  if grep -qE 'mongodb.org|does not have a Release file' /tmp/owpanel-apt.err 2>/dev/null; then
    log "removing broken MongoDB apt lists so other packages can install …"
    rm -f /etc/apt/sources.list.d/mongodb-org-*.list
    apt-get update -qq
    return $?
  fi
  cat /tmp/owpanel-apt.err >&2
  return 1
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
      apt_sanitize_known_bad_repos || true
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

apt_fix_broken() {
  export DEBIAN_FRONTEND=noninteractive
  dpkg --configure -a 2>/dev/null || true
  apt-get install -y -f -qq \
    -o Dpkg::Options::=--force-confdef \
    -o Dpkg::Options::=--force-confold 2>/dev/null || true
}

apt_install_retry() {
  local pkg
  apt_fix_broken
  if apt_install "$@"; then
    return 0
  fi
  apt_fix_broken
  apt_install "$@"
}

try_apt_retry() {
  apt_fix_broken
  try_apt "$@" || {
    apt_fix_broken
    try_apt "$@"
  }
}

gpg_dearmor_url() {
  local url="$1" dest="$2"
  install -d -m 0755 "$(dirname "$dest")"
  curl -fsSL --connect-timeout 30 --max-time 120 --retry 3 "$url" \
    | gpg --batch --yes --dearmor -o "$dest"
}

apt_arch() {
  dpkg --print-architecture 2>/dev/null || echo "amd64"
}

write_apt_repo() {
  local file="$1" content="$2"
  install -d -m 0755 "$(dirname "$file")"
  printf '%s\n' "$content" >"$file"
}

setup_mariadb_official_repo() {
  local ver="${1:-10.11}"
  local setup="/tmp/owpanel_mariadb_repo_setup"
  log "configuring MariaDB ${ver} official repo …"
  curl -fsSL --connect-timeout 30 --max-time 120 --retry 3 \
    -o "$setup" https://r.mariadb.com/downloads/mariadb_repo_setup 2>/dev/null || \
    curl -fsSL -o "$setup" https://downloads.mariadb.com/MariaDB/mariadb_repo_setup
  chmod +x "$setup"
  if "$setup" --mariadb-server-version="mariadb-${ver}" --skip-maxscale --skip-tools --yes 2>/dev/null; then
    return 0
  fi
  "$setup" --mariadb-server-version="mariadb-${ver}" --skip-maxscale --skip-tools
}

setup_docker_apt_repo() {
  local distro="$OS_ID"
  [[ "$distro" == "debian" || "$distro" == "ubuntu" ]] || distro="ubuntu"
  ensure_codename
  gpg_dearmor_url "https://download.docker.com/linux/${distro}/gpg" /usr/share/keyrings/docker.gpg
  write_apt_repo /etc/apt/sources.list.d/docker-owpanel.list \
    "deb [arch=$(apt_arch) signed-by=/usr/share/keyrings/docker.gpg] https://download.docker.com/linux/${distro} ${OS_CODENAME} stable"
  apt_update
}

setup_php_repo() {
  ensure_codename
  case "$OS_ID" in
    ubuntu)
      apt_install software-properties-common 2>/dev/null || true
      add-apt-repository -y ppa:ondrej/php
      apt_update
      ;;
    debian)
      gpg_dearmor_url "https://packages.sury.org/php/apt.gpg" /usr/share/keyrings/sury-php.gpg
      write_apt_repo /etc/apt/sources.list.d/sury-php.list \
        "deb [signed-by=/usr/share/keyrings/sury-php.gpg] https://packages.sury.org/php/ ${OS_CODENAME} main"
      apt_update
      ;;
    *)
      die "unsupported OS for PHP repo: $OS_PRETTY"
      ;;
  esac
}

setup_mongodb_repo() {
  local ver="${1:-7.0}"
  ensure_codename
  gpg_dearmor_url "https://pgp.mongodb.com/server-${ver}.asc" "/usr/share/keyrings/mongodb-server-${ver}.gpg"
  local suite
  suite="$(mongodb_apt_suite)"
  if [[ "$OS_ID" == "ubuntu" ]]; then
    write_apt_repo "/etc/apt/sources.list.d/mongodb-org-${ver}.list" \
      "deb [arch=$(apt_arch) signed-by=/usr/share/keyrings/mongodb-server-${ver}.gpg] https://repo.mongodb.org/apt/ubuntu ${suite}/mongodb-org/${ver} multiverse"
  else
    write_apt_repo "/etc/apt/sources.list.d/mongodb-org-${ver}.list" \
      "deb [signed-by=/usr/share/keyrings/mongodb-server-${ver}.gpg] https://repo.mongodb.org/apt/debian ${suite}/mongodb-org/${ver} main"
  fi
  apt_update
}

# MongoDB apt repos lag new distro releases; map to the nearest supported suite.
mongodb_apt_suite() {
  ensure_codename
  local suite="${OS_CODENAME}"
  if [[ "${OS_ID}" == "ubuntu" ]]; then
    case "${suite}" in
      jammy|focal) ;;
      *)
        log "MongoDB apt has no ${suite} suite; using jammy repository"
        suite="jammy"
        ;;
    esac
  elif [[ "${OS_ID}" == "debian" ]]; then
    case "${suite}" in
      bookworm|bullseye) ;;
      *)
        log "MongoDB apt has no ${suite} suite; using bookworm repository"
        suite="bookworm"
        ;;
    esac
  fi
  echo "${suite}"
}

install_docker_official_script() {
  command -v curl >/dev/null 2>&1 || die "curl required for Docker official install"
  log "installing Docker via get.docker.com script …"
  curl -fsSL --connect-timeout 30 --max-time 600 --retry 3 https://get.docker.com | sh
  systemctl enable docker >/dev/null 2>&1 || true
  systemctl start docker
}

stop_conflicting_webservers() {
  for svc in nginx apache2 httpd openresty; do
    systemctl stop "$svc" 2>/dev/null || true
  done
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
