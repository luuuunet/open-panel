package extension

// FeaturedPack is a curated one-click panel capability backed by the software store.
type FeaturedPack struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	NameEN        string   `json:"name_en,omitempty"`
	Description   string   `json:"description"`
	DescriptionEN string   `json:"description_en,omitempty"`
	Category      string   `json:"category"`
	Icon          string   `json:"icon"`
	Accent        string   `json:"accent"`
	AppKey        string   `json:"app_key"`
	ConfigRoute   string   `json:"config_route,omitempty"`
	Tags          []string `json:"tags"`
	InstallAPI    string   `json:"install_api,omitempty"`
	UninstallAPI  string   `json:"uninstall_api,omitempty"`
	LogsAPI       string   `json:"logs_api,omitempty"`
	Installed     bool     `json:"installed"`
	Running       bool     `json:"running"`
	Status        string   `json:"status"`
	AccessURL     string   `json:"access_url,omitempty"`
}

// FeaturedCatalog returns built-in extension packs users can install with one click.
func FeaturedCatalog() []FeaturedPack {
	return []FeaturedPack{
		{
			ID: "openpanel-analytics", Name: "网站产品分析", NameEN: "Product Analytics",
			Description: "漏斗、会话回放、用户分群与 A/B 测试，深度洞察站点转化",
			DescriptionEN: "Funnels, session replay, cohorts, and A/B testing for your sites",
			Category: "analytics", Icon: "DataAnalysis", Accent: "#6366f1",
			AppKey: "openpanel-analytics", ConfigRoute: "/product-analytics",
			Tags: []string{"A/B", "漏斗", "BI"},
		},
		{
			ID: "umami", Name: "Umami 统计", NameEN: "Umami Analytics",
			Description: "隐私友好的轻量网站访问统计，无需 Cookie 横幅",
			DescriptionEN: "Privacy-friendly lightweight web analytics",
			Category: "analytics", Icon: "TrendCharts", Accent: "#0ea5e9",
			AppKey: "umami", Tags: []string{"统计", "隐私"},
		},
		{
			ID: "metabase", Name: "Metabase BI", NameEN: "Metabase BI",
			Description: "连接数据库的可视化 BI 与自助查询报表",
			DescriptionEN: "Self-service BI and dashboards connected to your databases",
			Category: "analytics", Icon: "TrendCharts", Accent: "#8b5cf6",
			AppKey: "metabase", Tags: []string{"BI", "报表"},
		},
		{
			ID: "grafana", Name: "Grafana 监控", NameEN: "Grafana",
			Description: "开源监控与可观测性可视化大盘",
			DescriptionEN: "Open-source monitoring and observability dashboards",
			Category: "monitoring", Icon: "TrendCharts", Accent: "#f97316",
			AppKey: "grafana", Tags: []string{"监控", "大盘"},
		},
		{
			ID: "prometheus", Name: "Prometheus", NameEN: "Prometheus",
			Description: "时序指标采集与告警规则引擎",
			DescriptionEN: "Time-series metrics collection and alerting",
			Category: "monitoring", Icon: "Odometer", Accent: "#e11d48",
			AppKey: "prometheus", Tags: []string{"指标", "告警"},
		},
		{
			ID: "uptime-kuma", Name: "Uptime Kuma", NameEN: "Uptime Kuma",
			Description: "自托管网站与服务可用性监控，支持多通道通知",
			DescriptionEN: "Self-hosted uptime monitoring with notifications",
			Category: "monitoring", Icon: "Bell", Accent: "#22c55e",
			AppKey: "uptime-kuma", Tags: []string{"可用性", "告警"},
		},
		{
			ID: "netdata", Name: "Netdata", NameEN: "Netdata",
			Description: "秒级主机与容器性能实时监控",
			DescriptionEN: "Real-time host and container performance monitoring",
			Category: "monitoring", Icon: "Monitor", Accent: "#14b8a6",
			AppKey: "netdata", Tags: []string{"性能", "实时"},
		},
		{
			ID: "beszel", Name: "Beszel 监控", NameEN: "Beszel",
			Description: "轻量级多节点服务器资源监控",
			DescriptionEN: "Lightweight multi-node server monitoring",
			Category: "monitoring", Icon: "Histogram", Accent: "#64748b",
			AppKey: "beszel", Tags: []string{"轻量", "多机"},
		},
		{
			ID: "kafka", Name: "Kafka 消息队列", NameEN: "Kafka",
			Description: "高吞吐分布式事件流，适合日志与异步解耦",
			DescriptionEN: "High-throughput distributed event streaming",
			Category: "middleware", Icon: "Share", Accent: "#a855f7",
			AppKey: "kafka", Tags: []string{"消息", "流处理"},
		},
		{
			ID: "meilisearch", Name: "MeiliSearch", NameEN: "MeiliSearch",
			Description: "极速全文搜索引擎，为站点与应用提供即时搜索",
			DescriptionEN: "Blazing-fast full-text search for apps and sites",
			Category: "middleware", Icon: "Search", Accent: "#06b6d4",
			AppKey: "meilisearch", Tags: []string{"搜索", "全文"},
		},
		{
			ID: "rabbitmq", Name: "RabbitMQ", NameEN: "RabbitMQ",
			Description: "成熟的消息队列，含 Web 管理界面",
			DescriptionEN: "Mature message broker with management UI",
			Category: "middleware", Icon: "Connection", Accent: "#f59e0b",
			AppKey: "rabbitmq", Tags: []string{"MQ", "异步"},
		},
		{
			ID: "n8n", Name: "n8n 工作流", NameEN: "n8n Automation",
			Description: "可视化编排 Webhook、定时任务与第三方 API",
			DescriptionEN: "Visual workflow automation for webhooks and APIs",
			Category: "automation", Icon: "SetUp", Accent: "#ec4899",
			AppKey: "n8n", Tags: []string{"自动化", "Webhook"},
		},
		{
			ID: "portainer", Name: "Portainer", NameEN: "Portainer",
			Description: "Docker 容器与镜像可视化管理",
			DescriptionEN: "Visual Docker container and image management",
			Category: "devtools", Icon: "Box", Accent: "#2563eb",
			AppKey: "portainer", Tags: []string{"Docker", "容器"},
		},
		{
			ID: "dockge", Name: "Dockge", NameEN: "Dockge",
			Description: "Docker Compose 栈编辑、部署与日志查看",
			DescriptionEN: "Manage Docker Compose stacks with a clean UI",
			Category: "devtools", Icon: "Grid", Accent: "#7c3aed",
			AppKey: "dockge", Tags: []string{"Compose", "栈"},
		},
		{
			ID: "code-server", Name: "code-server", NameEN: "code-server",
			Description: "浏览器中的 VS Code 远程开发环境",
			DescriptionEN: "VS Code in the browser for remote development",
			Category: "devtools", Icon: "Monitor", Accent: "#1d4ed8",
			AppKey: "code-server", Tags: []string{"IDE", "远程"},
		},
		{
			ID: "huggingface-ai", Name: "Hugging Face AI", NameEN: "Hugging Face AI",
			Description: "一键部署 TGI 推理与 Web 对话，接入面板 AI 能力",
			DescriptionEN: "Deploy TGI inference and chat UI integrated with panel AI",
			Category: "devtools", Icon: "Cpu", Accent: "#eab308",
			AppKey: "huggingface-ai", ConfigRoute: "/ai-hub",
			Tags: []string{"AI", "推理"},
			UninstallAPI: "/ai/huggingface/uninstall",
		},
	}
}
