#!/usr/bin/env bash
# OWPanel — universal Linux installer (Ubuntu / Debian / CentOS / Rocky / AlmaLinux / RHEL)
# install.sh version: 2026-06-19-1
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/opt/owpanel}"
PORT="${OWPANEL_PORT:-8888}"
# lnmp = Nginx + MariaDB + PHP + FTP + Certbot (recommended for new servers)
# web  = Nginx + PHP only
# none = panel only, no runtime stack
INSTALL_STACK="${INSTALL_STACK:-lnmp}"
INSTALL_NGINX="${INSTALL_NGINX:-1}"
PANEL_USER="${PANEL_USER:-root}"
FROM_SOURCE="${FROM_SOURCE:-0}"
REPO_URL="${REPO_URL:-https://github.com/luuuunet/owpanel.git}"
SOURCE_REF="${SOURCE_REF:-main}"
RELEASE_VERSION="${RELEASE_VERSION:-v0.1.15}"
RELEASE_DIR="${RELEASE_DIR:-}"

export GIT_TERMINAL_PROMPT=0

log() { echo "[owpanel] $*"; }
die() { echo "[owpanel] ERROR: $*" >&2; exit 1; }

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

install_deps_minimal() {
  log "安装基础依赖 (curl/git/sqlite/常用工具)..."
  case "$PKG" in
    apt)
      export DEBIAN_FRONTEND=noninteractive
      apt-get update -qq
      apt-get install -y -qq curl ca-certificates tar git unzip sqlite3 acl openssl sudo
      ;;
    dnf)
      dnf install -y curl ca-certificates tar git unzip sqlite acl openssl sudo
      ;;
    yum)
      yum install -y curl ca-certificates tar git unzip sqlite acl openssl sudo
      ;;
  esac
}

install_build_deps() {
  log "安装编译依赖 (Go/Node/build-essential)..."
  case "$PKG" in
    apt)
      export DEBIAN_FRONTEND=noninteractive
      apt-get install -y -qq xz-utils sqlite3 build-essential
      ;;
    dnf)
      dnf install -y xz sqlite
      ;;
    yum)
      yum install -y xz sqlite
      ;;
  esac
}

install_go_if_needed() {
  go_version_ok() {
    command -v go >/dev/null 2>&1 || return 1
    local ver maj min
    ver="$(go version 2>/dev/null | grep -oE 'go[0-9]+\.[0-9]+(\.[0-9]+)?' | head -1 | sed 's/^go//')"
    [[ -n "$ver" ]] || return 1
    maj="${ver%%.*}"
    min="${ver#*.}"; min="${min%%.*}"
    [[ "$maj" -gt 1 ]] || { [[ "$maj" -eq 1 ]] && [[ "$min" -ge 22 ]]; }
  }
  if go_version_ok; then
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
  local tgz="/tmp/go${GO_VERSION}.linux-${GOARCH}.tar.gz"
  curl -fL --connect-timeout 30 --max-time 600 --retry 3 --retry-delay 3 \
    -o "$tgz" "https://go.dev/dl/go${GO_VERSION}.linux-${GOARCH}.tar.gz"
  tar -C /usr/local -xzf "$tgz"
  rm -f "$tgz"
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
  local txz="/tmp/node-v${NODE_VERSION}-linux-${NODEARCH}.tar.xz"
  curl -fL --connect-timeout 30 --max-time 600 --retry 3 --retry-delay 3 \
    -o "$txz" "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODEARCH}.tar.xz"
  tar -xJf "$txz" -C /usr/local --strip-components=1
  rm -f "$txz"
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
  local slug archive carchive tgz
  slug="$(repo_slug)"
  if [[ "$SOURCE_REF" == v* ]]; then
    archive="https://github.com/${slug}/archive/refs/tags/${SOURCE_REF}.tar.gz"
    carchive="https://codeload.github.com/${slug}/tar.gz/${SOURCE_REF}"
  else
    archive="https://github.com/${slug}/archive/refs/heads/${SOURCE_REF}.tar.gz"
    carchive="https://codeload.github.com/${slug}/tar.gz/refs/heads/${SOURCE_REF}"
  fi
  tgz="$(mktemp /tmp/owpanel-src.XXXXXX.tar.gz)"
  log "下载源码包 ${SOURCE_REF}（约 1–5 分钟，请耐心等待）..."
  if curl -fL --connect-timeout 30 --max-time 900 --retry 3 --retry-delay 5 \
      --progress-bar -o "$tgz" "$carchive"; then
    log "下载完成 ($(du -h "$tgz" | awk '{print $1}'))"
  elif curl -fL --connect-timeout 30 --max-time 900 --retry 3 --retry-delay 5 \
      --progress-bar -o "$tgz" "$archive"; then
    log "下载完成 ($(du -h "$tgz" | awk '{print $1}'))"
  else
    rm -f "$tgz"
    die "无法下载源码 github.com/${slug} @ ${SOURCE_REF}（请检查网络）"
  fi
  log "解压源码包..."
  mkdir -p "$dest"
  tar -xzf "$tgz" -C "$dest" --strip-components=1
  rm -f "$tgz"
  if [[ ! -f "$dest/backend/internal/services/logs/logs.go" ]]; then
    die "源码不完整（缺少 logs 模块）。请使用: SOURCE_REF=v0.1.0 sudo bash $0"
  fi
  log "源码就绪: ${SOURCE_REF}"
}

