<h1 align="center">Open Panel</h1>

<p align="center">
  Open-source Linux server management panel
</p>

<p align="center">
  <img src="docs/images/dashboard.png" alt="Open Panel dashboard" width="900" />
</p>

<p align="center"><em>Dashboard preview</em></p>

---

Open Panel is a self-hosted web panel for managing Linux servers — websites, databases, Docker, security, and more from one place.

### Highlights

- **Dashboard** — Live metrics, health score, and global traffic map
- **Websites** — Nginx vhosts, SSL, CDN cache, WordPress toolkit
- **Databases** — MySQL, PostgreSQL, Redis, backup and restore
- **Docker** — Containers, images, and one-click Compose apps
- **Security** — Firewall, Nginx WAF, login audit, 2FA
- **SSH / AI** — Web terminal with AI assistant and asset management

Built with **Go** (backend) and **Vue 3** (frontend). Single binary, embedded UI, systemd service.

### Install

Ubuntu, Debian, CentOS, Rocky, or AlmaLinux:

```bash
curl -fsSL https://raw.githubusercontent.com/luuuunet/open-panel/main/scripts/install.sh | sudo bash
```

Or clone and install from source:

```bash
git clone https://github.com/luuuunet/open-panel.git
cd open-panel
sudo bash scripts/install.sh
```

Open `http://YOUR_SERVER_IP:8888` — default user `admin`, password in `data/INITIAL_CREDENTIALS.txt` on the server.

### Documentation

Full user guide: [docs/en/USER_GUIDE.md](docs/en/USER_GUIDE.md)

### License

[MIT License](LICENSE)
