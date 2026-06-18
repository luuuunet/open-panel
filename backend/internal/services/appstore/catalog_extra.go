package appstore

import (
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

func panelApp(key, name, category, desc string, port int, icon string) catalogItem {
	if icon == "" {
		icon = "Box"
	}
	installPath := filepath.Join("apps", key)
	cfgPath := filepath.Join(installPath, ".env")
	defaultCfg := map[string]interface{}{}
	if spec, ok := dockerSpec(key); ok {
		for _, e := range spec.Env {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				defaultCfg[parts[0]] = parts[1]
			}
		}
	}
	return catalogItem{
		App: models.App{
			Key:         key,
			Name:        name,
			Category:    category,
			Versions:    "latest",
			Version:     "latest",
			Description: desc,
			Port:        port,
			InstallPath: installPath,
			ConfigPath:  cfgPath,
			Icon:        icon,
		},
		defaultConfig: defaultCfg,
	}
}

// catalogExtraApps — 内置扩展应用目录（Docker 一键部署等），不依赖外部商店同步。
var catalogExtraApps = []catalogItem{
	// ── 截图 Page1：AI ──
	panelApp("hermes-agent", "Hermes Agent", "人工智能", "Nous Research 自托管 AI Agent", 8010, "Cpu"),
	panelApp("openclaw", "OpenClaw", "人工智能", "个人 AI 助手，支持多模型与工具调用", 8011, "ChatDotRound"),
	panelApp("qwenpow", "QwenPow", "人工智能", "阿里开源 AI 个人助手", 8012, "ChatLineRound"),
	panelApp("maxkb", "MaxKB", "人工智能", "开源企业级 AI 知识库与 Agent 平台", 8080, "Document"),
	panelApp("clawswarm", "ClawSwarm", "人工智能", "OpenClaw Agent 集群编排系统", 8013, "Share"),
	panelApp("upage", "UPage", "人工智能", "LLM 驱动的可视化网页构建器", 8014, "Monitor"),
	panelApp("dbhub", "DBHub", "人工智能", "通用数据库 MCP 服务器（MySQL/PG/SQL Server 等）", 8015, "Coin"),

	// ── 截图 Page1：网站 / DevOps / 安全 ──
	panelApp("wordpress", "WordPress", "网站", "开源博客与 CMS 系统", 8081, "Reading"),
	panelApp("gitea", "Gitea", "DevOps", "轻量级 Git 代码托管平台", 3000, "Platform"),
	panelApp("jumpserver", "JumpServer", "安全", "开源堡垒机与运维审计", 8088, "Key"),
	panelApp("k3s", "K3s", "DevOps", "轻量级 Kubernetes，Cilium eBPF 网络与安全的基础运行时", 6443, "Platform"),
	panelApp("cilium", "Cilium", "安全", "CNCF 毕业项目：eBPF 网络、可观测性与 Host Firewall（需先安装 K3s）", 0, "Key"),
	panelApp("nginx-proxy-manager", "Nginx Proxy Manager", "Web服务器", "Nginx 反向代理与 SSL 可视化管理", 81, "SetUp"),
	panelApp("tomcat", "Apache Tomcat", "中间件", "开源 Java Web 容器与 Servlet 引擎", 8080, "CoffeeCup"),

	// ── 截图 Page1：监控 / 工具 / 数据库 ──
	panelApp("zabbix-server", "Zabbix Server", "工具", "开源 IT 基础设施监控服务端", 10051, "TrendCharts"),
	panelApp("zabbix-agent", "Zabbix Agent", "工具", "Zabbix 监控 Agent 客户端", 10050, "TrendCharts"),
	panelApp("sftpgo", "SFTPGo", "工具", "功能完整的 SFTP/HTTP 文件服务", 8082, "Upload"),

	// ── 截图 Page2 ──
	panelApp("alist", "AList", "云存储", "多存储网盘文件列表与私有云", 5244, "FolderOpened"),
	panelApp("mongo-express", "mongo-express", "开发工具", "MongoDB Web 管理界面", 8082, "Coin"),
	panelApp("dashy", "Dashy", "网站", "自托管个人仪表盘与启动页", 8083, "Monitor"),
	panelApp("nocobase", "NocoBase", "开发工具", "可扩展开源无代码开发平台", 8084, "Grid"),
	panelApp("bytebase", "Bytebase", "DevOps", "开源数据库 DevOps 与变更管理", 8085, "Coin"),
	panelApp("elasticsearch", "Elasticsearch", "数据库", "分布式搜索与分析引擎", 9200, "Search"),
	panelApp("synapse", "Synapse", "工具", "Matrix 开源聊天 Homeserver", 8008, "ChatDotRound"),
	panelApp("dockge", "Dockge", "工具", "Docker Compose 栈可视化管理", 5001, "Box"),
	panelApp("meilisearch", "MeiliSearch", "工具", "开源极速全文搜索引擎", 7700, "Search"),
	panelApp("ntfy", "ntfy", "开发工具", "HTTP 发布/订阅通知服务", 8086, "Bell"),
	panelApp("openvpn", "OpenVPN", "安全", "开源 VPN 服务", 1194, "Lock"),
	panelApp("komga", "Komga", "多媒体", "漫画、杂志与 BD 媒体服务器", 8087, "Reading"),
	panelApp("watchtower", "Watchtower", "工具", "自动更新 Docker 容器镜像", 0, "Refresh"),
	panelApp("cloudbeaver", "CloudBeaver", "开发工具", "云数据库 Web 管理器", 8088, "Coin"),
	panelApp("homepage", "Homepage", "工具", "现代化静态应用仪表盘", 8089, "Monitor"),
	panelApp("opengist", "Opengist", "DevOps", "基于 Git 的自托管 Pastebin", 8090, "Document"),
	panelApp("redpanda-console", "Redpanda Console", "开发工具", "Kafka/Redpanda 数据流 UI", 8091, "Odometer"),
	panelApp("discourse", "Discourse", "网站", "开源社区论坛平台", 8092, "ChatLineRound"),
	panelApp("obsidian-livesync", "Obsidian LiveSync", "工具", "Obsidian 自托管笔记同步", 8093, "Document"),

	// ── 截图 Page3 ──
	panelApp("verdaccio", "Verdaccio", "DevOps", "私有 NPM 仓库", 4873, "Box"),
	panelApp("audiobookshelf", "Audiobookshelf", "多媒体", "自托管有声书与播客服务器", 8094, "Headset"),
	panelApp("grafana", "Grafana", "工具", "开源监控与可观测性可视化平台", 3003, "TrendCharts"),
	panelApp("prometheus", "Prometheus", "数据库", "监控时序数据库与告警", 9090, "TrendCharts"),
	panelApp("redis-commander", "Redis-Commander", "开发工具", "Redis Web 管理工具", 8095, "Coin"),
	panelApp("screego", "screego", "工具", "开源屏幕共享工具", 8096, "Monitor"),
	panelApp("cloudflared", "cloudflared", "工具", "Cloudflare Tunnel 内网穿透客户端", 8097, "Link"),
	panelApp("kafka", "Kafka", "中间件", "分布式事件流平台", 9092, "Share"),
	panelApp("ghost", "Ghost", "网站", "开源博客与 Newsletter 平台", 2368, "Reading"),
	panelApp("stalwart", "Stalwart Mail Server", "邮件", "Docker 一体化邮件方案（与「邮件服务器」套件二选一）", 8098, "Message"),
	panelApp("beszel", "Beszel", "工具", "轻量级服务器监控", 8099, "TrendCharts"),
	panelApp("windows", "Windows", "工具", "Docker 内运行 Windows 桌面（实验性）", 8006, "Monitor"),
	panelApp("wallos", "Wallos", "工具", "开源个人订阅追踪器", 8100, "Wallet"),
	panelApp("typesense", "Typesense", "数据库", "开源极速搜索引擎", 8108, "Search"),
	panelApp("zitadel", "ZITADEL", "安全", "开源身份与访问管理（IAM）", 8101, "Key"),
	panelApp("jaeger", "Jaeger", "中间件", "分布式链路追踪系统", 16686, "Share"),
	panelApp("teable", "Teable", "工具", "开源 Airtable 替代方案", 8102, "Grid"),
	panelApp("twenty", "Twenty", "CRM", "开源 Salesforce 替代 CRM", 8103, "User"),
	panelApp("jupyter-notebook", "Jupyter Notebook", "工具", "多语言交互式 Notebook 环境", 8889, "Notebook"),
	panelApp("odoo", "Odoo", "CRM", "开源 ERP/CRM 业务套件", 8104, "OfficeBuilding"),
	panelApp("rustfs", "RustFS", "云存储", "分布式对象存储", 8105, "FolderOpened"),
	panelApp("caddy", "Caddy", "Web服务器", "自动 HTTPS 的现代 Web 服务器", 2015, "SetUp"),

	// ── Docker 应用（已有 spec，补全商店条目）──
	panelApp("halo", "Halo", "网站", "现代化开源博客/CMS", 8090, "Reading"),
	panelApp("typecho", "Typecho", "网站", "轻量 PHP 博客系统", 8080, "Reading"),
	panelApp("outline", "Outline", "网站", "团队知识库 Wiki", 3005, "Document"),
	panelApp("memos", "Memos", "工具", "轻量私有化备忘录", 5230, "EditPen"),
	panelApp("lsky-pro", "Lsky Pro", "工具", "兰空图床", 8099, "Picture"),
	panelApp("flarum", "Flarum", "网站", "现代化 PHP 论坛", 8889, "ChatLineRound"),
	panelApp("portainer", "Portainer", "DevOps", "Docker 容器可视化管理", 9000, "Box"),
	panelApp("jenkins", "Jenkins", "DevOps", "开源 CI/CD 自动化", 8082, "SetUp"),
	panelApp("gitlab", "GitLab", "DevOps", "DevOps 一体化代码平台", 8929, "Platform"),
	panelApp("uptime-kuma", "Uptime Kuma", "工具", "自托管可用性监控", 3004, "Bell"),
	panelApp("netdata", "Netdata", "工具", "实时系统与容器监控", 19999, "TrendCharts"),
	panelApp("sonarqube", "SonarQube", "DevOps", "代码质量与安全分析", 9002, "Warning"),
	panelApp("frps", "frp Server", "工具", "内网穿透 frp 服务端", 7500, "Link"),
	panelApp("minio", "MinIO", "云存储", "S3 兼容对象存储", 9001, "FolderOpened"),
	panelApp("rabbitmq", "RabbitMQ", "中间件", "消息队列（含管理界面）", 15672, "Share"),
	panelApp("nacos", "Nacos", "中间件", "服务发现与配置中心", 8848, "SetUp"),
	panelApp("etcd", "etcd", "中间件", "分布式键值存储", 2379, "Coin"),
	panelApp("traefik", "Traefik", "Web服务器", "云原生反向代理与负载均衡", 8083, "SetUp"),
	panelApp("redis-insight", "Redis Insight", "开发工具", "Redis 可视化管理", 5540, "Coin"),
	panelApp("pgadmin", "pgAdmin", "开发工具", "PostgreSQL Web 管理", 5050, "Coin"),
	panelApp("kibana", "Kibana", "BI", "Elasticsearch 数据可视化", 5601, "TrendCharts"),
	panelApp("metabase", "Metabase", "BI", "开源 BI 与数据分析", 3007, "TrendCharts"),
	panelApp("superset", "Apache Superset", "BI", "现代数据探索与可视化", 8089, "TrendCharts"),
	panelApp("code-server", "code-server", "开发工具", "浏览器 VS Code 远程开发", 8088, "Monitor"),
	panelApp("hoppscotch", "Hoppscotch", "开发工具", "开源 API 调试客户端", 3006, "Connection"),
	panelApp("home-assistant", "Home Assistant", "生活", "智能家居自动化平台", 8123, "House"),
	panelApp("qbittorrent", "qBittorrent", "多媒体", "BT 下载客户端", 8085, "Download"),
	panelApp("jellyfin", "Jellyfin", "多媒体", "开源媒体服务器", 8096, "VideoCamera"),
	panelApp("navidrome", "Navidrome", "多媒体", "自托管音乐流媒体", 4533, "Headset"),
	panelApp("emby", "Emby", "多媒体", "媒体服务器", 8097, "VideoCamera"),
	panelApp("nextcloud", "Nextcloud", "云存储", "私有云盘与协作", 8087, "FolderOpened"),
	panelApp("syncthing", "Syncthing", "工具", "P2P 文件同步", 8384, "Refresh"),
	panelApp("filebrowser", "FileBrowser", "工具", "Web 文件管理器", 8086, "Folder"),
	panelApp("casdoor", "Casdoor", "安全", "OAuth/OIDC 身份认证平台", 8000, "Key"),

	// ── 补充热门 AI / 工具 ──
	panelApp("lobe-chat", "Lobe Chat", "人工智能", "开源 ChatGPT 风格对话 UI", 3210, "ChatDotRound"),
	panelApp("one-api", "One API", "人工智能", "OpenAI 接口聚合与分发", 3000, "Connection"),
	panelApp("new-api", "New API", "人工智能", "新一代 LLM API 网关", 3008, "Connection"),
	panelApp("chatgpt-next-web", "ChatGPT Next Web", "人工智能", "一键部署 ChatGPT 网页客户端", 3009, "ChatDotRound"),
	panelApp("milvus", "Milvus", "人工智能", "开源向量数据库", 19530, "Coin"),
	panelApp("qdrant", "Qdrant", "人工智能", "向量搜索引擎", 6333, "Search"),
	panelApp("n8n", "n8n", "工具", "可视化工作流自动化", 5678, "SetUp"),
	panelApp("activepieces", "Activepieces", "工具", "开源 Zapier 替代", 8080, "SetUp"),
	panelApp("flowise", "Flowise", "人工智能", "LangChain 可视化 AI 编排", 3010, "Share"),
	panelApp("rustdesk", "RustDesk", "工具", "开源远程桌面", 21115, "Monitor"),
	panelApp("adguard-home", "AdGuard Home", "安全", "网络级广告与 DNS 过滤", 3011, "Warning"),
	panelApp("vaultwarden", "Vaultwarden", "安全", "Bitwarden 兼容密码库", 3012, "Lock"),
	panelApp("immich", "Immich", "多媒体", "自托管照片与视频备份", 3013, "Picture"),
	panelApp("paperless-ngx", "Paperless-ngx", "工具", "文档扫描与 OCR 归档", 3014, "Document"),
	panelApp("firefly-iii", "Firefly III", "工具", "个人财务与记账", 3015, "Wallet"),
	panelApp("stirling-pdf", "Stirling PDF", "工具", "本地 PDF 工具箱", 3016, "Document"),
	panelApp("it-tools", "IT-Tools", "开发工具", "开发者在线工具集", 3017, "Tools"),
	panelApp("umami", "Umami", "BI", "隐私友好网站分析", 3018, "TrendCharts"),
	panelApp("matomo", "Matomo", "BI", "开源网站统计", 3019, "TrendCharts"),
	panelApp("searxng", "SearXNG", "工具", "元搜索引擎", 3020, "Search"),
	panelApp("nocodb", "NocoDB", "开发工具", "Airtable 替代，连接现有数据库", 3021, "Grid"),
	panelApp("bookstack", "BookStack", "网站", "简单易用的 Wiki 文档", 3022, "Document"),
	panelApp("wiki-js", "Wiki.js", "网站", "现代化 Wiki 系统", 3023, "Document"),
	panelApp("mattermost", "Mattermost", "工具", "开源团队协作与聊天", 8065, "ChatDotRound"),
	panelApp("rocketchat", "Rocket.Chat", "工具", "开源团队聊天平台", 3024, "ChatDotRound"),
	panelApp("adminer", "Adminer", "开发工具", "单文件数据库管理", 3025, "Coin"),
	panelApp("influxdb", "InfluxDB", "数据库", "时序数据库", 8086, "TrendCharts"),
	panelApp("clickhouse", "ClickHouse", "数据库", "列式 OLAP 数据库", 8123, "Coin"),
	panelApp("keycloak", "Keycloak", "安全", "开源 IAM 与 SSO", 3026, "Key"),
	panelApp("affine", "AFFiNE", "工具", "开源 Notion 替代", 3027, "EditPen"),
	panelApp("trilium", "Trilium", "工具", "分层笔记与知识库", 3028, "Document"),
	panelApp("drawio", "draw.io", "工具", "在线流程图与图表", 3029, "Picture"),
	panelApp("excalidraw", "Excalidraw", "工具", "虚拟白板手绘风格", 3030, "EditPen"),

	// ── 图形处理 ──
	panelApp("photoprism", "PhotoPrism", "图形处理", "AI 照片管理与索引，支持人脸识别、地图与相册", 2342, "Picture"),
	panelApp("pigallery2", "PiGallery2", "图形处理", "轻量级自托管图库，快速浏览与管理照片", 3031, "PictureFilled"),
	panelApp("imgproxy", "imgproxy", "图形处理", "高性能图片实时处理：缩放、裁剪、水印与格式转换", 3032, "Picture"),
	panelApp("imagor", "imagor", "图形处理", "Go 语言图片处理服务，安全缩略图与 CDN 友好", 3033, "Picture"),
	panelApp("thumbor", "Thumbor", "图形处理", "智能图片裁剪与实时处理 API 服务", 3037, "PictureFilled"),

	// ── 视频处理 ──
	panelApp("tdarr", "Tdarr", "视频处理", "自动化视频转码、健康检查与媒体库批量处理", 8265, "VideoCamera"),
	panelApp("unmanic", "Unmanic", "视频处理", "扫描媒体库并自动转码为统一格式（H.264/H.265）", 3038, "VideoCamera"),
	panelApp("handbrake", "HandBrake", "视频处理", "开源视频转码工具，浏览器 Web UI 操作", 5800, "Film"),
	panelApp("fileflows", "FileFlows", "视频处理", "可视化媒体工作流：转码、重封装与自动化处理", 3040, "Film"),
}

func mergedCatalog() []catalogItem {
	seen := make(map[string]struct{}, len(catalog))
	for _, item := range catalog {
		seen[item.Key] = struct{}{}
	}
	out := make([]catalogItem, 0, len(catalog)+len(catalogExtraApps))
	out = append(out, catalog...)
	for _, item := range catalogExtraApps {
		if _, ok := seen[item.Key]; ok {
			continue
		}
		seen[item.Key] = struct{}{}
		out = append(out, item)
	}
	out = appendExtensionCatalog(out)
	out = appendBuiltinPHPCatalog(out)
	out = appendMySQLVersionCatalog(out)
	return appendDynamicCatalog(out)
}