build_from_source() {
  log "从源码构建（小内存机器可能需要 10–20 分钟）..."
  install_build_deps
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
  export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
  export NODE_OPTIONS="${NODE_OPTIONS:---max-old-space-size=768}"
  log "编译后端..."
  cd "$SRC/backend"
  go mod download
  CGO_ENABLED=0 go build -ldflags="-s -w" -o "$INSTALL_DIR/owpanel" ./cmd/server
  CGO_ENABLED=0 go build -ldflags="-s -w" -o "$INSTALL_DIR/op" ./cmd/op
  if command -v npm >/dev/null 2>&1; then
    log "构建前端（npm，请耐心等待）..."
    cd "$SRC/frontend"
    if [[ -f package-lock.json ]]; then npm ci; else npm install; fi
    npm run build
    rm -rf "$INSTALL_DIR/web"
    cp -a "$SRC/backend/web" "$INSTALL_DIR/web"
  elif [[ -d "$SRC/backend/web" && -n "$(ls -A "$SRC/backend/web" 2>/dev/null)" ]]; then
    rm -rf "$INSTALL_DIR/web"
    cp -a "$SRC/backend/web" "$INSTALL_DIR/web"
  else
    die "未找到 npm，且仓库内无预构建 frontend。请安装 Node.js 18+ 后重试"
  fi
  log "构建完成"
}

