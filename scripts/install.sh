#!/usr/bin/env bash
# Open Panel — universal Linux installer (Ubuntu / Debian / CentOS / Rocky / AlmaLinux / RHEL)
# install.sh version: 2026-06-13-3
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/opt/open-panel}"
PORT="${OPEN_PANEL_PORT:-8888}"
PANEL_USER="${PANEL_USER:-root}"
FROM_SOURCE="${FROM_SOURCE:-0}"
REPO_URL="${REPO_URL:-https://github.com/luuuunet/open-panel.git}"
BRANCH="${BRANCH:-main}"
RELEASE_DIR="${RELEASE_DIR:-}"

export GIT_TERMINAL_PROMPT=0

log() { echo "[open-panel] $*"; }
die() { echo "[open-panel] ERROR: $*" >&2; exit 1; }

require_root() {
  if [[ "$(id -u)" -ne 0 ]]; then
    die "请使用 root 运行，或: sudo bash $0"
  fi
}

detect_os() {
  if [[ ! -f /etc/os-release ]]; then
    die "无法识别 Linux 发行版（缺少 /etc/os-release）"
  fi
  # shellcheck disable=SC1091
  . /etc/os-release
  OS_ID="${ID:-unknown}"
  OS_ID_LIKE="${ID_LIKE:-}"
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
  [[ -n "$PKG" ]] || die "不支持的发行版: $OS_PRETTY（需要 apt / dnf / yum）"
  log "检测到: $OS_PRETTY，包管理器: $PKG"
}

install_deps() {
  log "安装基础依赖..."
  case "$PKG" in
    apt)
      export DEBIAN_FRONTEND=noninteractive
      apt-get update -qq
      apt-get install -y -qq curl ca-certificates tar xz-utils sqlite3 build-essential
      ;;
    dnf)
      dnf install -y curl ca-certificates git sqlite
      ;;
    yum)
      yum install -y curl ca-certificates git sqlite
      ;;
  esac
}

install_go_if_needed() {
  if command -v go >/dev/null 2>&1; then
    export PATH="$(dirname "$(command -v go)"):$PATH"
    return
  fi
  log "安装 Go 1.22..."
  GO_VERSION="1.22.5"
  ARCH="$(uname -m)"
  case "$ARCH" in
    x86_64) GOARCH="amd64" ;;
    aarch64|arm64) GOARCH="arm64" ;;
    *) die "不支持的 CPU 架构: $ARCH" ;;
  esac
  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GOARCH}.tar.gz" | tar -C /usr/local -xzf -
  export PATH="/usr/local/go/bin:$PATH"
  grep -q '/usr/local/go/bin' /etc/profile || echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
}

install_node_if_needed() {
  if command -v npm >/dev/null 2>&1; then
    return
  fi
  log "安装 Node.js 18..."
  case "$PKG" in
    apt)
      apt-get install -y -qq nodejs npm 2>/dev/null || true
      ;;
    dnf|yum)
      $PKG install -y nodejs npm 2>/dev/null || true
      ;;
  esac
  if command -v npm >/dev/null 2>&1; then
    return
  fi
  NODE_VERSION="18.20.4"
  ARCH="$(uname -m)"
  case "$ARCH" in
    x86_64) NODEARCH="x64" ;;
    aarch64|arm64) NODEARCH="arm64" ;;
    *) die "不支持的 CPU 架构: $ARCH" ;;
  esac
  curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODEARCH}.tar.xz" \
    | tar -xJ -C /usr/local --strip-components=1
  hash -r 2>/dev/null || true
}

repo_slug() {
  local url="${REPO_URL%.git}"
  url="${url#https://github.com/}"
  url="${url#http://github.com/}"
  echo "$url"
}

fetch_repo() {
  local dest="$1"
  local slug archive carchive
  slug="$(repo_slug)"
  archive="https://github.com/${slug}/archive/refs/heads/${BRANCH}.tar.gz"
  carchive="https://codeload.github.com/${slug}/tar.gz/refs/heads/${BRANCH}"
  log "下载源码包: github.com/${slug} (${BRANCH})"
  mkdir -p "$dest"
  if curl -fsSL "$archive" | tar -xz -C "$dest" --strip-components=1; then
    return 0
  fi
  log "GitHub archive 失败，尝试 codeload..."
  if curl -fsSL "$carchive" | tar -xz -C "$dest" --strip-components=1; then
    return 0
  fi
  die "无法下载源码 github.com/${slug}（请检查网络，或设置 REPO_URL）"
}

