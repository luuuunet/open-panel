# OWPanel User Guide

Complete reference for administrators, operators, and authorized sub-users.

---

## Table of Contents

1. [Overview](#1-overview)
2. [Login & UI](#2-login--ui)
3. [Dashboard](#3-dashboard)
4. [Websites & Runtimes](#4-websites--runtimes)
5. [Databases](#5-databases)
6. [Docker & Compose](#6-docker--compose)
7. [Files & OSS](#7-files--oss)
8. [FTP & Mail](#8-ftp--mail)
9. [Automation & Monitoring](#9-automation--monitoring)
10. [Protection Center](#10-protection-center)
11. [SSH / PAM](#11-ssh--pam)
12. [Toolbox](#12-toolbox)
13. [AI Hub](#13-ai-hub)
14. [App Store & Extensions](#14-app-store--extensions)
15. [Logs & DNS](#15-logs--dns)
16. [Users & Permissions](#16-users--permissions)
17. [Panel Settings](#17-panel-settings)
18. [Data Directory & CLI](#18-data-directory--cli)
19. [FAQ](#19-faq)

---

## 1. Overview

OWPanel is an open-source Linux server management panel with a modern Vue 3 UI and Go backend (single binary + embedded frontend).

**Core capabilities:** websites, SSL, CDN cache, databases, Docker, backups, cron, firewall, WAF, SSH terminal with PAM, traffic analytics, sub-user permissions, i18n (Simplified / Traditional Chinese / English).

---

## 2. Login & UI

| Scenario | URL |
|----------|-----|
| Default | `http://SERVER_IP:8888` |
| Security entrance enabled | `http://IP:8888/<entrance-path>/` |

**First login:** username `admin`, random password in `{dataDir}/INITIAL_CREDENTIALS.txt` or service logs.

**2FA:** enable TOTP under **Panel Settings → Security**.

**Language:** bottom-right on login page, or **Panel Settings → Language** after login.

**Theme:** light / dark / system; dark mode supports multiple color variants.

---

## 3. Dashboard

- Real-time CPU, memory, disk, network metrics and trends
- **Health score** with actionable recommendations
- **One-click optimize:** safe cleanup (page cache, swap, Docker/log/package cache) with step-by-step results
- **Free memory:** drop Linux page cache
- **Traffic map:** geo distribution; drill down country → domain → paths/referrers/IPs; geo block or 301 redirect policies

---

## 4. Websites & Runtimes

| Menu | Features |
|------|----------|
| **Websites** | Nginx/OpenResty vhosts, domains, PHP/proxy, rewrites |
| **WP Toolkit** | WordPress install, migration, full backup, [search engine push](./WORDPRESS_SEO.md) |
| **Runtimes** | Node.js / Java project management |
| **SSL** | Let's Encrypt, manual certs, deploy to sites |
| **PHP** | versions, extensions, config |

---

## 5. Databases

MySQL/MariaDB, PostgreSQL (with extensions), Redis:

- create DB/users, grants, remote access
- backup, import, SQL execution
- OSS remote backup, phpMyAdmin (admin)

---

## 6. Docker & Compose

- **Docker:** containers, images, volumes, networks, logs
- **Compose:** template-based multi-service deploy, project management

---

## 7. Files & OSS

- **Files:** browse, upload, archive, online code editor; disk quota for sub-users
- **OSS:** S3-compatible storage, sync, backup targets

---

## 8. FTP & Mail

- **FTP:** Pure-FTPd virtual users tied to site paths
- **Mail:** Postfix/Dovecot virtual mailboxes

---

## 9. Automation & Monitoring

> **Beginner guide with cloud comparison:** see [AUTOMATION.md](./AUTOMATION.md)

| Module | Purpose |
|--------|---------|
| **Auto Ops** | Hub: comparison vs aaPanel/1Panel/Alibaba/AWS/GCP, one-click site protection, service watch, website audit, webhooks |
| **Cron** | Scheduled tasks, built-in templates, logs, manual run |
| **Backup** | Sites, DBs, paths → local / FTP / SFTP / WebDAV / OSS; quick templates **backup all sites/DBs** |
| **Uptime** | HTTP(S) probes; **import from websites** one-click |
| **Cluster** | Multi-node agents, centralized status |
| **DevOps** | CI/CD hooks (admin) |

---

## 10. Protection Center

Single hub (**Automation → Cache & Security**) with tabs:

- **CDN cache** — rules, purge, warm-up
- **Firewall** — ufw/firewalld sync (Linux)
- **Nginx / OpenResty** — global and site config
- **WAF** — custom rules, block logs
- **Edge Workers / Kafka / Cilium** — advanced (when installed)
- **Security** — login logs, IP access, password policy, security score, scans

---

## 11. SSH / PAM

Menu: **SSH / PAM** (standalone Bastion menu merged here).

### SSH Terminal tab

- Multi-tab Web SSH
- Quick connect: localhost, cluster nodes, authorized PAM assets
- Password / SSH keys, saved connections, AI assistant
- Shortcuts: `Ctrl+Shift+T` new tab, `Ctrl+Shift+W` close, `F11` fullscreen

### PAM Management tab

| Tab | Features |
|-----|----------|
| Assets | host inventory, groups, cluster import, known hosts |
| Access requests | JIT access with approval workflow |
| Permissions | user–asset grants, command policies |
| Audit | session replay, download, command extraction |
| Active sessions | kill live connections |
| Accounts | privileged accounts, rotation, vault import/export |
| Ops center | ad-hoc commands, templates, scheduled jobs |

Legacy URL `/bastion` redirects to `/terminal?tab=pam`.

Sub-users need **SSH / PAM** permission; management tabs are mostly admin-only.

---

## 12. Toolbox

Admin-only: system overview, ping/traceroute/DNS, port/process list (add firewall rule), command snippets, drop cache.

---

## 13. AI Hub

Admin-only: AI app management; configure provider in **Panel Settings → AI** (OpenAI-compatible, Ollama, etc.).

---

## 14. App Store & Extensions

- **App Store:** one-click Nginx, MySQL, PHP, Redis, PostgreSQL, LNMP/LAMP readiness
- **Extensions:** third-party embedded modules (admin)

---

## 15. Logs & DNS

- **Log center** (admin): panel, site, WAF, system logs
- **DNS** (admin): record management

---

## 16. Users & Permissions

| Role | Access |
|------|--------|
| admin | full |
| user | all non-admin items |
| subuser | per-module permissions only |

**Sub-user modules:** `websites`, `databases`, `files`, `docker`, `ftp`, `mail`, `backup`, `monitor`, `bastion` (SSH / PAM).

**Admin-only examples:** software install, firewall/WAF write, DNS, logs, DevOps, AI, toolbox, settings, user management, most PAM admin features.

Optional **disk quota** (MB) per sub-user.

---

## 17. Panel Settings

Panel name, port, security entrance, panel SSL, login captcha, session timeout, backup/website paths, language, theme, AI keys, 2FA.

---

## 18. Data Directory & CLI

Default `{dataDir}` = `/opt/owpanel/data`:

| Path | Purpose |
|------|---------|
| `wwwroot/` | site roots |
| `server/` | panel-managed apps |
| `apps/` | Docker apps |
| `logs/` | logs |
| `backup/` | local backups |

```bash
op info      # URLs, port, data dir
op config    # port, entrance, SSL
op restart   # restart service
```

Install/deploy details: [README.md](../../README.md).

---

## 19. FAQ

**404 on login?** Check security entrance path in URL.

**Sub-user missing SSH?** Grant **SSH / PAM** permission.

**Cron not running?** Enable task, check logs and scheduler status on Linux.

**Backup fails?** Verify credentials, paths, disk space.

---

*Documentation reflects the current codebase; UI may vary slightly by version.*