install_binary_layout() {
  mkdir -p "$INSTALL_DIR/data" "$INSTALL_DIR/logs"
  chmod +x "$INSTALL_DIR/owpanel" 2>/dev/null || true
  ln -sf "$INSTALL_DIR/op" /usr/local/bin/op 2>/dev/null || true
  rm -f /usr/local/bin/bt "$INSTALL_DIR/bt" 2>/dev/null || true
  local stack_src=""
  for candidate in \
    "$(dirname "$0")/stack" \
    "$(cd "$(dirname "$0")/.." && pwd)/scripts/stack"; do
    if [[ -f "$candidate/fallback.sh" ]]; then
      stack_src="$candidate"
      break
    fi
  done
  if [[ -n "$stack_src" ]]; then
    mkdir -p "$INSTALL_DIR/scripts"
    rm -rf "$INSTALL_DIR/scripts/stack"
    cp -a "$stack_src" "$INSTALL_DIR/scripts/stack"
    find "$INSTALL_DIR/scripts/stack" -name '*.sh' -exec chmod +x {} \;
    find "$INSTALL_DIR/scripts/stack" -name '*.sh' -exec sed -i 's/\r$//' {} \; 2>/dev/null || true
    log "已安装 stack 备用脚本 → $INSTALL_DIR/scripts/stack"
    return 0
  fi
  # Fallback: download from GitHub release or main branch
  local dest="$INSTALL_DIR/scripts/stack"
  mkdir -p "$dest"
  local urls=(
    "https://github.com/luuuunet/owpanel/releases/download/${RELEASE_VERSION}/owpanel-stack-scripts.tar.gz"
    "https://github.com/luuuunet/owpanel/releases/latest/download/owpanel-stack-scripts.tar.gz"
  )
  local url
  for url in "${urls[@]}"; do
    if curl -fsSL --connect-timeout 30 --max-time 120 "$url" | tar -xzf - -C "$dest" 2>/dev/null; then
      find "$dest" -name '*.sh' -exec chmod +x {} \;
      log "已从 GitHub 下载 stack 安装脚本 → $dest"
      return 0
    fi
  done
  log "WARN: 未找到本地 stack 脚本，GitHub 下载亦失败；软件安装将尝试在线拉取 fallback.sh"
}

write_systemd() {
  log "配置 systemd 服务..."
  cat > /etc/systemd/system/owpanel.service <<EOF
[Unit]
Description=OWPanel Server Management
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$PANEL_USER
WorkingDirectory=$INSTALL_DIR
Environment=OWPANEL_PORT=$PORT
Environment=OWPANEL_HOME=$INSTALL_DIR
Environment=OWPANEL_DATA=$INSTALL_DIR/data
Environment=OWPANEL_WEB=$INSTALL_DIR/web
ExecStart=$INSTALL_DIR/owpanel
Restart=on-failure
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable owpanel
  systemctl restart owpanel
}

store_app_count() {
  local db="$1"
  [[ -f "$db" ]] || { echo 0; return; }
  if command -v sqlite3 >/dev/null 2>&1; then
    sqlite3 "$db" 'SELECT COUNT(*) FROM apps WHERE deleted_at IS NULL;' 2>/dev/null || echo 0
    return
  fi
  python3 - "$db" <<'PY' 2>/dev/null || echo 0
import sqlite3, sys
try:
    con = sqlite3.connect(sys.argv[1])
    print(con.execute('SELECT COUNT(*) FROM apps WHERE deleted_at IS NULL').fetchone()[0])
except Exception:
    print(0)
PY
}

wait_for_store_catalog() {
  local db="$INSTALL_DIR/data/panel.db" n=0
  log "等待软件商店目录初始化..."
  for _ in $(seq 1 60); do
    n="$(store_app_count "$db")"
    if [[ "${n:-0}" -ge 50 ]]; then
      log "软件商店已就绪（${n} 个软件）"
      return 0
    fi
    sleep 2
  done
  log "软件商店仍在后台初始化，首次打开面板时会自动同步"
  return 1
}

install_from_release() {
  local src="${RELEASE_DIR:-}"
  if [[ -z "$src" ]]; then
    local script_root
    script_root="$(cd "$(dirname "$0")/.." && pwd)"
    if [[ -f "$script_root/owpanel" && -d "$script_root/web" ]]; then
      src="$script_root"
    fi
  fi
  if [[ -n "$src" && -f "$src/owpanel" ]]; then
    log "从发布包安装: $src"
    cp -f "$src/owpanel" "$INSTALL_DIR/owpanel"
    cp -f "$src/op" "$INSTALL_DIR/op" 2>/dev/null || true
    rm -f "$INSTALL_DIR/bt" 2>/dev/null || true
    rm -rf "$INSTALL_DIR/web"
    cp -a "$src/web" "$INSTALL_DIR/web"
    return 0
  fi
  return 1
}

