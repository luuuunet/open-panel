#!/usr/bin/env bash
# OWPanel — 一键自动搭建（本地构建 / 安装 / 远程部署 / Docker）
# Usage:
#   ./scripts/auto-setup.sh              # 同 build
#   ./scripts/auto-setup.sh build        # 构建前端 + Linux 发布包
#   ./scripts/auto-setup.sh install      # 本机安装（需 root，Linux）
#   ./scripts/auto-setup.sh deploy       # 构建并部署到远程（需 scripts/deploy.env）
#   ./scripts/auto-setup.sh dev          # 开发模式（后端 + 提示启动前端）
#   ./scripts/auto-setup.sh docker       # Docker Compose 启动
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CMD="${1:-build}"
DIST="$ROOT/dist/owpanel-linux-amd64"
GOARCH="${DEPLOY_GOARCH:-amd64}"

log() { echo "[auto-setup] $*"; }
die() { echo "[auto-setup] ERROR: $*" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "缺少命令: $1"
}

build_frontend() {
  log "构建前端..."
  need_cmd npm
  cd "$ROOT/frontend"
  if [[ -f package-lock.json ]]; then npm ci; else npm install; fi
  npm run build
}

build_backend_linux() {
  log "交叉编译 Linux/$GOARCH 后端..."
  need_cmd go
  mkdir -p "$DIST/data"
  cd "$ROOT/backend"
  GOOS=linux GOARCH="$GOARCH" CGO_ENABLED=0 \
    go build -ldflags="-s -w" -o "$DIST/owpanel" ./cmd/server
  GOOS=linux GOARCH="$GOARCH" CGO_ENABLED=0 \
    go build -ldflags="-s -w" -o "$DIST/op" ./cmd/op
  rm -rf "$DIST/web"
  cp -a "$ROOT/backend/web" "$DIST/web"
  log "发布包: $DIST"
}

cmd_build() {
  build_frontend
  build_backend_linux
  log "构建完成"
}

cmd_install() {
  if [[ "$(id -u)" -ne 0 ]]; then
    die "install 需要 root: sudo bash $0 install"
  fi
  FROM_SOURCE=1 bash "$ROOT/scripts/install.sh"
}

cmd_deploy() {
  if [[ ! -f "$ROOT/scripts/deploy.env" ]]; then
    die "请先复制 scripts/deploy.env.example 为 scripts/deploy.env 并填写服务器信息"
  fi
  cmd_build
  need_cmd python3
  python3 -c "import paramiko" 2>/dev/null || die "请安装 paramiko: pip install paramiko"
  python3 "$ROOT/scripts/deploy.py" --full --binary "$DIST/owpanel"
}

cmd_dev() {
  log "安装前端依赖..."
  cd "$ROOT/frontend"
  if [[ -f package-lock.json ]]; then npm ci; else npm install; fi
  cd "$ROOT/backend"
  go mod download
  log "请在另一终端运行: cd frontend && npm run dev"
  log "启动后端..."
  go run ./cmd/server
}

cmd_docker() {
  need_cmd docker
  cd "$ROOT"
  docker compose up -d --build
  log "Docker 已启动: http://127.0.0.1:8888"
}

case "$CMD" in
  build)   cmd_build ;;
  install) cmd_install ;;
  deploy)  cmd_deploy ;;
  dev)     cmd_dev ;;
  docker)  cmd_docker ;;
  help|-h|--help)
    sed -n '2,10p' "$0"
    ;;
  *)
    die "未知命令: $CMD（可用: build | install | deploy | dev | docker）"
    ;;
esac
