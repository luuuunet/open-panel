# 存储生命周期与云原生备份

> 入口速查：
> - **设置 → 一键迁移** — 面板云备份 / 从云端恢复
> - **自动化 → 备份** — 定时任务、本地清理
> - **文件 → 对象存储** — OSS 生命周期、大文件归档
> - **日志** — 轮转压缩与保留策略

OWPanel 将「日志膨胀、备份占盘、OSS 只增不删、面板配置无异地副本」等运维痛点，整合为**可配置的生命周期编排**——类似云厂商对象存储 ILM + 快照计划，但在**自托管面板**里一键完成，数据仍在你自己的服务器与桶里。

参考产品思路：

| 产品 | 借鉴点 |
|------|--------|
| [Openpanel](https://github.com/Openpanel-dev/openpanel) | 开源、自托管、数据自主；功能用对比表讲清价值 |
| [Xboard](https://github.com/cedar2025/Xboard) | 面板级运维文档、升级前务必备份、Docker/多云部署指引 |

---

## ✨ 功能一览

- **📦 面板云原生备份**：一键将 `panel.db`、网站、SSL、Nginx 等打包上传 S3/MinIO，支持定时与保留份数
- **☁️ 从云端恢复**：从 OSS 拉取迁移包，完整恢复或合并导入（复用迁移安全校验）
- **🔄 日志自动轮转**：超阈值 rename 链 + 可选 gzip，配合按天清理
- **🗑️ 本地过期清理**：预设规则清理日志目录、迁移临时文件、旧迁移包
- **⏳ OSS 生命周期**：按前缀 + 天数删除远程对象，支持 dry-run 与最少保留份数
- **📤 大文件自动归档**：超大 zip/sql/log 上传对象存储，可选上传后删本地释放 SSD
- **🔗 备份远程联动**：网站/数据库备份删除时同步删除 OSS 对象，避免「本地删了、云上还在」

---

## 📊 能力对照（OWPanel vs 云厂商 vs 常见面板）

| 能力 | OWPanel | 阿里云 OSS ILM | AWS S3 Lifecycle | 宝塔 / 1Panel | Xboard 类面板 |
|------|---------|----------------|----------------|---------------|---------------|
| 面板配置异地备份 | ✅ 一键 OSS | ❌ 需自建脚本 | ❌ 需自建脚本 | ⚠️ 多为手动导出 | ⚠️ 依赖 DB 备份习惯 |
| 网站/库备份 + OSS | ✅ 定时 + 联动删除 | ⚠️ 仅存储侧规则 | ⚠️ 仅存储侧规则 | ✅ 插件/计划任务 | ✅ 计划任务 |
| 日志轮转 + 压缩 | ✅ 内置 | ❌ | ❌ | ⚠️ logrotate 手配 | ⚠️ 需 Cron |
| OSS 前缀过期（面板侧） | ✅ Walk + 规则 | ✅ 原生 ILM | ✅ 原生 ILM | ❌ | ❌ |
| 大文件冷归档 | ✅ 阈值触发 | ✅ 生命周期转冷 | ✅ Intelligent-Tiering | ❌ | ❌ |
| 自托管 / 数据自主 | ✅ | ❌ 绑云账号 | ❌ 绑云账号 | ✅ | ✅ |

> **说明：** OWPanel 的 OSS 规则在面板内执行（兼容 MinIO/各云 S3），不依赖单一云控制台；若桶已配置原生 ILM，可与面板规则并存，注意勿重复删除。

---

## 🚀 快速开始（5 分钟）

### 1. 接入对象存储

路径：**文件 → 对象存储 → 添加存储**

支持：本机目录、MinIO、阿里云 OSS、腾讯云 COS、AWS S3、Google Cloud Storage、IBM COS 等 S3 兼容端点。

填写 Bucket、AccessKey/SecretKey 后 **连接测试** 通过即可。

### 2. 开启面板云备份（灾难恢复）

路径：**设置 → 一键迁移 / 导出全部数据 → 云原生备份**

1. 选择 **OSS 存储端点**
2. 设置 **Cron**（默认 `0 4 * * *` 每天 4:00）
3. 设置 **保留份数**（默认 5）
4. 点击 **立即备份到云端** 或 **保存定时设置**

备份对象 key：`backups/panel/owpanel-migration-{时间}.tar.gz`

> ⚠️ **升级 / 大改前请先云备份**（借鉴 [Xboard 升级须知](https://github.com/cedar2025/Xboard)）：面板升级、迁移、从云端恢复前，务必先完成一次云备份或本地下载迁移包。

### 3. 开启网站/数据库备份并上传 OSS

路径：**自动化 → 备份**

- 快捷模板：**备份全部网站**（2:00）、**备份全部数据库**（3:00）、**面板云备份**（4:00）
- 模板上方可选 **默认 OSS 上传**

本地 prune 时会**同步删除**对应 OSS 对象（需为新产生的备份记录，历史备份无 `remote_key` 的需手动或靠 OSS 生命周期规则清理）。

### 4. 配置日志轮转

路径：**日志 → 保留策略**

| 项 | 建议 |
|----|------|
| 保留天数 | 7–30 |
| 自动清理 | 开启 |
| 轮转阈值 (MB) | 50 |
| 保留份数 | 5 |
| 压缩轮转 | 开启 |

### 5. OSS 生命周期（可选）

路径：**文件 → 对象存储 → 生命周期**

示例规则：

| 名称 | 前缀 | 过期天数 | 最少保留 |
|------|------|----------|----------|
| 清理旧面板备份 | `backups/panel/` | 90 | 3 |
| 清理旧站点备份 | `backups/` | 60 | 5 |

先点 **试运行（dry-run）** 确认将删除的对象，再正式执行。

---

## 📖 功能详解

### 面板云原生备份

**包含内容**（与手动迁移导出一致）：

- SQLite `panel.db`
- 网站根目录、SSL、Nginx 配置、扩展、邮件数据等（见迁移预览）
- 可选包含日志目录（体积较大）

**定时任务**：备份页 `type: panel` 任务，由 `startAutoBackupLoop`（每 15 分钟检查 Cron）调度。

**恢复**：

1. 设置页云备份历史 → **从云端恢复**
2. 选择 **完整恢复（replace）** 或 **合并（merge）**
3. 确认后自动拉取 bundle 并 `migration.Import`
4. 按提示 **重启 OWPanel**；若 JWT 密钥变更，用户需重新登录

### 日志轮转与清理

执行顺序（每日）：

1. `RotateOversizedLogs` — 超阈值 active 日志 → `.1` … `.N`，最旧可 gzip
2. `CleanOlderThan` — 删除超保留天数的轮转文件 / 裁剪大行数日志

### 本地清理规则

路径：**自动化 → 备份 → 本地清理**

内置预设：

| 预设 | 作用 |
|------|------|
| 面板日志目录 | 清理 `data/logs` 下过旧文件 |
| 迁移临时目录 | 清理 `panel-migration/staging-*` 残留 |
| 旧迁移包 | 清理超龄的 `owpanel-migration-*.tar.gz` |

路径限制在 `dataDir`、备份目录、网站根等安全范围内。

### 大文件归档

路径：**文件 → 对象存储 → 大文件归档**

适用场景：

- 网站备份 zip 超过 100MB → `archives/site-backups/`
- 数据库 dump → `archives/db-backups/`
- 轮转后的 `.gz` 日志 → `archives/logs/`

**删除本地文件** 默认关闭；开启前请确认已有远程副本。

---

## 🔌 API 参考

权限：与备份 / 文件（OSS）模块一致，需管理员及对应权限组。

### 面板云备份

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/backup/panel/config` | 获取定时配置 |
| PUT | `/api/backup/panel/config` | 保存定时配置 |
| POST | `/api/backup/panel/run` | 立即备份并上传 |
| GET | `/api/backup/panel/history` | 备份历史 |
| POST | `/api/backup/panel/restore` | 从云端恢复 |

`POST /api/backup/panel/run` 请求体示例：

```json
{
  "oss_storage_id": 1,
  "include_logs": false,
  "keep_count": 5
}
```

`POST /api/backup/panel/restore` 请求体示例：

```json
{
  "record_id": 12,
  "mode": "replace"
}
```

### 本地清理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/lifecycle/local-rules` | 规则列表 |
| GET | `/api/lifecycle/local-rules/presets` | 预设模板 |
| POST | `/api/lifecycle/local-rules` | 创建规则 |
| PUT | `/api/lifecycle/local-rules/:id` | 更新规则 |
| DELETE | `/api/lifecycle/local-rules/:id` | 删除规则 |
| POST | `/api/lifecycle/local-rules/:id/run` | 立即执行 |

### OSS 生命周期与归档

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/POST/PUT/DELETE | `/api/oss/lifecycle-rules` | OSS 过期规则 CRUD |
| POST | `/api/oss/lifecycle-rules/:id/run` | 执行规则 |
| POST | `/api/oss/lifecycle-rules/:id/dry-run` | 试运行 |
| GET/POST/PUT/DELETE | `/api/oss/archive-rules` | 大文件归档 CRUD |
| POST | `/api/oss/archive-rules/:id/run` | 立即归档 |

---

## 🏗️ 架构与调度

```text
每日 startLifecycleLoop
  ├── logs.RotateOversizedLogs
  ├── lifecycle.RunDueRules        # 本地清理
  ├── ossstorage.RunDueLifecycleRules
  └── ossstorage.RunDueArchiveRules

每 15 分钟 startAutoBackupLoop
  ├── 网站/数据库自动备份
  └── RunDueBackupTasks（含 type=panel）

每日 startLogCleanupLoop
  ├── RotateOversizedLogs
  └── CleanOlderThan(retention_days)
```

运维概览（**自动化 → 自动化运维 → 运维概览**）展示：最近面板云备份时间、日志轮转状态、OSS 生命周期/归档规则数量。

---

## 💡 推荐方案

### 单机小 VPS（磁盘紧张）

1. 日志：轮转 50MB + 保留 7 天 + 压缩
2. 备份：网站/库每日本地 5 份 + OSS 上传
3. OSS 规则：`backups/` 保留 30 天、最少 3 份
4. 面板：每周云备份 1 份，保留 4 份

### 已有 MinIO / 多云桶

1. 文件 → 对象存储 添加 MinIO
2. 云厂商整合页一键迁移包（可选关联 OSS）
3. 大文件归档：备份目录 → `archives/`，**勿开**删除本地（除非盘位极紧）
4. 跨云：用同步/迁移任务做桶间复制（见 [云厂商整合指南](./CLOUD.md)）

### 升级 / 迁机（Xboard 式流程）

1. **升级前**：设置页 → 立即云备份 + 本地下载迁移包
2. **新机器**：安装 OWPanel → 设置页 → 从云端恢复 或 上传迁移包导入
3. **验证**：`systemctl restart owpanel`，检查网站、SSL、计划任务
4. **旧机**：确认新环境正常后再下线

---

## ❓ 常见问题

**Q：云备份和本地下载迁移包有什么区别？**  
A：内容基本一致。云备份多了一步自动上传 OSS 与历史记录管理；本地下载适合离线保管。

**Q：开启 OSS 上传后，远程空间还会无限涨吗？**  
A：新产生的网站/库备份会联动删除远程 key；面板云备份按保留份数 prune；也可用 OSS 生命周期规则兜底。

**Q：dry-run 和规则里的「试运行」有什么区别？**  
A：规则可常驻 `dry_run: true` 只预览；单次「试运行」接口强制预览不写删除。

**Q：从云端恢复会覆盖什么？**  
A：`replace` 覆盖 `panel.db` 与 data 下同名文件，导入前会自动做 pre-import 快照；`merge` 不替换数据库。

**Q：和 Openpanel / Xboard 的关系？**  
A：OWPanel 是 **Linux 服务器控制面板**（网站、Docker、备份、OSS），与 [Openpanel 分析平台](https://github.com/Openpanel-dev/openpanel)、[Xboard 业务面板](https://github.com/cedar2025/Xboard) 产品定位不同；本文档仅借鉴其**文档结构与「升级前备份」运维习惯**。

---

## 相关文档

- [云厂商整合指南](./CLOUD.md) — OSS 接入、一键迁移包、跨云同步
- [自动化指南](./AUTOMATION.md) — 建站保护、备份模板、与云产品对照
- [用户手册](./USER_GUIDE.md) — 全模块说明
