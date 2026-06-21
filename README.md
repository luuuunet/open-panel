<h1 align="center">OWPanel</h1>

<p align="center">
  <strong>🌐 Open source & self-hosted · 🔓 Decentralized · 🤖 Automated Linux server control panel</strong>
</p>

<p align="center">
  <a href="https://github.com/luuuunet/owpanel/blob/main/LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
  <a href="https://github.com/luuuunet/owpanel"><img src="https://img.shields.io/badge/language-Go-green.svg" alt="Language"></a>
  <a href="https://github.com/luuuunet/owpanel"><img src="https://img.shields.io/badge/frontend-Vue3-brightgreen.svg" alt="Frontend"></a>
</p>

<p align="center">
  <a href="#-quick-install">⚡ Quick install</a> ·
  <a href="#-documentation">📚 Documentation</a> ·
  <a href="docs/zh-CN/USER_GUIDE.md">📖 中文手册</a> ·
  <a href="https://github.com/luuuunet/owpanel">🔗 GitHub</a>
</p>

---

**OWPanel** is a self-hosted Linux server control panel. Your data stays on your machine — no vendor cloud account required. Manage websites, databases, Docker, security, backups, and automation from one web UI.

> 📦 Formerly **Open Panel**. Repository: [github.com/luuuunet/owpanel](https://github.com/luuuunet/owpanel)

## ✨ Highlights

- 🛡️ **Self-hosted / decentralized** — Single-binary deploy, no third-party panel account
- 🚀 **Ready to use** — Embedded Vue 3 UI, systemd service, one-line Linux install
- ⚡ **Lightweight** — Go backend, ~16 MB prebuilt package, runs on 1 GB VPS
- 🌍 **Multilingual UI** — Simplified Chinese / Traditional Chinese / English
- 🔐 **Security hardening** — Security entrance, 2FA, IP allow/deny lists, session timeout, security headers
- 📊 **Smart ops** — Health score, one-click optimize, memory release, auto inspection & alerts
- 🤖 **AI assist** (optional) — Log analysis, terminal helper, site/deploy workflows
- 📥 **Official repos first** — App store uses apt/dnf packages first, GitHub stack scripts as fallback
- 🧩 **Extensible** — Extension marketplace, Docker Compose templates, one-click deploy
- 👥 **Sub-accounts** — Module-level permissions for team workflows
- ⌨️ **CLI** — `op` for panel config, service control, and updates

## 🛠 Modules

| Category | Features |
|----------|----------|
| 📊 **Overview** | Dashboard, CPU/memory/disk/network monitoring, health score, global traffic map, one-click optimize |
| 🌐 **Websites** | Virtual hosts (Nginx/OpenResty), SSL, rewrite/redirect, WordPress toolkit, A/B testing |
| ⚙️ **Runtimes** | PHP multi-version, Node.js / Java / Go / Rust / Python / .NET, PM2 / Docker |
| 🗄️ **Databases** | MySQL/MariaDB, PostgreSQL (incl. extensions), MongoDB, Redis, backup & restore |
| 🐳 **Containers** | Docker containers/images/volumes/networks, Compose projects, Portainer & templates |
| 📁 **Files** | Online file manager, upload/download, recycle bin, object storage (OSS) |
| 📧 **Mail & transfer** | Mail server (Postfix/Dovecot), FTP (Pure-FTPd), DNS management |
| 🛡️ **Security** | Firewall, Nginx WAF, CDN cache, Cilium policies, security scan, Fail2ban |
| 🤖 **Automation** | Cron jobs, panel/site/DB backups, uptime monitoring, auto-ops, DevOps center |
| ☸️ **Cluster** | Multi-node cluster agent, Kubernetes management |
| 📋 **Logs** | Panel/system/site/CDN/WAF log aggregation, AI log analysis |
| 🧠 **AI** | AI hub, Hugging Face model deploy, site assistant, file editor AI chat |
| 🏪 **Software** | App store, installed apps, extension marketplace, online config & install logs |
| 🖥️ **System** | SSH terminal, PAM bastion, toolbox, users & permissions, settings & online update |

Built with **Go** + **Vue 3**.

---

## 📸 Screenshots

### 📊 Dashboard

Real-time resource monitoring, health score, traffic map, and one-click control for installed services.

<p align="center">
  <img src="https://github.com/luuuunet/owpanel/raw/main/docs/images/ss1.png" alt="OWPanel dashboard" width="920" />
</p>

### 📋 Log center & AI

Aggregates panel, system, site, CDN, and WAF logs — with AI analysis and fix suggestions.

<p align="center">
  <img src="docs/images/log-center-ai.png" alt="Log Center with AI assistant" width="920" />
</p>

---

## ⚡ Quick install

One line — downloads a **prebuilt binary** (~16 MB, ~1–2 minutes on a 1 GB VPS):

```bash
curl -fsSL https://raw.githubusercontent.com/luuuunet/owpanel/v0.1.15/scripts/install.sh | sudo bash
```

Force build from source (slower on small VPS, ~15–30 minutes):

```bash
FROM_SOURCE=1 curl -fsSL https://raw.githubusercontent.com/luuuunet/owpanel/v0.1.15/scripts/install.sh | sudo bash
```

Or clone the repo and install:

```bash
git clone https://github.com/luuuunet/owpanel.git
cd owpanel
sudo bash scripts/install.sh
```

### ✅ After install

| Item | Default |
|------|---------|
| 🌐 Web UI | `http://YOUR_SERVER_IP:8888` |
| 👤 Username | `admin` |
| 🔑 Password | `data/INITIAL_CREDENTIALS.txt` under install directory |
| 📂 Install path | `/opt/owpanel` |
| 🔧 Service | `systemctl status owpanel` |

### ⌨️ CLI (on server)

```bash
op          # interactive menu
op info     # panel info
op config   # edit config
op restart  # restart service
op update   # check/apply panel update
```

### 🔄 Upgrade from Open Panel

If you previously installed **Open Panel** (`/opt/open-panel`), re-run the install script or keep existing data:

```bash
export OWPANEL_DATA=/opt/open-panel/data
export OWPANEL_WEB=/opt/open-panel/web
```

Legacy `OPEN_PANEL_*` environment variables are still supported.

---

## 📚 Documentation

### 📘 English

| Doc | Description |
|-----|-------------|
| [English User Guide](docs/en/USER_GUIDE.md) | Full module reference, permissions & security |
| [Storage Lifecycle (EN)](docs/en/LIFECYCLE.md) | Log rotation, OSS expiry, disaster recovery |
| [Automation Guide (EN)](docs/en/AUTOMATION.md) | Cloud comparison, presets, migration |
| [Rust Runtime (EN)](docs/en/RUST.md) | Install, runtimes, PM2/Docker |

### 📖 中文手册

| 文档 | 说明 |
|------|------|
| [中文用户手册](docs/zh-CN/USER_GUIDE.md) | 全模块功能说明、权限、安全与常见问题 |
| [自动化指南（小白版）](docs/zh-CN/AUTOMATION.md) | 与宝塔/1Panel/云厂商对比、一键预设、迁移对照 |
| [云厂商整合指南](docs/zh-CN/CLOUD.md) | OSS/DNS/备份/监控多云接入 |
| [存储生命周期与云备份](docs/zh-CN/LIFECYCLE.md) | 日志轮转、OSS 过期、面板云备份、灾难恢复 |
| [WordPress 搜索引擎推送](docs/zh-CN/WORDPRESS_SEO.md) | Google、Bing、IndexNow、百度等 |
| [Rust 运行环境](docs/zh-CN/RUST.md) | 安装、PM2/Docker、AI 部署 |

🗂️ Full index: [docs/README.md](docs/README.md)

---

## 📄 License

[MIT License](LICENSE)
