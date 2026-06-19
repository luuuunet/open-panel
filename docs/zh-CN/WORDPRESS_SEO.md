# WordPress 搜索引擎推送（SEO Push）

OWPanel 可为 WordPress 站点**自动或手动**向 Google、Bing、IndexNow（Bing/Yandex/Naver 等）、百度、Yandex 提交站点地图，加快收录。

> 入口：**应用 → WordPress** → 站点列表「SEO 推送」列或操作栏 📣 按钮。

---

## 一、能做什么？

| 能力 | 说明 |
|------|------|
| **部署后自动推送** | WordPress 安装成功并上线后，自动提交站点地图（默认开启） |
| **立即推送** | 发布新文章、改版后，手动再推一次 |
| **多引擎** | Google / Bing / IndexNow / 百度 / Yandex 可单独开关 |
| **自动找站点地图** | 优先检测 `https://你的域名/wp-sitemap.xml`，其次 `sitemap_index.xml`、`sitemap.xml` |
| **IndexNow 密钥** | 自动生成并在网站根目录写入 `{密钥}.txt`，供 Bing 等验证 |

---

## 二、快速上手（3 步）

### 1. 部署 WordPress

在 **WordPress → 部署 WordPress** 填写域名。部署表单里可勾选 **「SEO 推送」**（默认开）。

### 2. 确保站点可访问

- 域名已解析到服务器
- 建议开启 **SSL**（HTTPS 有助于 IndexNow 验证密钥文件）
- 站点状态为 **running** 后再推送

### 3. 打开 SEO 推送设置

点击站点行的 **SEO 推送** 标签或 📣 图标：

1. 确认 **启用 SEO 推送** 已打开  
2. **IndexNow** 建议保持开启（一次通知多个引擎）  
3. 点击 **立即推送**  
4. 查看 **本次推送结果** 和 **上次推送日志**

---

## 三、各搜索引擎说明

### Google

- OWPanel 仍会尝试发送 sitemap ping（历史接口）。
- **Google 已官方弃用 Ping**，不保证收录速度。
- **务必**在 [Google Search Console](https://search.google.com/search-console) 添加站点并提交站点地图。

### Bing

- 通过 `bing.com/ping?sitemap=...` 提交站点地图，适合国际站。

### IndexNow（推荐）

- 开放协议，一次 POST 可通知 **Bing、Yandex、Naver、Seznam** 等。
- OWPanel 会：
  1. 生成或读取 IndexNow 密钥  
  2. 在网站根目录创建 `https://域名/{密钥}.txt`  
  3. 向 `api.indexnow.org` 提交首页 + 站点地图 URL  

### 百度

- 面向国内收录，使用 `data.zz.baidu.com/ping`。
- 在 [百度搜索资源平台](https://ziyuan.baidu.com/) 获取 **推送 Token** 后填入（可选，无 Token 也可尝试 Ping）。

### Yandex

- 俄罗斯搜索引擎，通过 Yandex Webmaster ping 提交站点地图。

---

## 四、常见配置

### 自定义站点地图 URL

若使用 Yoast、Rank Math 等插件且站点地图路径特殊，在 **站点地图 URL** 填完整地址，例如：

```
https://example.com/sitemap_index.xml
```

留空则自动检测 WordPress 默认路径。

### 关闭部署后自动推送

在 SEO 设置里关闭 **「部署后自动推送」**，仅保留手动 **立即推送**。

### 完全关闭

关闭 **「启用 SEO 推送」** 即可；不影响网站正常运行。

---

## 五、部署后没推送？

检查：

1. **启用 SEO 推送** 和 **部署后自动推送** 是否开启  
2. 站点状态是否为 `running`（部署失败不会推）  
3. 域名是否能从公网访问（HEAD 请求检测站点地图）  
4. 查看 **上次推送日志** 里各引擎 OK/FAIL 详情  

---

## 六、与 SEO 插件的关系

- WordPress 5.5+ 自带 `/wp-sitemap.xml`，一般无需额外插件。  
- 若插件接管站点地图，请在 OWPanel 填写插件提供的 **站点地图 URL**。  
- OWPanel **不会**代替 Search Console / 站长平台的「站点验证」，验证仍需在对应平台完成。

---

## 七、API（高级）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/wordpress/:id/seo-push` | 读取设置 |
| PUT | `/api/wordpress/:id/seo-push` | 更新设置 |
| POST | `/api/wordpress/:id/seo-push` | 立即推送 |

---

## 相关文档

- [用户手册](./USER_GUIDE.md) — WordPress 部署与备份  
- [自动化指南](./AUTOMATION.md) — 监控、备份一键预设  
