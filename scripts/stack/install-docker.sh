#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/common.sh"

require_root
detect_os

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  log "docker already running"
  exit 0
fi

ensure_prereqs

case "$PKG" in
  apt)
    if try_apt_retry docker.io; then
      :
    else
      log "trying Docker official apt repo …"
      setup_docker_apt_repo
      if try_apt_retry docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin; then
        log "installed docker-ce from official repo"
      else
        log "Docker apt repo failed, trying get.docker.com script …"
        install_docker_official_script
        exit 0
      fi
    fi
    try_apt docker-compose-plugin || try_apt docker-compose || true
    enable_start docker
    ;;
  dnf|yum)
    if $PKG install -y docker docker-compose-plugin 2>/dev/null; then
      :
    elif $PKG install -y docker 2>/dev/null; then
      :
    else
      install_docker_official_script
      exit 0
    fi
    enable_start docker
    ;;
  *) die "unsupported package manager: $PKG" ;;
esac
log "docker installed"
