<p align="center">
  <img src="frontend/public/logo.svg" alt="Open Panel logo" width="128" style="border-radius: 22%;" />
</p>

<h1 align="center">Open Panel</h1>

<p align="center">
  Open-source Linux server management panel · Modern Vue 3 UI<br/>
  开源 Linux 服务器运维管理面板 · 现代化 Web 界面
</p>

<p align="center">
  <a href="#zh-cn"><img src="https://img.shields.io/badge/语言-中文-blue?style=flat-square" alt="中文"></a>
  <a href="#english"><img src="https://img.shields.io/badge/Language-English-blue?style=flat-square" alt="English"></a>
  <a href="docs/README.md"><img src="https://img.shields.io/badge/文档%20%2F%20Docs-README-lightgrey?style=flat-square" alt="文档 / Docs"></a>
</p>

---

<a id="zh-cn"></a>

## 中文

Open Panel 是一个开源的 Linux 服务器运维管理面板，参考 [1Panel](https://1panel.cn/) 等现代面板的功能设计，提供 Web 管理界面。

📖 **完整用户手册**：[docs/zh-CN/USER_GUIDE.md](docs/zh-CN/USER_GUIDE.md) · [English](docs/en/USER_GUIDE.md)

### 功能特性

| 模块 | 说明 |
|------|------|
| **仪表盘** | 实时监控、健康评分、一键优化、全球流量地图与地理策略 |
| **网站** | Nginx/OpenResty 虚拟主机、SSL、CDN 缓存、WordPress 工具箱、Node.js/Java 运行环境 |
| **数据库** | MySQL / PostgreSQL / Redis 管理、扩展、备份导入、OSS 远程备份 |
| **文件 / OSS** | 在线文件管理、压缩解压、对象存储同步 |
| **Docker** | 容器/镜像管理、Docker Compose 多模板一键部署 |
| **FTP / 邮件** | Pure-FTPd 用户同步、Postfix/Dovecot 虚拟邮箱 |
| **备份 / 计划任务** | 网站/数据库/目录定时备份，FTP/SFTP/WebDAV/OSS 远程目标；Cron 模板 |
| **监控** | 可用性探测、集群节点、自动化运维、DevOps 中心 |
| **缓存&安全** | 防火墙、Nginx WAF、安全检测、登录审计、2FA、Edge/Kafka/Cilium（可选） |
| **SSH / PAM** | Web SSH 多会话、AI 助手、资产/权限/审计/JIT/账号轮换/批量运维 |
| **工具箱** | 网络诊断、端口进程、命令片段、系统健康 |
| **AI 中心** | AI 应用与面板级模型配置 |
| **其他** | DNS 管理、软件商店、扩展、phpMyAdmin、日志中心 |
| **用户权限** | 管理员 / 普通用户 / 子账户，9 项模块权限 + 磁盘配额 |
| **国际化** | 简体中文 / 繁体中文 / English |

### 技术栈

| 层级 | 技术 |
|------|------|
| 后端 | Go 1.22+ / Gin / GORM / SQLite |
| 前端 | Vue 3 / Vite / Element Plus / Pinia |
| 部署 | 单二进制 + 内嵌前端 / systemd |

### 快速开始

#### 环境要求

- **Linux（生产推荐）**：Ubuntu 20.04+、Debian 11+、CentOS 7+、Rocky / AlmaLinux / RHEL（自动识别 `apt` / `dnf` / `yum`）
- **Windows**：Windows 10/11、Windows Server（本地开发/演示，部分系统级功能降级）
- **Docker**：任意支持 Docker 的主机（见下方 `docker compose`）
- Go 1.22+（仅源码编译时需要）
- Node.js 18+（仅前端开发/构建时需要）

#### 一键安装（多系统）

**Ubuntu / Debian / CentOS / Rocky / AlmaLinux**

```bash
curl -fsSL https://raw.githubusercontent.com/luuuunet/open-panel/main/scripts/install.sh | sudo bash
# 或本地：sudo bash scripts/install.sh
# 从源码：sudo FROM_SOURCE=1 bash scripts/install.sh
# 自定义目录：sudo INSTALL_DIR=/opt/open-panel OPEN_PANEL_PORT=8888 bash scripts/install.sh
```

**Windows（管理员 PowerShell）**

```powershell
Set-ExecutionPolicy Bypass -Scope Process -Force
cd C:\path\to\open-panel
.\scripts\install.ps1 -FromSource
# 自定义：.\scripts\install.ps1 -InstallDir C:\open-panel -Port 8888
```

**Docker（Linux / Windows / macOS 均可）**

```bash
docker compose up -d --build
# 访问 http://localhost:8888
# 或使用自动搭建脚本：
bash scripts/auto-setup.sh docker
# Windows: .\scripts\auto-setup.ps1 -Action docker
```

#### 自动搭建脚本

项目提供统一的自动搭建入口，覆盖**本地构建、本机安装、远程部署、开发模式、Docker** 等场景。

| 脚本 | 平台 | 说明 |
|------|------|------|
| `scripts/auto-setup.sh` | Linux / macOS / Git Bash | Bash 一键脚本 |
| `scripts/auto-setup.ps1` | Windows PowerShell | PowerShell 一键脚本 |
| `scripts/deploy.py` | 全平台 | 远程 SSH 部署（上传二进制 + 前端） |
| `scripts/deploy.env.example` | — | 远程部署配置模板 |
| `scripts/install.sh` | Linux | 本机安装（被 `auto-setup install` 调用） |
| `scripts/install.ps1` | Windows | 本机安装（被 `auto-setup install` 调用） |
| `scripts/build-release.sh` | Linux | 多架构发布包构建（维护者） |

**环境依赖**

| 场景 | 需要 |
|------|------|
| `build` | Go 1.22+、Node.js 18+ |
| `install` | Linux 需 root；Windows 需管理员 |
| `deploy` | 上述构建依赖 + Python 3 + `pip install paramiko` |
| `docker` | Docker + Docker Compose |

**命令一览**

| 模式 | Linux / Git Bash | Windows PowerShell | 作用 |
|------|------------------|--------------------|------|
| 构建 | `bash scripts/auto-setup.sh build` | `.\scripts\auto-setup.ps1 -Action build` | 构建前端 + 交叉编译 Linux 二进制 |
| 安装 | `sudo bash scripts/auto-setup.sh install` | `.\scripts\auto-setup.ps1 -Action install` | 本机安装面板并注册服务 |
| 部署 | `bash scripts/auto-setup.sh deploy` | `.\scripts\auto-setup.ps1 -Action deploy` | 构建后上传到远程服务器并重启 |
| 开发 | `bash scripts/auto-setup.sh dev` | `.\scripts\auto-setup.ps1 -Action dev` | 启动后端（前端另开终端 `npm run dev`） |
| Docker | `bash scripts/auto-setup.sh docker` | `.\scripts\auto-setup.ps1 -Action docker` | `docker compose up -d --build` |

构建产物默认输出到 `dist/open-panel-linux-amd64/`：

```
dist/open-panel-linux-amd64/
├── open-panel      # Linux 二进制
├── op              # CLI 工具
└── web/            # 前端静态文件
```

**远程部署配置**

1. 复制配置模板：

```bash
cp scripts/deploy.env.example scripts/deploy.env
# Windows: copy scripts\deploy.env.example scripts\deploy.env
```

2. 编辑 `scripts/deploy.env`：

```ini
DEPLOY_HOST=your-server-ip      # 目标服务器 IP 或域名
DEPLOY_USER=root                # SSH 用户名
DEPLOY_KEY=~/.ssh/id_rsa        # SSH 私钥路径（推荐）
# DEPLOY_PASSWORD=              # 或使用密码（二选一）
OPEN_PANEL_PORT=8888            # 面板端口
INSTALL_DIR=/opt/open-panel     # 远程安装目录
DEPLOY_GOARCH=amd64             # 目标 CPU 架构：amd64 | arm64
```

3. 安装 Python 依赖并执行部署：

```bash
pip install paramiko

# Linux / Git Bash — 一键构建 + 部署
bash scripts/auto-setup.sh deploy

# Windows PowerShell
.\scripts\auto-setup.ps1 -Action deploy
```

`deploy` 流程：构建前端 → 交叉编译 Linux 后端 → SSH 上传二进制与 `web/` → 重启远程 `open-panel` 服务。

也可单独使用 `deploy.py`：

```bash
# 仅上传前端（需已构建 backend/web）
python3 scripts/deploy.py --web-only

# 上传二进制 + 前端
python3 scripts/deploy.py --full --binary dist/open-panel-linux-amd64/open-panel
```

**典型场景**

```bash
# 场景 1：开发者在 Windows 上改代码，部署到 Linux 生产机
copy scripts\deploy.env.example scripts\deploy.env
# 编辑 deploy.env 填写服务器
pip install paramiko
.\scripts\auto-setup.ps1 -Action deploy

# 场景 2：在 Linux 服务器上从源码安装面板
git clone https://github.com/luuuunet/open-panel.git
cd open-panel
sudo bash scripts/auto-setup.sh install

# 场景 3：仅本地构建发布包，手动拷贝到服务器
bash scripts/auto-setup.sh build
# 将 dist/open-panel-linux-amd64/ 上传到服务器后：
# cp open-panel /opt/open-panel/open-panel && systemctl restart open-panel

# 场景 4：维护者构建多架构 tar 包
bash scripts/build-release.sh
# 产出 dist/open-panel-linux-amd64.tar.gz 等
```

**Makefile 快捷命令**

```bash
make setup    # 等同 bash scripts/auto-setup.sh build
make deploy   # 等同 bash scripts/auto-setup.sh deploy
make install  # 等同 bash scripts/install.sh
```

安装完成后 API `GET /api/system/platform` 可查看当前系统的包管理器与能力开关（需登录）。

#### 各系统能力对照

| 能力 | Ubuntu/Debian | CentOS/RHEL 系 | Windows |
|------|---------------|----------------|---------|
| 面板 Web 管理 | ✅ | ✅ | ✅ |
| apt/dnf/yum 装软件 | ✅ apt | ✅ dnf/yum | ⚠️ winget |
| systemd 服务 | ✅ | ✅ | ⚠️ 计划任务 |
| 防火墙写入系统 | ✅ ufw/firewalld | ✅ | ❌ 仅面板记录 |
| FTP/邮件同步 | ✅ | ✅ | ❌ |
| Docker 应用商店 | ✅ | ✅ | ✅（需 Docker Desktop） |

Windows 可编译运行，部分系统级功能（防火墙/FTP/邮件同步）为模拟或跳过。

#### 源码开发

```bash
git clone https://github.com/luuuunet/open-panel.git
cd open-panel

# 后端
cd backend
go mod download
go run ./cmd/server

# 前端（另开终端，开发模式）
cd frontend
npm install
npm run dev

# 或构建并嵌入后端
cd frontend && npm run build
cd ../backend && go build -o open-panel ./cmd/server/
```

默认访问地址：`http://服务器IP:8888`  
首次登录：用户名 `admin`，密码为**随机生成**，保存在服务器 `data/INITIAL_CREDENTIALS.txt`，或查看启动日志（`journalctl -u open-panel` / 控制台输出）。请登录后立即修改密码；若启用安全入口则为 `http://IP:8888/<安全路径>/`

#### 数据目录约定

Open Panel 将所有可管理数据放在 `dataDir`（默认 `/opt/open-panel/data`，可通过安装脚本 `INSTALL_DIR` 调整）下，例如：

| 路径 | 用途 |
|------|------|
| `{dataDir}/wwwroot` | 网站根目录 |
| `{dataDir}/server/{app}` | 已安装软件（Nginx、MySQL、PHP 等） |
| `{dataDir}/apps/{app}` | Docker 一键应用 |
| `{dataDir}/ai/{app}` | AI 相关应用 |
| `{dataDir}/logs` | 站点/WAF/面板日志 |
| `{dataDir}/backup` | 本地备份 |

#### Panel CLI `op`

```bash
op              # 交互菜单（先显示面板信息）
op info         # 面板 URL、端口与数据目录
op config       # 修改端口、安全入口或 SSL
op restart      # 重启面板服务
op help         # 完整命令列表
```

### 子账户权限

子账户 (subuser) 可在 **用户管理** 中创建，按模块分配权限：

`websites` · `databases` · `files` · `docker` · `ftp` · `mail` · `backup` · `monitor` · `bastion`（SSH / PAM）

后端 API 与前端菜单均会校验权限。软件安装、防火墙/WAF 写入、DNS、日志、DevOps、AI、工具箱、面板设置等仅管理员可用；子账户凭 `bastion` 权限可进入 **SSH / PAM** 连接已授权资产。

详细说明见 [用户手册](docs/zh-CN/USER_GUIDE.md#16-用户与权限)。

### 项目结构

```
open-panel/
├── backend/              # Go API 服务
│   ├── cmd/server/       # 入口
│   └── internal/         # 业务逻辑
├── frontend/             # Vue 3 前端
│   └── public/logo.svg   # 项目 Logo
├── scripts/
│   ├── auto-setup.sh     # 自动搭建（Linux / macOS / Git Bash）
│   ├── auto-setup.ps1    # 自动搭建（Windows）
│   ├── deploy.py         # 远程 SSH 部署
│   ├── deploy.env.example
│   ├── install.sh        # Linux 安装
│   ├── install.ps1       # Windows 安装
│   └── build-release.sh  # 多架构发布包
└── docker-compose.yml
```

### 路线图

- [x] 仪表盘与系统监控
- [x] 网站 / Nginx / SSL / CDN 缓存
- [x] 数据库管理与备份（含 OSS）
- [x] 文件管理器与对象存储
- [x] Docker / Compose 模板部署
- [x] 防火墙 / WAF / 安全扫描
- [x] 计划任务与备份任务
- [x] FTP / 邮件 / DNS
- [x] 多用户与子账户权限、磁盘配额
- [x] SSH / PAM（堡垒机能力整合至 SSH 终端）
- [x] 流量地图地理钻取与策略
- [x] 仪表盘健康评分与一键优化
- [x] 工具箱与 PGSQL 扩展管理
- [x] 登录 2FA、三语界面
- [ ] WHM 风格经销商 / 多租户
- [x] LNMP/LAMP 一键栈与环境就绪度检查
- [ ] 备份对象存储可靠性与告警通知
- [ ] 更多 Compose 应用商店模板
- [ ] 插件 / 扩展生态

### 贡献

欢迎提交 Issue 和 Pull Request！

### 许可证

[MIT License](LICENSE)

---

<a id="english"></a>

## English

Open Panel is an open-source Linux server management panel inspired by modern panels such as [1Panel](https://1panel.cn/), with a full web UI for day-to-day operations.

📖 **User guides:** [中文](docs/zh-CN/USER_GUIDE.md) · [English](docs/en/USER_GUIDE.md)

### Features

| Module | Description |
|--------|-------------|
| **Dashboard** | Real-time metrics, health score, one-click optimize, global traffic map & geo policies |
| **Websites** | Nginx/OpenResty vhosts, SSL, CDN cache, WordPress toolkit, Node.js/Java runtimes |
| **Databases** | MySQL / PostgreSQL / Redis, extensions, backup/import, OSS remote backup |
| **Files / OSS** | Online file manager, archive tools, object storage sync |
| **Docker** | Container/image management, one-click Docker Compose templates |
| **FTP / Mail** | Pure-FTPd user sync, Postfix/Dovecot virtual mailboxes |
| **Backup / Cron** | Scheduled site/DB/directory backups to FTP/SFTP/WebDAV/OSS; cron templates |
| **Monitoring** | Uptime probes, cluster nodes, auto ops, DevOps center |
| **Protection** | Firewall, Nginx WAF, security scans, login audit, 2FA, optional Edge/Kafka/Cilium |
| **SSH / PAM** | Multi-tab Web SSH, AI assistant, assets/permissions/audit/JIT/account rotation/batch ops |
| **Toolbox** | Network diagnostics, ports/processes, command snippets, system health |
| **AI Hub** | AI apps and panel-level model configuration |
| **Other** | DNS, app store, extensions, phpMyAdmin, log center |
| **Users** | Admin / regular / sub-users, 9 module permissions, disk quotas |
| **i18n** | Simplified Chinese / Traditional Chinese / English |

### Tech stack

| Layer | Stack |
|-------|-------|
| Backend | Go 1.22+ / Gin / GORM / SQLite |
| Frontend | Vue 3 / Vite / Element Plus / Pinia |
| Deploy | Single binary + embedded frontend / systemd |

### Quick start

#### Requirements

- **Linux (recommended for production)**: Ubuntu 20.04+, Debian 11+, CentOS 7+, Rocky / AlmaLinux / RHEL (`apt` / `dnf` / `yum` auto-detected)
- **Windows**: Windows 10/11, Windows Server (dev/demo; some OS-level features are limited)
- **Docker**: Any host with Docker (see `docker compose` below)
- Go 1.22+ (source builds only)
- Node.js 18+ (frontend dev/build only)

#### One-click install

**Ubuntu / Debian / CentOS / Rocky / AlmaLinux**

```bash
curl -fsSL https://raw.githubusercontent.com/luuuunet/open-panel/main/scripts/install.sh | sudo bash
# Local: sudo bash scripts/install.sh
# From source: sudo FROM_SOURCE=1 bash scripts/install.sh
# Custom path: sudo INSTALL_DIR=/opt/open-panel OPEN_PANEL_PORT=8888 bash scripts/install.sh
```

**Windows (Administrator PowerShell)**

```powershell
Set-ExecutionPolicy Bypass -Scope Process -Force
cd C:\path\to\open-panel
.\scripts\install.ps1 -FromSource
# Custom: .\scripts\install.ps1 -InstallDir C:\open-panel -Port 8888
```

**Docker (Linux / Windows / macOS)**

```bash
docker compose up -d --build
# Open http://localhost:8888
# Or use the auto-setup script:
bash scripts/auto-setup.sh docker
# Windows: .\scripts\auto-setup.ps1 -Action docker
```

#### Auto-setup scripts

Unified entry points for **local build, install, remote deploy, dev mode, and Docker**.

| Script | Platform | Description |
|--------|----------|-------------|
| `scripts/auto-setup.sh` | Linux / macOS / Git Bash | Bash one-click script |
| `scripts/auto-setup.ps1` | Windows PowerShell | PowerShell one-click script |
| `scripts/deploy.py` | All platforms | Remote SSH deploy (binary + web) |
| `scripts/deploy.env.example` | — | Remote deploy config template |
| `scripts/install.sh` | Linux | Local install (used by `auto-setup install`) |
| `scripts/install.ps1` | Windows | Local install (used by `auto-setup install`) |
| `scripts/build-release.sh` | Linux | Multi-arch release packages (maintainers) |

**Dependencies**

| Mode | Requires |
|------|----------|
| `build` | Go 1.22+, Node.js 18+ |
| `install` | root on Linux; Administrator on Windows |
| `deploy` | build deps + Python 3 + `pip install paramiko` |
| `docker` | Docker + Docker Compose |

**Commands**

| Mode | Linux / Git Bash | Windows PowerShell | Purpose |
|------|------------------|--------------------|---------|
| Build | `bash scripts/auto-setup.sh build` | `.\scripts\auto-setup.ps1 -Action build` | Build frontend + cross-compile Linux binary |
| Install | `sudo bash scripts/auto-setup.sh install` | `.\scripts\auto-setup.ps1 -Action install` | Install panel locally and register service |
| Deploy | `bash scripts/auto-setup.sh deploy` | `.\scripts\auto-setup.ps1 -Action deploy` | Build, upload to remote server, restart |
| Dev | `bash scripts/auto-setup.sh dev` | `.\scripts\auto-setup.ps1 -Action dev` | Run backend (frontend: `npm run dev` in another terminal) |
| Docker | `bash scripts/auto-setup.sh docker` | `.\scripts\auto-setup.ps1 -Action docker` | `docker compose up -d --build` |

Build output goes to `dist/open-panel-linux-amd64/`:

```
dist/open-panel-linux-amd64/
├── open-panel      # Linux binary
├── op              # CLI tool
└── web/            # Frontend static assets
```

**Remote deploy configuration**

1. Copy the template:

```bash
cp scripts/deploy.env.example scripts/deploy.env
# Windows: copy scripts\deploy.env.example scripts\deploy.env
```

2. Edit `scripts/deploy.env`:

```ini
DEPLOY_HOST=your-server-ip      # Target server IP or hostname
DEPLOY_USER=root                # SSH username
DEPLOY_KEY=~/.ssh/id_rsa        # SSH private key path (recommended)
# DEPLOY_PASSWORD=              # Or use password (pick one)
OPEN_PANEL_PORT=8888            # Panel port
INSTALL_DIR=/opt/open-panel     # Remote install directory
DEPLOY_GOARCH=amd64             # Target CPU arch: amd64 | arm64
```

3. Install Python deps and deploy:

```bash
pip install paramiko

# Linux / Git Bash — build + deploy
bash scripts/auto-setup.sh deploy

# Windows PowerShell
.\scripts\auto-setup.ps1 -Action deploy
```

Deploy flow: build frontend → cross-compile Linux backend → upload binary and `web/` over SSH → restart remote `open-panel` service.

You can also use `deploy.py` directly:

```bash
# Web assets only (requires backend/web built)
python3 scripts/deploy.py --web-only

# Binary + web
python3 scripts/deploy.py --full --binary dist/open-panel-linux-amd64/open-panel
```

**Typical workflows**

```bash
# 1. Develop on Windows, deploy to Linux production
copy scripts\deploy.env.example scripts\deploy.env
# Edit deploy.env with server details
pip install paramiko
.\scripts\auto-setup.ps1 -Action deploy

# 2. Install from source on a Linux server
git clone https://github.com/luuuunet/open-panel.git
cd open-panel
sudo bash scripts/auto-setup.sh install

# 3. Build locally, copy to server manually
bash scripts/auto-setup.sh build
# Upload dist/open-panel-linux-amd64/ then:
# cp open-panel /opt/open-panel/open-panel && systemctl restart open-panel

# 4. Maintainer: multi-arch release tarballs
bash scripts/build-release.sh
# Output: dist/open-panel-linux-amd64.tar.gz, etc.
```

**Makefile shortcuts**

```bash
make setup    # same as bash scripts/auto-setup.sh build
make deploy   # same as bash scripts/auto-setup.sh deploy
make install  # same as bash scripts/install.sh
```

After install, call `GET /api/system/platform` (login required) to inspect package manager and capability flags.

#### Platform capability matrix

| Capability | Ubuntu/Debian | CentOS/RHEL family | Windows |
|------------|---------------|--------------------|---------|
| Panel web UI | ✅ | ✅ | ✅ |
| apt/dnf/yum app install | ✅ apt | ✅ dnf/yum | ⚠️ winget |
| systemd service | ✅ | ✅ | ⚠️ scheduled task |
| Firewall applied to OS | ✅ ufw/firewalld | ✅ | ❌ panel record only |
| FTP/mail sync | ✅ | ✅ | ❌ |
| Docker app store | ✅ | ✅ | ✅ (Docker Desktop) |

Windows builds run, but some OS-level features (firewall/FTP/mail sync) are simulated or skipped.

#### Source development

```bash
git clone https://github.com/luuuunet/open-panel.git
cd open-panel

# Backend
cd backend
go mod download
go run ./cmd/server

# Frontend (separate terminal, dev mode)
cd frontend
npm install
npm run dev

# Or build and embed frontend
cd frontend && npm run build
cd ../backend && go build -o open-panel ./cmd/server/
```

Default URL: `http://SERVER_IP:8888`  
First login: username `admin`, **random password** in `data/INITIAL_CREDENTIALS.txt` or server logs (`journalctl -u open-panel` / console). Change the password immediately. With security entrance enabled: `http://IP:8888/<entrance-path>/`

#### Data directory layout

All managed data lives under `dataDir` (default `/opt/open-panel/data`, configurable via `INSTALL_DIR`):

| Path | Purpose |
|------|---------|
| `{dataDir}/wwwroot` | Website document roots |
| `{dataDir}/server/{app}` | Installed apps (Nginx, MySQL, PHP, etc.) |
| `{dataDir}/apps/{app}` | One-click Docker apps |
| `{dataDir}/ai/{app}` | AI-related apps |
| `{dataDir}/logs` | Site/WAF/panel logs |
| `{dataDir}/backup` | Local backups |

#### Panel CLI `op`

```bash
op              # Interactive menu (shows panel info first)
op info         # Panel URLs, port, and data directory
op config       # Change port, entrance path, or SSL
op restart      # Restart the panel service
op help         # Full command list
```

### Sub-user permissions

Create sub-users under **User management** and assign module permissions:

`websites` · `databases` · `files` · `docker` · `ftp` · `mail` · `backup` · `monitor` · `bastion` (SSH / PAM)

API and UI enforce permissions. Software install, firewall/WAF write, DNS, logs, DevOps, AI, toolbox, and panel settings are admin-only; sub-users with `bastion` can use **SSH / PAM** for authorized assets.

See the [user guide](docs/en/USER_GUIDE.md#16-users--permissions) for details.

### Project structure

```
open-panel/
├── backend/              # Go API service
│   ├── cmd/server/       # Entry point
│   └── internal/         # Business logic
├── frontend/             # Vue 3 frontend
│   └── public/logo.svg   # Project logo
├── scripts/
│   ├── auto-setup.sh     # Auto-setup (Linux / macOS / Git Bash)
│   ├── auto-setup.ps1    # Auto-setup (Windows)
│   ├── deploy.py         # Remote SSH deploy
│   ├── deploy.env.example
│   ├── install.sh        # Linux install
│   ├── install.ps1       # Windows install
│   └── build-release.sh  # Multi-arch releases
└── docker-compose.yml
```

### Roadmap

- [x] Dashboard and system monitoring
- [x] Websites / Nginx / SSL / CDN cache
- [x] Database management and backup (incl. OSS)
- [x] File manager and object storage
- [x] Docker / Compose template deploy
- [x] Firewall / WAF / security scans
- [x] Cron and backup jobs
- [x] FTP / mail / DNS
- [x] Multi-user, sub-accounts, disk quotas
- [x] SSH / PAM (bastion merged into SSH terminal)
- [x] Traffic map geo drill-down and policies
- [x] Dashboard health score and one-click optimize
- [x] Toolbox and PostgreSQL extension management
- [x] Login 2FA and trilingual UI
- [ ] WHM-style reseller / multi-tenant
- [x] LNMP/LAMP one-click stack and readiness checks
- [ ] Backup object-storage reliability and alerts
- [ ] More Compose app-store templates
- [ ] Plugin / extension ecosystem

### Contributing

Issues and pull requests are welcome!

### License

[MIT License](LICENSE)
