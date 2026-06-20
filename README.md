<h1 align="center">OWPanel</h1>

<p align="center">
  <a href="https://github.com/luuuunet/owpanel">GitHub</a> ·
  Self-hosted · Decentralized · Automated Linux server management
</p>

---

**OWPanel** is a self-hosted control plane for your own Linux servers. No vendor lock-in, no central cloud — you run the panel on your machine, and your data stays on your machine. Built-in automation handles backups, cron jobs, service restarts, log analysis, and AI-assisted ops.

> Formerly **Open Panel** — same project, new name and repository: [github.com/luuuunet/owpanel](https://github.com/luuuunet/owpanel)

### Highlights

- **Decentralized by design** — Single binary on your server; no external account required
- **Automated operations** — Scheduled backups, cron templates, one-click service control, auto log cleanup
- **Smart dashboard** — Live metrics, health score, traffic map, and running-service overview
- **AI-assisted ops** — Log analysis, SSH terminal help, and site/project workflows
- **Full stack control** — Websites, databases, Docker, firewall/WAF, FTP, mail, DNS from one UI

Built with **Go** + **Vue 3**. Embedded web UI, systemd service, Linux only.

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
- [Docs index](docs/README.md)

### License

[MIT License](LICENSE)
