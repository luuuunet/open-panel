# OWPanel 文档

OWPanel 完整使用与运维文档。

| 文档 | 说明 |
|------|------|
| [中文用户手册](./zh-CN/USER_GUIDE.md) | 全模块功能说明、权限、安全与常见问题 |
| [English User Guide](./en/USER_GUIDE.md) | Full module reference (English) |
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