build_from_source() {
  log "从源码构建..."
  install_go_if_needed
  install_node_if_needed
  WORK="$(mktemp -d)"
  trap 'rm -rf "$WORK"' EXIT
  if [[ -d "$INSTALL_DIR/.git" ]] && command -v git >/dev/null 2>&1; then
    git -C "$INSTALL_DIR" pull --ff-only || true
    SRC="$INSTALL_DIR"
  else
    fetch_repo "$WORK/src"
    SRC="$WORK/src"
  fi
  export PATH="/usr/local/go/bin:/usr/local/bin:$PATH"
  cd "$SRC/backend"
  go mod download
  CGO_ENABLED=0 go build -ldflags="-s -w" -o "$INSTALL_DIR/open-panel" ./cmd/server
  CGO_ENABLED=0 go build -ldflags="-s -w" -o "$INSTALL_DIR/op" ./cmd/op
  if command -v npm >/dev/null 2>&1; then
    cd "$SRC/frontend"
    if [[ -f package-lock.json ]]; then npm ci; else npm install; fi
    npm run build
    rm -rf "$INSTALL_DIR/web"
    cp -a "$SRC/backend/web" "$INSTALL_DIR/web"
  elif [[ -d "$SRC/backend/web" && -n "$(ls -A "$SRC/backend/web" 2>/dev/null)" ]]; then
    rm -rf "$INSTALL_DIR/web"
    cp -a "$SRC/backend/web" "$INSTALL_DIR/web"
  else
    die "未找到 npm，且仓库内无预构建 frontend（backend/web 为空）。请安装 Node.js 18+ 后重试"
  fi
}

install_binary_layout() {
  mkdir -p "$INSTALL_DIR/data" "$INSTALL_DIR/logs"
  chmod +x "$INSTALL_DIR/open-panel" 2>/dev/null || true
  ln -sf "$INSTALL_DIR/op" /usr/local/bin/op 2>/dev/null || true
  rm -f /usr/local/bin/bt "$INSTALL_DIR/bt" 2>/dev/null || true
}

write_systemd() {
  log "配置 systemd 服务..."
  cat > /etc/systemd/system/open-panel.service <<EOF
[Unit]
Description=Open Panel Server Management
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$PANEL_USER
WorkingDirectory=$INSTALL_DIR
Environment=OPEN_PANEL_PORT=$PORT
Environment=OPEN_PANEL_HOME=$INSTALL_DIR
Environment=OPEN_PANEL_DATA=$INSTALL_DIR/data
Environment=OPEN_PANEL_WEB=$INSTALL_DIR/web
ExecStart=$INSTALL_DIR/open-panel
Restart=on-failure
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable open-panel
  systemctl restart open-panel
}

install_from_release() {
  local src="${RELEASE_DIR:-}"
  if [[ -z "$src" ]]; then
    local script_root
    script_root="$(cd "$(dirname "$0")/.." && pwd)"
    if [[ -f "$script_root/open-panel" && -d "$script_root/web" ]]; then
      src="$script_root"
    fi
  fi
  if [[ -n "$src" && -f "$src/open-panel" ]]; then
    log "从发布包安装: $src"
    cp -f "$src/open-panel" "$INSTALL_DIR/open-panel"
    cp -f "$src/op" "$INSTALL_DIR/op" 2>/dev/null || true
    rm -f "$INSTALL_DIR/bt" 2>/dev/null || true
    rm -rf "$INSTALL_DIR/web"
    cp -a "$src/web" "$INSTALL_DIR/web"
    return 0
  fi
  return 1
}

open_firewall() {
  if command -v ufw >/dev/null 2>&1 && ufw status | grep -qi active; then
    ufw allow "$PORT/tcp" || true
  elif command -v firewall-cmd >/dev/null 2>&1; then
    firewall-cmd --permanent --add-port="${PORT}/tcp" 2>/dev/null || true
    firewall-cmd --reload 2>/dev/null || true
  fi
}

main() {
  echo "========================================="
  echo "  Open Panel 多系统安装 (Linux)"
  echo "  installer: 2026-06-13-3"
  echo "========================================="
  require_root
  detect_os
  install_deps
  mkdir -p "$INSTALL_DIR"
  if install_from_release; then
    log "发布包已部署"
  elif [[ "$FROM_SOURCE" == "1" ]] || [[ ! -f "$INSTALL_DIR/open-panel" ]]; then
    build_from_source
  else
    log "使用已有二进制: $INSTALL_DIR/open-panel"
  fi
  install_binary_layout
  write_systemd
  open_firewall
  IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
  echo ""
  echo "========================================="
  echo "  安装完成"
  echo "  地址: http://${IP:-127.0.0.1}:$PORT"
  echo "  账号: admin / (随机密码)"
  echo "  密码文件: $INSTALL_DIR/data/INITIAL_CREDENTIALS.txt"
  echo "  或: journalctl -u open-panel | grep password"
  echo "========================================="
}

main "$@"
