# 📚 OWPanel 文档

OWPanel 完整使用与运维文档索引。

## 📖 用户手册

| 文档 | 说明 |
|------|------|
| 🇨🇳 [中文用户手册](./zh-CN/USER_GUIDE.md) | 全模块功能说明、权限、安全与常见问题 |
| 🇺🇸 [English User Guide](./en/USER_GUIDE.md) | Full module reference (English) |

## 📑 专题指南

| 文档 | 说明 |
|------|------|
| 🤖 [自动化指南（小白版）](./zh-CN/AUTOMATION.md) | 与宝塔/1Panel/云厂商对比、一键预设、迁移对照 |
| ☁️ [云厂商整合指南](./zh-CN/CLOUD.md) | OSS/DNS/备份/监控多云接入 |
| 💾 [存储生命周期与云备份](./zh-CN/LIFECYCLE.md) | 日志轮转、OSS 过期、面板云备份、灾难恢复 |
| 💾 [Storage Lifecycle (EN)](./en/LIFECYCLE.md) | Log rotation, OSS expiry, panel DR |
| 🔍 [WordPress 搜索引擎推送](./zh-CN/WORDPRESS_SEO.md) | Google、Bing、IndexNow、百度等 |
| 🦀 [Rust 运行环境](./zh-CN/RUST.md) | 安装、PM2/Docker、AI 部署 |
| 🦀 [Rust Runtime (EN)](./en/RUST.md) | Install, runtimes, PM2/Docker |
| 🤖 [Automation Guide (EN)](./en/AUTOMATION.md) | Cloud comparison, presets, migration |

## 🛠 安装与开发

| 文档 | 说明 |
|------|------|
| 🏠 [项目 README](../README.md) | 产品特点、功能列表、一键安装 |
| 🤝 [贡献指南](../CONTRIBUTING.md) | 开发环境与 PR 规范 |

## ⚡ 快速参考

- 🔑 **首次登录**：用户名 `admin`，密码见 `data/INITIAL_CREDENTIALS.txt` 或 `journalctl -u owpanel`
- 🛡️ **安全入口**：启用后访问 `http://IP:端口/<安全路径>/`
- ⌨️ **CLI**：`op info` / `op config` / `op restart` / `op update`
- 🌍 **界面语言**：登录页右下角或 **面板设置 → 界面语言**
