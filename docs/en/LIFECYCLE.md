# Storage Lifecycle & Cloud-Native Backup

> Quick links:
> - **Settings → Migration** — Panel cloud backup / restore from cloud
> - **Automation → Backup** — Scheduled tasks, local cleanup
> - **Files → Object Storage** — OSS lifecycle, large-file archive
> - **Logs** — Rotation, compression, retention

OWPanel bundles log growth, backup disk usage, unbounded OSS objects, and missing off-site panel config into **configurable lifecycle management** — similar to cloud object ILM + snapshot schedules, but **self-hosted** in one panel.

Inspired by documentation patterns from:

| Project | What we borrow |
|---------|----------------|
| [Openpanel](https://github.com/Openpanel-dev/openpanel) | Open-source, self-hosted, feature comparison tables |
| [Xboard](https://github.com/cedar2025/Xboard) | Panel ops docs, **backup before upgrade**, deployment flows |

---

## Features

- **Panel cloud backup** — Export `panel.db`, sites, SSL, Nginx, etc. to S3/MinIO on schedule
- **Restore from cloud** — Pull migration bundle from OSS; replace or merge import
- **Log rotation** — Size-based rename chain + optional gzip + age cleanup
- **Local expiry rules** — Presets for logs, migration staging, old bundles
- **OSS lifecycle** — Prefix + max-age deletion with dry-run and keep-min count
- **Large-file archive** — Upload files over threshold to object storage; optional local delete
- **Backup remote sync** — Deleting local website/DB backups also deletes OSS objects

---

## Comparison

| Capability | OWPanel | Cloud OSS ILM | Baota / 1Panel | Typical panels |
|------------|---------|---------------|----------------|----------------|
| Off-site panel config | Yes | DIY scripts | Manual export | DB backup habit |
| Site/DB backup + OSS | Yes + linked delete | Storage-side only | Plugins / cron | Cron |
| Log rotate + compress | Built-in | No | logrotate manual | Cron |
| Panel-side OSS expiry | Yes | Native ILM | No | No |
| Self-hosted data control | Yes | Cloud account | Yes | Yes |

---

## Quick start

### 1. Add object storage

**Files → Object Storage → Add storage** — MinIO, Aliyun OSS, Tencent COS, AWS S3, GCS, IBM COS, or local path.

### 2. Enable panel cloud backup

**Settings → Migration → Cloud backup**

1. Pick **OSS endpoint**
2. Set **Cron** (default `0 4 * * *`)
3. Set **keep count** (default 5)
4. **Backup to cloud now** or **Save schedule**

Object key: `backups/panel/owpanel-migration-{timestamp}.tar.gz`

> **Back up before upgrades** (Xboard-style): always cloud-backup or download a migration bundle before major upgrades or restore operations.

### 3. Site / database backups with OSS

**Automation → Backup** — Templates: all sites (2:00), all DBs (3:00), panel (4:00). Optional **default OSS** on template row.

### 4. Log rotation

**Logs → Retention** — Suggested: 7–30 days, auto cleanup on, rotate at 50MB, 5 copies, compress on.

### 5. OSS lifecycle (optional)

**Files → Object Storage → Lifecycle** — Example: prefix `backups/panel/`, max age 90 days, keep min 3. Use **dry-run** first.

---

## API

### Panel backup

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/backup/panel/config` | Get schedule config |
| PUT | `/api/backup/panel/config` | Save schedule config |
| POST | `/api/backup/panel/run` | Backup now + upload |
| GET | `/api/backup/panel/history` | History list |
| POST | `/api/backup/panel/restore` | Restore from cloud |

### Local cleanup

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/lifecycle/local-rules` | List rules |
| POST | `/api/lifecycle/local-rules` | Create rule |
| POST | `/api/lifecycle/local-rules/:id/run` | Run now |

### OSS lifecycle & archive

| Method | Path | Description |
|--------|------|-------------|
| GET/POST/PUT/DELETE | `/api/oss/lifecycle-rules` | Expiry rules |
| POST | `/api/oss/lifecycle-rules/:id/dry-run` | Preview deletes |
| GET/POST/PUT/DELETE | `/api/oss/archive-rules` | Archive rules |

---

## Schedulers

- **Every 15 min** — Auto backups including `type=panel`
- **Daily** — Log rotation + cleanup, local rules, OSS lifecycle, archive rules

Overview cards: **Automation → Auto Ops → Overview** (last panel cloud backup, log rotation, OSS rule counts).

---

## Related docs

- [Cloud integration (zh)](../zh-CN/CLOUD.md)
- [Automation (beginner)](./AUTOMATION.md)
- [User guide](./USER_GUIDE.md)