release_package_name() {
  case "$(uname -m)" in
    x86_64) echo "owpanel-linux-amd64" ;;
    aarch64|arm64) echo "owpanel-linux-arm64" ;;
    *) die "不支持的 CPU 架构: $(uname -m)" ;;
  esac
}

install_from_github_release() {
  [[ "$FROM_SOURCE" == "1" ]] && return 1
  local slug pkg ver url tgz tmpdir versions v
  slug="$(repo_slug)"
  pkg="$(release_package_name)"
  versions=("$RELEASE_VERSION")
  v="$(curl -fsSL --connect-timeout 10 --max-time 20 \
    "https://api.github.com/repos/${slug}/releases/latest" 2>/dev/null \
    | grep -oE '"tag_name"[[:space:]]*:[[:space:]]*"v[^"]+"' \
    | head -1 | grep -oE 'v[^"]+' || true)"
  if [[ -n "$v" && "$v" != "$RELEASE_VERSION" ]]; then
    versions+=("$v")
  fi
  for ver in "${versions[@]}"; do
    url="https://github.com/${slug}/releases/download/${ver}/${pkg}.tar.gz"
    tgz="$(mktemp /tmp/owpanel-rel.XXXXXX.tar.gz)"
    log "快速安装：下载预编译包 ${ver} (${pkg})..."
    if curl -fL --connect-timeout 30 --max-time 600 --retry 3 --retry-delay 5 \
        --progress-bar -o "$tgz" "$url"; then
      log "解压预编译包 ($(du -h "$tgz" | awk '{print $1}'))..."
      tmpdir="$(mktemp -d)"
      tar -xzf "$tgz" -C "$tmpdir"
      rm -f "$tgz"
      local root="$tmpdir/$pkg"
      [[ -d "$root" ]] || root="$tmpdir"
      [[ -f "$root/owpanel" && -d "$root/web" ]] || die "预编译包格式错误"
      cp -f "$root/owpanel" "$INSTALL_DIR/owpanel"
      cp -f "$root/op" "$INSTALL_DIR/op" 2>/dev/null || true
      rm -rf "$INSTALL_DIR/web"
      cp -a "$root/web" "$INSTALL_DIR/web"
      rm -rf "$tmpdir"
      log "预编译包安装完成（约 1–2 分钟）"
      return 0
    fi
    rm -f "$tgz"
    log "预编译包不可用 (${ver})，尝试下一版本..."
  done
  log "无可用预编译包，将尝试源码构建..."
  return 1
}

open_firewall() {
  if command -v ufw >/dev/null 2>&1 && ufw status | grep -qi active; then
    ufw allow "$PORT/tcp" || true
    ufw allow 80/tcp || true
    ufw allow 443/tcp || true
  elif command -v firewall-cmd >/dev/null 2>&1; then
    firewall-cmd --permanent --add-port="${PORT}/tcp" 2>/dev/null || true
    firewall-cmd --permanent --add-service=http 2>/dev/null || true
    firewall-cmd --permanent --add-service=https 2>/dev/null || true
    firewall-cmd --reload 2>/dev/null || true
  fi
}

install_web_server() {
  if [[ "${INSTALL_NGINX}" != "1" ]]; then
    log "跳过 Nginx (INSTALL_NGINX=${INSTALL_NGINX})"
    return 0
  fi
  if ! command -v nginx >/dev/null 2>&1; then
    log "安装 Nginx（网站托管 80 端口）..."
    case "$PKG" in
      apt)
        export DEBIAN_FRONTEND=noninteractive
        if ! apt-get install -y -qq nginx 2>/dev/null; then
          log "默认 apt 安装 Nginx 失败，尝试 stack 脚本 …"
          bash "$(dirname "$0")/stack/fallback.sh" nginx
        fi
        ;;
      dnf|yum)
        $PKG install -y nginx
        ;;
      *)
        log "当前包管理器不支持自动安装 Nginx"
        return 0
        ;;
    esac
  else
    log "Nginx 已存在，配置 OWPanel 虚拟主机..."
  fi
  configure_nginx_for_owpanel
}

