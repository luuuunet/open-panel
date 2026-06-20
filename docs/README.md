# OWPanel 文档

OWPanel 完整使用与运维文档。

| 文档 | 说明 |
|------|------|
| [中文用户手册](./zh-CN/USER_GUIDE.md) | 全模块功能说明、权限、安全与常见问题 |
| [自动化指南（小白版）](./zh-CN/AUTOMATION.md) | 与宝塔/1Panel/阿里云/AWS/GCP 对比、一键预设、迁移对照 |
| [WordPress 搜索引擎推送](./zh-CN/WORDPRESS_SEO.md) | 自动/手动提交 Google、Bing、IndexNow、百度等 |
| [云厂商整合指南](./zh-CN/CLOUD.md) | OSS/DNS/备份/监控多云接入与一键迁移包 |
| [存储生命周期与云备份](./zh-CN/LIFECYCLE.md) | 日志轮转、OSS 过期、面板云备份、灾难恢复 |
| [Rust 运行环境](./zh-CN/RUST.md) | 软件商店安装、运行环境、PM2/Docker、AI 部署 |
| [English User Guide](./en/USER_GUIDE.md) | Full module reference (English) |
| [Rust Runtime (English)](./en/RUST.md) | Install, runtimes, PM2/Docker, AI deploy |
| [Automation Guide (beginner)](./en/AUTOMATION.md) | Cloud comparison, presets, migration cheat sheet |
| [Storage Lifecycle & Cloud Backup](./en/LIFECYCLE.md) | Log rotation, OSS expiry, panel DR |
| [WordPress SEO Push (English)](./en/WORDPRESS_SEO.md) | Submit sitemaps to search engines |
| [安装与部署](../README.md) | 一键安装、源码开发、远程部署（见项目 README） |
| [贡献指南](../CONTRIBUTING.md) | 开发环境与 PR 规范 |

## 文档结构

```
docs/
├── README.md              # 本页（文档索引）
├── zh-CN/
│   └── USER_GUIDE.md      # 中文用户手册
└── en/
    └── USER_GUIDE.md      # English user guide
```

## 快速链接

- **首次登录**：用户名 `admin`，密码见服务器 `data/INITIAL_CREDENTIALS.txt` 或 `journalctl -u owpanel`
- **安全入口**：启用后访问 `http://IP:端口/<安全路径>/`
- **CLI**：安装后可用 `op info` / `op config` / `op restart`
- **界面语言**：登录页右下角或 **面板设置 → 界面语言**（简体 / 繁体 / English）
