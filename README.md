<h1 align="center">OWPanel</h1>

<p align="center">
  <a href="https://github.com/luuuunet/owpanel">GitHub</a> ·
  Self-hosted · Decentralized · Automated Linux server management
</p>

---

**OWPanel** 是面向 Linux 服务器的开源自托管运维面板。数据留在你的机器上，不绑定厂商云端账号；通过 Web 界面统一管理网站、数据库、Docker、安全、备份与自动化运维。

> 原项目名 **Open Panel**，现仓库：[github.com/luuuunet/owpanel](https://github.com/luuuunet/owpanel)

### 产品特点

- **自托管 / 去中心化** — 单二进制部署，无需注册第三方面板账号
- **开箱即用** — 内嵌 Vue 3 前端，systemd 服务，Linux 一键安装
- **轻量高效** — Go 后端，预编译包约 16 MB，1 GB VPS 亦可运行
- **多语言界面** — 简体中文 / 繁体中文 / English
- **安全加固** — 安全入口、2FA、IP 黑白名单、会话超时、安全响应头
- **智能运维** — 健康评分、一键优化、内存释放、自动巡检与告警
- **AI 辅助**（可选） — 日志分析、终端助手、建站/部署工作流
- **官方源优先安装** — 软件商店先走 apt/dnf 官方包，失败再从 GitHub 拉取 stack 安装脚本
- **可扩展** — 扩展市场卡片式安装，Docker Compose 模板一键部署
- **子账户权限** — 按模块授权，适合团队分工
- **CLI 工具** — `op` 命令行管理面板配置与服务

### 功能模块

| 分类 | 功能 |
|------|------|
| **概览** | 仪表盘、CPU/内存/磁盘/网络监控、健康评分、全球流量地图、一键优化 |
| **网站** | 虚拟主机（Nginx/OpenResty）、SSL 证书、伪静态/重定向、WP 工具包、A/B 测试 |
| **运行环境** | PHP 多版本、Node.js / Java / Go / Rust / Python / .NET、PM2 / Docker |
| **数据库** | MySQL/MariaDB、PostgreSQL（含扩展管理）、MongoDB、Redis、备份与恢复 |
| **容器** | Docker 容器/镜像/卷/网络、Compose 项目、Portainer 等模板 |
| **文件** | 在线文件管理、上传下载、回收站、对象存储（OSS）对接 |
| **邮件 & 传输** | 邮件服务器（Postfix/Dovecot）、FTP（Pure-FTPd）、DNS 解析管理 |
| **安全** | 防火墙、Nginx WAF、CDN 缓存、Cilium 策略、安全检测、Fail2ban |
| **自动化** | 计划任务、面板/网站/数据库备份、可用性监控、自动化运维、DevOps 中心 |
| **集群** | 多节点集群代理、Kubernetes 集群管理 |
| **日志** | 面板/系统/网站/CDN/WAF 日志聚合、AI 日志分析 |
| **AI** | AI 中心、Hugging Face 模型部署、建站助手、文件编辑器 AI 对话 |
| **软件** | 软件商店、已安装管理、扩展市场、在线配置与安装日志 |
| **系统** | SSH 终端、PAM 堡垒机、系统工具箱、用户与权限、面板设置与在线更新 |

---

### Features (English)

**Characteristics**

- Self-hosted, decentralized — single binary, no vendor cloud account
- Embedded Vue 3 UI, systemd service, one-command Linux install
- Lightweight Go backend (~16 MB release), runs on 1 GB VPS
- i18n: Simplified Chinese, Traditional Chinese, English
- Security: entrance path, 2FA, IP allow/deny lists, session timeout
- Smart ops: health score, one-click optimize, auto inspection
- Optional AI: log analysis, terminal help, site bootstrap workflows
- Install strategy: distro official packages first, GitHub stack scripts as fallback
- Extension marketplace, Docker Compose templates, sub-account RBAC
- CLI: `op info`, `op config`, `op restart`, `op update`

**Modules**

| Category | Capabilities |
|----------|--------------|
| **Overview** | Dashboard, metrics, health score, traffic map, service control |
| **Websites** | Vhosts, SSL, WordPress toolbox, A/B analytics |
| **Runtimes** | PHP, Node.js, Java, Go, Rust, Python, .NET |
| **Databases** | MySQL/MariaDB, PostgreSQL, MongoDB, Redis, backups |
| **Containers** | Docker management, Compose stacks, app templates |
| **Files & OSS** | File manager, cloud object storage |
| **Mail & DNS** | Mail server, FTP, DNS records |
| **Security** | Firewall, WAF, CDN cache, Cilium, Fail2ban |
| **Automation** | Cron, backups, uptime monitoring, auto-ops, DevOps |
| **Cluster** | Multi-server cluster agent, Kubernetes |
| **Logs & AI** | Centralized logs, AI analysis, AI Hub |
| **Software** | App store, extensions, install logs, online config |
| **System** | SSH terminal, PAM bastion, toolbox, users, panel update |

Built with **Go** + **Vue 3**.

### Dashboard

Real-time health, resource trends, global traffic, and one place to start/stop/restart every installed service.

<p align="center">
  <img src="https://github.com/luuuunet/owpanel/raw/main/docs/images/ss1.png" alt="OWPanel dashboard" width="920" />
</p>

### Log Center & AI

Centralized logs across panel, system, websites, CDN, and WAF — with **AI analysis** to spot errors and suggest fixes.

<p align="center">
  <img src="docs/images/log-center-ai.png" alt="Log Center with AI assistant" width="920" />
</p>

---

### Install (fast — recommended)

One command. Downloads a **pre-built binary** (~16 MB, **1–2 minutes** on a 1 GB VPS):

```bash
curl -fsSL https://raw.githubusercontent.com/luuuunet/owpanel/v0.1.15/scripts/install.sh | sudo bash
```

Force source build (slow, 15–30 min on small VPS):

```bash
FROM_SOURCE=1 curl -fsSL https://raw.githubusercontent.com/luuuunet/owpanel/v0.1.15/scripts/install.sh | sudo bash
```

Or from a local clone:

```bash
git clone https://github.com/luuuunet/owpanel.git
cd owpanel
sudo bash scripts/install.sh
```

**After install**

| Item | Default |
|------|---------|
| Web UI | `http://YOUR_SERVER_IP:8888` |
| Username | `admin` |
| Password | `data/INITIAL_CREDENTIALS.txt` under install dir |
| Install dir | `/opt/owpanel` |
| Service | `systemctl status owpanel` |

**CLI** (on the server):

```bash
op          # interactive menu
op info     # panel info
op config   # edit config
op restart  # restart service
```

### Upgrade from Open Panel

If you previously installed **Open Panel** under `/opt/open-panel`, re-run the install script or migrate manually:

```bash
# Option A: fresh install to new path (recommended for new servers)
curl -fsSL https://raw.githubusercontent.com/luuuunet/owpanel/v0.1.15/scripts/install.sh | sudo bash

# Option B: keep existing data — set env vars before starting owpanel
export OWPANEL_DATA=/opt/open-panel/data
export OWPANEL_WEB=/opt/open-panel/web
```

Legacy `OPEN_PANEL_*` environment variables are still accepted for compatibility.

### Documentation

- [English User Guide](docs/en/USER_GUIDE.md)
- [中文用户手册](docs/zh-CN/USER_GUIDE.md)
- [存储生命周期与云备份](docs/zh-CN/LIFECYCLE.md) · [Storage Lifecycle (EN)](docs/en/LIFECYCLE.md)
- [Docs index](docs/README.md)

### License

[MIT License](LICENSE)