configure_nginx_for_owpanel() {
  local data_dir="$INSTALL_DIR/data"
  local vhost_dir="$data_dir/nginx/vhosts"
  local panel_conf="$data_dir/nginx/owpanel.conf"
  local main_conf="/etc/nginx/nginx.conf"
  mkdir -p "$vhost_dir" "$data_dir/logs"
  cat > "$panel_conf" <<EOF
# OWPanel auto-generated
include ${vhost_dir}/*.conf;
EOF

  if [[ -f "$main_conf" ]] && ! grep -q 'owpanel-vhosts' "$main_conf"; then
    sed -i "/http {/a\\    # owpanel-vhosts\\n    include ${panel_conf};" "$main_conf"
  fi
  for f in /etc/nginx/sites-enabled/default /etc/nginx/conf.d/default.conf; do
    [[ -f "$f" && ! -f "${f}.owpanel-disabled" ]] && mv "$f" "${f}.owpanel-disabled" || true
  done

  if command -v nginx >/dev/null 2>&1 && nginx -t >/dev/null 2>&1; then
    systemctl enable nginx >/dev/null 2>&1 || true
    systemctl restart nginx >/dev/null 2>&1 || true
    log "Nginx 已就绪（80 端口）"
  else
    log "Nginx 配置需检查，请登录面板 → 软件商店"
  fi
}

install_php() {
  log "安装 PHP-FPM（PHP 站点必需）..."
  local svc=""
  case "$PKG" in
    apt)
      export DEBIAN_FRONTEND=noninteractive
      local pkgs=()
      if apt-cache show php8.3-fpm >/dev/null 2>&1; then
        pkgs=(php8.3-fpm php8.3-mysql php8.3-cli php8.3-common php8.3-xml php8.3-curl php8.3-mbstring php8.3-gd php8.3-zip)
        svc=php8.3-fpm
      elif apt-cache show php8.2-fpm >/dev/null 2>&1; then
        pkgs=(php8.2-fpm php8.2-mysql php8.2-cli php8.2-common php8.2-xml php8.2-curl php8.2-mbstring php8.2-gd php8.2-zip)
        svc=php8.2-fpm
      else
        pkgs=(php-fpm php-mysql php-cli php-xml php-curl php-mbstring php-gd php-zip)
        svc=php-fpm
      fi
      apt-get install -y -qq "${pkgs[@]}"
      ;;
    dnf|yum)
      $PKG install -y php-fpm php-mysqlnd php-cli php-xml php-mbstring php-gd php-zip
      svc=php-fpm
      ;;
    *)
      log "跳过 PHP（不支持的包管理器）"
      return 0
      ;;
  esac
  if [[ -n "$svc" ]] && systemctl list-unit-files "${svc}.service" >/dev/null 2>&1; then
    systemctl enable "$svc" >/dev/null 2>&1 || true
    systemctl start "$svc" >/dev/null 2>&1 || true
    log "PHP-FPM 已启动 ($svc)"
  fi
}

install_database() {
  log "安装 MariaDB/MySQL（建站数据库）..."
  case "$PKG" in
    apt)
      export DEBIAN_FRONTEND=noninteractive
      if ! apt-get install -y -qq mariadb-server 2>/dev/null \
        && ! apt-get install -y -qq default-mysql-server 2>/dev/null \
        && ! apt-get install -y -qq mysql-server 2>/dev/null; then
        log "默认 apt 安装数据库失败，尝试 stack 脚本 …"
        bash "$(dirname "$0")/stack/fallback.sh" mariadb
      fi
      ;;
    dnf|yum)
      $PKG install -y mariadb-server 2>/dev/null || $PKG install -y mysql-server
      ;;
  esac
  for svc in mariadb mysql mysqld; do
    if systemctl list-unit-files "${svc}.service" >/dev/null 2>&1; then
      systemctl enable "$svc" >/dev/null 2>&1 || true
      systemctl start "$svc" >/dev/null 2>&1 || true
      log "数据库服务已启动 ($svc)"
      return 0
    fi
  done
}

install_ftp() {
  log "安装 Pure-FTPd（建站 FTP）..."
  case "$PKG" in
    apt)
      export DEBIAN_FRONTEND=noninteractive
      apt-get install -y -qq pure-ftpd
      ;;
    dnf|yum)
      $PKG install -y pure-ftpd
      ;;
  esac
  if systemctl list-unit-files pure-ftpd.service >/dev/null 2>&1; then
    systemctl enable pure-ftpd >/dev/null 2>&1 || true
    systemctl start pure-ftpd >/dev/null 2>&1 || true
    log "Pure-FTPd 已启动"
  fi
}

install_certbot() {
  log "安装 Certbot（免费 SSL）..."
  case "$PKG" in
    apt)
      export DEBIAN_FRONTEND=noninteractive
      apt-get install -y -qq certbot python3-certbot-nginx 2>/dev/null || apt-get install -y -qq certbot
      ;;
    dnf|yum)
      $PKG install -y certbot python3-certbot-nginx 2>/dev/null || $PKG install -y certbot
      ;;
  esac
}

tune_host_memory() {
  local ram_mb swap_mb
  ram_mb="$(awk '/MemTotal/{printf "%d", $2/1024}' /proc/meminfo 2>/dev/null || echo 0)"
  swap_mb="$(awk '/SwapTotal/{printf "%d", $2/1024}' /proc/meminfo 2>/dev/null || echo 0)"
  log "系统内存: ${ram_mb}MB, Swap: ${swap_mb}MB"

  if [[ "${swap_mb:-0}" -eq 0 && "${ram_mb:-0}" -gt 0 && "${ram_mb:-0}" -lt 4096 ]]; then
    log "未检测到 Swap，创建 1GB swapfile（缓解 kswapd0 内存抖动）..."
    if [[ ! -f /swapfile ]]; then
      fallocate -l 1G /swapfile 2>/dev/null || dd if=/dev/zero of=/swapfile bs=1M count=1024 status=none
      chmod 600 /swapfile
      mkswap /swapfile
    fi
    swapon /swapfile 2>/dev/null || true
    grep -q '/swapfile' /etc/fstab 2>/dev/null || echo '/swapfile none swap sw 0 0' >> /etc/fstab
  fi

  cat >/etc/sysctl.d/99-owpanel-memory.conf <<'EOF'
# OWPanel auto-generated memory tuning
vm.swappiness=10
vm.vfs_cache_pressure=50
EOF
  sysctl -p /etc/sysctl.d/99-owpanel-memory.conf 2>/dev/null || sysctl -w vm.swappiness=10 vm.vfs_cache_pressure=50 2>/dev/null || true

  if [[ "${ram_mb:-0}" -lt 1024 && "${INSTALL_STACK}" == "lnmp" ]]; then
    log "低内存 (${ram_mb}MB)：自动切换 INSTALL_STACK=web（跳过 MariaDB/FTP）"
    INSTALL_STACK=web
  elif [[ "${ram_mb:-0}" -lt 2048 ]]; then
    log "小内存 (${ram_mb}MB)：已启用 Swap 与内核调优；MariaDB 将使用低内存配置"
    local dbconf="/etc/mysql/mariadb.conf.d/99-owpanel-lowmem.cnf"
    mkdir -p "$(dirname "$dbconf")"
    cat >"$dbconf" <<'EOF'
[mysqld]
innodb_buffer_pool_size = 64M
max_connections = 50
performance_schema = OFF
EOF
  fi
}

install_runtime_stack() {
  tune_host_memory
  local stack
  stack="$(echo "${INSTALL_STACK}" | tr '[:upper:]' '[:lower:]')"
  case "$stack" in
    none|0|skip)
      log "跳过运行环境安装 (INSTALL_STACK=${INSTALL_STACK})"
      return 0
      ;;
    web)
      install_web_server
      install_php
      ;;
    lnmp|*)
      install_web_server
      install_php
      install_database
      install_ftp
      install_certbot
      ;;
  esac
}

read_admin_credentials() {
  local cred="$INSTALL_DIR/data/INITIAL_CREDENTIALS.txt"
  local i user pass
  for i in $(seq 1 20); do
    if [[ -f "$cred" ]]; then
      user="$(grep -m1 '^Username:' "$cred" 2>/dev/null | sed 's/^Username:[[:space:]]*//')"
      pass="$(grep -m1 '^Password:' "$cred" 2>/dev/null | sed 's/^Password:[[:space:]]*//')"
      if [[ -n "$pass" ]]; then
        printf '%s|%s' "${user:-admin}" "$pass"
        return 0
      fi
    fi
    pass="$(journalctl -u owpanel --no-pager -n 100 2>/dev/null \
      | grep -m1 'first login' \
      | grep -oE 'password: [^ ]+' \
      | awk '{print $2}' || true)"
    if [[ -n "$pass" ]]; then
      printf 'admin|%s' "$pass"
      return 0
    fi
    sleep 1
  done
  return 1
}

