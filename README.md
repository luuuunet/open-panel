<h1 align="center">OWPanel</h1>

<p align="center">
  <strong>🌐 开源自托管 · 🔓 去中心化 · 🤖 自动化 Linux 服务器管理面板</strong>
</p>

<p align="center">
  <a href="https://github.com/luuuunet/owpanel/blob/main/LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
  <a href="https://github.com/luuuunet/owpanel"><img src="https://img.shields.io/badge/language-Go-green.svg" alt="Language"></a>
  <a href="https://github.com/luuuunet/owpanel"><img src="https://img.shields.io/badge/frontend-Vue3-brightgreen.svg" alt="Frontend"></a>
</p>

<p align="center">
  <a href="#-快速安装">⚡ 快速安装</a> ·
  <a href="docs/README.md">📚 文档中心</a> ·
  <a href="https://github.com/luuuunet/owpanel">🔗 GitHub 仓库</a>
</p>

---

**OWPanel** 是面向 Linux 服务器的开源自托管运维面板。数据留在你的机器上，不绑定厂商云端账号；通过 Web 界面统一管理网站、数据库、Docker、安全、备份与自动化运维。

> 📦 原项目名 **Open Panel**，现仓库：[github.com/luuuunet/owpanel](https://github.com/luuuunet/owpanel)

## ✨ 产品特点

- 🛡️ **自托管 / 去中心化** — 单二进制部署，无需注册第三方面板账号
- 🚀 **开箱即用** — 内嵌 Vue 3 前端，systemd 服务，Linux 一键安装
- ⚡ **轻量高效** — Go 后端，预编译包约 16 MB，1 GB VPS 亦可运行
- 🌍 **多语言界面** — 简体中文 / 繁体中文 / English
- 🔐 **安全加固** — 安全入口、2FA、IP 黑白名单、会话超时、安全响应头
- 📊 **智能运维** — 健康评分、一键优化、内存释放、自动巡检与告警
- 🤖 **AI 辅助**（可选） — 日志分析、终端助手、建站/部署工作流
- 📥 **官方源优先安装** — 软件商店先走 apt/dnf 官方包，失败再从 GitHub 拉取 stack 安装脚本
- 🧩 **可扩展** — 扩展市场卡片式安装，Docker Compose 模板一键部署
- 👥 **子账户权限** — 按模块授权，适合团队分工
- ⌨️ **CLI 工具** — `op` 命令行管理面板配置、服务与更新

## 🛠 功能模块

| 分类 | 功能 |
|------|------|
| 📊 **概览** | 仪表盘、CPU/内存/磁盘/网络监控、健康评分、全球流量地图、一键优化 |
| 🌐 **网站** | 虚拟主机（Nginx/OpenResty）、SSL 证书、伪静态/重定向、WP 工具包、A/B 测试 |
| ⚙️ **运行环境** | PHP 多版本、Node.js / Java / Go / Rust / Python / .NET、PM2 / Docker |
| 🗄️ **数据库** | MySQL/MariaDB、PostgreSQL（含扩展管理）、MongoDB、Redis、备份与恢复 |
| 🐳 **容器** | Docker 容器/镜像/卷/网络、Compose 项目、Portainer 等模板 |
| 📁 **文件** | 在线文件管理、上传下载、回收站、对象存储（OSS）对接 |
| 📧 **邮件与传输** | 邮件服务器（Postfix/Dovecot）、FTP（Pure-FTPd）、DNS 解析管理 |
| 🛡️ **安全** | 防火墙、Nginx WAF、CDN 缓存、Cilium 策略、安全检测、Fail2ban |
| 🤖 **自动化** | 计划任务、面板/网站/数据库备份、可用性监控、自动化运维、DevOps 中心 |
| ☸️ **集群** | 多节点集群代理、Kubernetes 集群管理 |
| 📋 **日志** | 面板/系统/网站/CDN/WAF 日志聚合、AI 日志分析 |
| 🧠 **AI** | AI 中心、Hugging Face 模型部署、建站助手、文件编辑器 AI 对话 |
| 🏪 **软件** | 软件商店、已安装管理、扩展市场、在线配置与安装日志 |
| 🖥️ **系统** | SSH 终端、PAM 堡垒机、系统工具箱、用户与权限、面板设置与在线更新 |

---

## 🌏 Features (English)

**OWPanel** is a self-hosted Linux server control panel. No vendor lock-in — your data stays on your server.

**✨ Highlights**

- 🛡️ Self-hosted single binary · Embedded Vue 3 UI · systemd service
- ⚡ Lightweight Go backend (~16 MB) · runs on 1 GB VPS
- 🌍 i18n: zh-CN / zh-TW / English · security entrance · 2FA · IP lists
- 📊 Smart dashboard, health score, traffic map, auto-ops
- 🤖 Optional AI: log analysis, terminal help, site workflows
- 📥 Install: distro packages first, GitHub stack scripts as fallback
- 🧩 App store, extensions, Compose templates, sub-account RBAC
- ⌨️ CLI: `op info` · `op config` · `op restart` · `op update`

**🛠 Modules:** 📊 dashboard · 🌐 websites & SSL · ⚙️ runtimes · 🗄️ databases · 🐳 Docker & Compose · 📁 files & OSS · 📧 mail/FTP/DNS · 🛡️ firewall/WAF/cache · 🤖 cron/backup/uptime · ☸️ cluster/K8s · 📋 logs & 🧠 AI Hub · 🏪 software store · 🖥️ SSH/PAM

Built with **Go** + **Vue 3**.

---

## 📸 界面预览

### 📊 仪表盘

实时资源监控、健康评分、流量地图，以及已安装服务的一键启停。

<p align="center">
  <img src="https://github.com/luuuunet/owpanel/raw/main/docs/images/ss1.png" alt="OWPanel dashboard" width="920" />
</p>

### 📋 日志中心与 AI

聚合面板、系统、网站、CDN、WAF 日志，支持 AI 分析错误并给出修复建议。

<p align="center">
  <img src="docs/images/log-center-ai.png" alt="Log Center with AI assistant" width="920" />
</p>

---

## ⚡ 快速安装

一行命令，下载**预编译二进制**（约 16 MB，1 GB VPS 上约 1–2 分钟）：

```bash
curl -fsSL https://raw.githubusercontent.com/luuuunet/owpanel/v0.1.15/scripts/install.sh | sudo bash
```

强制从源码编译（小内存 VPS 较慢，约 15–30 分钟）：

```bash
FROM_SOURCE=1 curl -fsSL https://raw.githubusercontent.com/luuuunet/owpanel/v0.1.15/scripts/install.sh | sudo bash
```

或克隆仓库后安装：

```bash
git clone https://github.com/luuuunet/owpanel.git
cd owpanel
sudo bash scripts/install.sh
```

### ✅ 安装后

| 项目 | 默认值 |
|------|--------|
| 🌐 Web 界面 | `http://服务器IP:8888` |
| 👤 用户名 | `admin` |
| 🔑 密码 | 安装目录下 `data/INITIAL_CREDENTIALS.txt` |
| 📂 安装路径 | `/opt/owpanel` |
| 🔧 服务管理 | `systemctl status owpanel` |

### ⌨️ CLI（服务器上）

```bash
op          # 交互菜单
op info     # 面板信息
op config   # 编辑配置
op restart  # 重启服务
op update   # 检查/应用面板更新
```

### 🔄 从 Open Panel 升级

若曾安装 **Open Panel**（路径 `/opt/open-panel`），可重新执行安装脚本，或保留数据：

```bash
export OWPANEL_DATA=/opt/open-panel/data
export OWPANEL_WEB=/opt/open-panel/web
```

仍兼容旧环境变量 `OPEN_PANEL_*`。

---

## 📚 文档

| 文档 | 说明 |
|------|------|
| 📖 [中文用户手册](docs/zh-CN/USER_GUIDE.md) | 全模块功能说明、权限与安全 |
| 📘 [English User Guide](docs/en/USER_GUIDE.md) | Full module reference |
| 🗂️ [文档索引](docs/README.md) | 自动化、云厂商、生命周期等专题 |
| ☁️ [存储生命周期与云备份](docs/zh-CN/LIFECYCLE.md) | 日志轮转、OSS、灾难恢复 |

---

## 📄 License

[MIT License](LICENSE)
