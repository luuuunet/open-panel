#!/usr/bin/env bash
# Install PHP-FPM via apt; falls back to ondrej/php PPA on Debian/Ubuntu.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/common.sh"

PHP_VERSION="${PHP_VERSION:-8.3}"
require_root
detect_os

php_pkg() {
  echo "php${PHP_VERSION}-$1"
}

php_svc="php${PHP_VERSION}-fpm"
if command -v "php${PHP_VERSION}" >/dev/null 2>&1 && systemctl list-unit-files "${php_svc}.service" >/dev/null 2>&1; then
  if service_active "$php_svc"; then
    log "PHP ${PHP_VERSION} already running"
    exit 0
  fi
fi

ensure_prereqs

case "$PKG" in
  apt)
    pkgs=(
      "$(php_pkg fpm)" "$(php_pkg mysql)" "$(php_pkg cli)" "$(php_pkg common)"
      "$(php_pkg xml)" "$(php_pkg curl)" "$(php_pkg mbstring)" "$(php_pkg gd)" "$(php_pkg zip)"
    )
    if try_apt_retry "${pkgs[@]}"; then
      enable_start "$php_svc"
      log "PHP ${PHP_VERSION} installed from default apt"
      exit 0
    fi
    log "default apt PHP ${PHP_VERSION} unavailable, trying third-party repo …"
    setup_php_repo
    apt_install_retry "${pkgs[@]}"
    enable_start "$php_svc"
    log "PHP ${PHP_VERSION} installed from third-party repo"
    ;;
  dnf|yum)
    $PKG install -y php-fpm php-mysqlnd php-cli php-xml php-mbstring php-gd php-zip
    enable_start php-fpm
    log "PHP installed from $PKG"
    ;;
  *)
    die "unsupported package manager: $PKG"
    ;;
esac
