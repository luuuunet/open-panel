package edgeworker

type WorkerTemplate struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	NameZh      string `json:"name_zh"`
	Description string `json:"description"`
	DescriptionZh string `json:"description_zh"`
	ScriptType  string `json:"script_type"`
	RoutePattern string `json:"route_pattern"`
	Triggers    string `json:"triggers"`
	Script      string `json:"script"`
}

func (s *Service) Templates() []WorkerTemplate {
	return []WorkerTemplate{
		{
			ID:            "redirect-path",
			Name:          "Redirect by path",
			NameZh:        "按路径重定向",
			Description:   "301 redirect /old-path to /new-path (Cloudflare Workers fetch event style)",
			DescriptionZh: "将 /old-path 301 重定向到 /new-path（类似 Cloudflare Workers 拦截请求）",
			ScriptType:    "lua",
			RoutePattern:  "~ ^/old-path",
			Triggers:      "request",
			Script: `-- Edge Worker: redirect by path
-- Matches Cloudflare: return Response.redirect(url, 301)
if ngx.var.uri == "/old-path" or string.match(ngx.var.uri, "^/old%-path/") then
    return ngx.redirect("/new-path", ngx.HTTP_MOVED_PERMANENTLY)
end
`,
		},
		{
			ID:            "add-header",
			Name:          "Add custom response header",
			NameZh:        "添加自定义响应头",
			Description:   "Add X-Powered-By edge header on all matched routes",
			DescriptionZh: "在匹配的响应上添加 X-Powered-By 边缘节点标识",
			ScriptType:    "lua",
			RoutePattern:  "/",
			Triggers:      "response",
			Script: `-- Edge Worker: add response header (header_filter phase)
-- Matches Cloudflare: response.headers.set('X-Edge', 'owpanel')
ngx.header["X-Powered-By"] = "OWPanel-Edge"
ngx.header["X-Edge-Worker"] = "add-header"
`,
		},
		{
			ID:            "block-user-agent",
			Name:          "Block User-Agent",
			NameZh:        "拦截 User-Agent",
			Description:   "Return 403 for bad bots / scanners",
			DescriptionZh: "拦截恶意爬虫与扫描器 User-Agent",
			ScriptType:    "lua",
			RoutePattern:  "/",
			Triggers:      "request",
			Script: `-- Edge Worker: block User-Agent
local ua = ngx.var.http_user_agent or ""
if string.match(ua, "(?i)sqlmap") or string.match(ua, "(?i)nikto") or string.match(ua, "(?i)acunetix") then
    return ngx.exit(ngx.HTTP_FORBIDDEN)
end
`,
		},
		{
			ID:            "maintenance-mode",
			Name:          "Maintenance mode page",
			NameZh:        "维护模式页面",
			Description:   "Serve 503 maintenance HTML for all requests",
			DescriptionZh: "全站维护模式，返回 503 维护页面",
			ScriptType:    "lua",
			RoutePattern:  "/",
			Triggers:      "request",
			Script: `-- Edge Worker: maintenance mode
ngx.status = ngx.HTTP_SERVICE_UNAVAILABLE
ngx.header.content_type = "text/html; charset=utf-8"
ngx.say([[<!DOCTYPE html><html><head><title>Maintenance</title></head>
<body><h1>Site under maintenance</h1><p>Please check back soon.</p></body></html>]])
return ngx.exit(ngx.HTTP_SERVICE_UNAVAILABLE)
`,
		},
		{
			ID:            "rewrite-url",
			Name:          "Rewrite URL",
			NameZh:        "URL 重写",
			Description:   "Internal rewrite /blog/* to /wordpress/*",
			DescriptionZh: "内部重写 /blog/* 到 /wordpress/*",
			ScriptType:    "template",
			RoutePattern:  "^~ /blog/",
			Triggers:      "request",
			Script: `rewrite ^/blog/(.*)$ /wordpress/$1 last;`,
		},
		{
			ID:            "counter-kv",
			Name:          "Counter with KV",
			NameZh:        "KV 计数器",
			Description:   "Increment a visit counter stored in Workers KV binding (bind MY_KV to a namespace first)",
			DescriptionZh: "使用 KV 绑定递增访问计数（请先在 Worker 中绑定 MY_KV 到命名空间）",
			ScriptType:    "lua",
			RoutePattern:  "/",
			Triggers:      "request",
			Script: `-- Edge Worker: counter with KV binding
-- Requires binding: MY_KV -> KV namespace
local key = "counter:" .. (ngx.var.uri or "/")
local val = MY_KV:get(key)
local n = tonumber(val) or 0
n = n + 1
MY_KV:put(key, tostring(n), 86400)
ngx.req.set_header("X-Edge-Counter", tostring(n))
`,
		},
	}
}
