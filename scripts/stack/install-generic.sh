#!/usr/bin/env bash
# Install simple apt/dnf packages (memcached, fail2ban, supervisor, mail, ftp).
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/common.sh"

GENERIC_KEY="${GENERIC_KEY:-${1:-}}"
[[ -n "$GENERIC_KEY" ]] || die "usage: $0 memcached|fail2ban|supervisor|pureftpd|postfix|dovecot"

require_root
detect_os

declare -a PKGS=()
declare -a SVCS=()

case "$GENERIC_KEY" in
  memcached)
    PKGS=(memcached)
    SVCS=(memcached)
    ;;
  fail2ban)
    PKGS=(fail2ban)
    SVCS=(fail2ban)
    ;;
  supervisor)
    PKGS=(supervisor)
    SVCS=(supervisor)
    ;;
  pureftpd)
    PKGS=(pure-ftpd pure-ftpd-common pure-ftpd)
    SVCS=(pure-ftpd)
    ;;
  postfix)
    PKGS=(postfix)
    SVCS=(postfix)
    ;;
  dovecot)
    PKGS=(dovecot-core dovecot-imapd dovecot-pop3d dovecot)
    SVCS=(dovecot)
    ;;
  *)
    die "unsupported generic component: $GENERIC_KEY"
    ;;
esac

for svc in "${SVCS[@]}"; do
  if service_active "$svc"; then
    log "$GENERIC_KEY already running ($svc)"
    exit 0
  fi
done

ensure_prereqs

case "$PKG" in
  apt)
    if [[ "$GENERIC_KEY" == "postfix" ]]; then
      export DEBIAN_FRONTEND=noninteractive
      debconf-set-selections <<'EOF' || true
postfix postfix/main_mailer_type select Local only
EOF
    fi
    installed=0
    for pkg in "${PKGS[@]}"; do
      if try_apt "$pkg"; then
        installed=1
        break
      fi
    done
    [[ "$installed" -eq 1 ]] || die "could not install $GENERIC_KEY from apt"
    ;;
  dnf|yum)
    $PKG install -y "${PKGS[@]}"
    ;;
  *) die "unsupported package manager: $PKG" ;;
esac

enable_start_any "${SVCS[@]}"
log "$GENERIC_KEY installed"