print_install_summary() {
  local ip panel_url cred user pass
  ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  [[ -n "$ip" ]] || ip="127.0.0.1"
  panel_url="http://${ip}:${PORT}"

  user="admin"
  pass=""
  cred="$(read_admin_credentials || true)"
  if [[ -n "$cred" ]]; then
    user="${cred%%|*}"
    pass="${cred#*|}"
  fi

  echo ""
  echo "========================================="
  echo "  OWPanel installed successfully"
  echo "========================================="
  echo ""
  echo "  Panel URL:  ${panel_url}"
  echo "  Username:   ${user}"
  if [[ -n "$pass" ]]; then
    echo "  Password:   ${pass}"
  else
    echo "  Password:   (starting up — run: op info)"
  fi
  echo ""
  echo "  Panel CLI (run anytime):"
  echo "    op info       Show panel URLs, port, and data directory"
  echo "    op config     Change port, security entrance, or SSL"
  echo "    op restart    Restart the panel service"
  echo "    op uninstall  Remove panel service and files (sudo)"
  echo ""
  echo "  Change your password after first login."
  echo ""
  echo "  Runtime stack (${INSTALL_STACK}):"
  command -v nginx >/dev/null 2>&1 && echo "    Nginx:    running on port 80" || echo "    Nginx:    not installed"
  systemctl is-active php8.3-fpm php8.2-fpm php-fpm 2>/dev/null | grep -q active && echo "    PHP-FPM:  active" || echo "    PHP-FPM:  check Software Store"
  systemctl is-active mariadb mysql mysqld 2>/dev/null | grep -q active && echo "    Database: active" || echo "    Database: optional / not running"
  if command -v nginx >/dev/null 2>&1; then
    echo "  Websites:   http://${ip}/"
  fi
  echo "  Tip: INSTALL_STACK=web|lnmp|none  —  lighter install: INSTALL_STACK=web"
  echo "========================================="
}

main() {
  echo "========================================="
  echo "  OWPanel Linux Installer"
  echo "  installer: 2026-06-19-1 (stack: ${INSTALL_STACK})"
  echo "========================================="
  require_root
  detect_os
  install_deps_minimal
  mkdir -p "$INSTALL_DIR"
  if install_from_release; then
    log "Installed from local release bundle"
  elif install_from_github_release; then
    :
  elif [[ "$FROM_SOURCE" == "1" ]] || [[ ! -f "$INSTALL_DIR/owpanel" ]]; then
    build_from_source
  else
    log "Using existing binary: $INSTALL_DIR/owpanel"
  fi
  install_binary_layout
  write_systemd
  wait_for_store_catalog || true
  install_runtime_stack
  open_firewall
  print_install_summary
}

main "$@"
