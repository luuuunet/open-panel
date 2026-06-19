package security

import (
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/appstore"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"github.com/luuuunet/owpanel/internal/services/waf"
	"gorm.io/gorm"
)

type RiskItem struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Level    string `json:"level"`
	Status   string `json:"status"`
	Solution string `json:"solution"`
	FixType  string `json:"fix_type,omitempty"` // auto | install | navigate | none
}

type Service struct {
	waf      *waf.Service
	appstore *appstore.Service
	settings *settings.Service
	db       *gorm.DB
}

func NewService(wafSvc *waf.Service, apps *appstore.Service, settingsSvc *settings.Service, db *gorm.DB) *Service {
	return &Service{waf: wafSvc, appstore: apps, settings: settingsSvc, db: db}
}

func (s *Service) Scan() []RiskItem {
	status := map[string]interface{}{}
	if s.waf != nil {
		status = s.waf.StatusSummary()
	}

	wafStatus := "warn"
	if enabled, _ := status["rate_limit"].(bool); enabled {
		if rules, _ := status["filter_rules"].(int); rules > 0 {
			wafStatus = "pass"
		}
	}
	if exists, _ := status["conf_exists"].(bool); exists {
		wafStatus = "pass"
	}

	items := []RiskItem{
		{Key: "rate_limit", Name: "流量控制 (Rate Limit)", Level: "high", Status: boolStatus(status["rate_limit"]), Solution: "限制单 IP 请求频率，防御 CC 攻击", FixType: "auto"},
		{Key: "conn_limit", Name: "并发连接限制", Level: "high", Status: boolStatus(status["conn_limit"]), Solution: "限制单 IP 最大并发，防止 Worker 耗尽", FixType: "auto"},
		{Key: "ip_blacklist", Name: "IP 黑名单", Level: "medium", Status: blacklistStatus(status), Solution: "维护动态 IP 黑名单 Map", FixType: "navigate"},
		{Key: "geo_block", Name: "国家/地区访问限制", Level: "high", Status: geoStatus(status), Solution: "基于 GeoIP2 限制国家/地区访问，需 GeoLite2 数据库", FixType: "auto"},
		{Key: "filter_rules", Name: "恶意请求过滤", Level: "high", Status: filterStatus(status), Solution: "拦截 SQL 注入、XSS、扫描器 UA", FixType: "auto"},
		{Key: "edge_policies", Name: "边缘策略 (Edge Policies)", Level: "medium", Status: edgePoliciesStatus(status), Solution: "白名单、Bot 管理、慢速攻击、API 限速、防盗链", FixType: "navigate"},
		{Key: "headers", Name: "安全 Headers", Level: "medium", Status: boolStatus(status["headers"]), Solution: "启用 CSP、HSTS、X-Frame-Options", FixType: "auto"},
		{Key: "security_log", Name: "安全日志 / Fail2Ban", Level: "medium", Status: boolStatus(status["security_log"]), Solution: "规范化日志含 request_id、upstream_response_time", FixType: "auto"},
		{Key: "nginx_waf", Name: "Nginx WAF 配置", Level: "high", Status: wafStatus, Solution: "在 WAF 页面点击「应用配置」生成 nginx 规则", FixType: "auto"},
		{Key: "fail2ban", Name: "Fail2ban", Level: "medium", Status: s.fail2banStatus(), Solution: "建议安装 Fail2ban 联动安全日志", FixType: "install"},
		{Key: "ssh_port", Name: "SSH 加固", Level: "medium", Status: sshPortStatus(), Solution: "建议修改 SSH 默认端口并禁用 root/密码登录", FixType: "navigate"},
		{Key: "panel_entry", Name: "面板安全入口", Level: "high", Status: panelEntryStatus(s.settings), Solution: "启用随机安全入口路径，隐藏面板地址", FixType: "navigate"},
		{Key: "panel_ip_whitelist", Name: "面板 IP 访问控制", Level: "high", Status: s.panelIPWhitelistStatus(), Solution: "启用 IP 白名单限制面板登录来源", FixType: "auto"},
		{Key: "login_audit", Name: "登录审计日志", Level: "medium", Status: s.loginAuditStatus(), Solution: "记录登录成功/失败并在安全页查看", FixType: "none"},
		{Key: "strong_password", Name: "强密码策略", Level: "medium", Status: s.strongPasswordStatus(), Solution: "要求密码包含大小写字母与数字", FixType: "auto"},
		{Key: "db_remote", Name: "数据库远程访问", Level: "high", Status: s.dbRemoteStatus(), Solution: "已禁止远程访问", FixType: "none"},
		{Key: "ssl", Name: "SSL 证书", Level: "medium", Status: s.sslStatus(), Solution: "证书配置正常", FixType: "none"},
	}
	return items
}

func (s *Service) fail2banStatus() string {
	if s.appstore == nil {
		return "warn"
	}
	app, err := s.appstore.Get("fail2ban")
	if err != nil || !app.Installed {
		return "warn"
	}
	if s.appstore.LiveStatus("fail2ban") == "running" {
		return "pass"
	}
	return "warn"
}

func boolStatus(v interface{}) string {
	if b, ok := v.(bool); ok && b {
		return "pass"
	}
	return "warn"
}

func filterStatus(status map[string]interface{}) string {
	if n, ok := status["filter_rules"].(int); ok && n > 0 {
		return "pass"
	}
	return "warn"
}

func blacklistStatus(status map[string]interface{}) string {
	if n, ok := status["blacklist_count"].(int); ok && n >= 0 {
		return "pass"
	}
	return "warn"
}

func geoStatus(status map[string]interface{}) string {
	enabled, _ := status["geo_block"].(bool)
	if !enabled {
		return "warn"
	}
	dbOK, _ := status["geo_db_exists"].(bool)
	countries, _ := status["geo_countries"].(int)
	if dbOK && countries > 0 {
		return "pass"
	}
	if countries > 0 {
		return "warn"
	}
	return "warn"
}

func edgePoliciesStatus(status map[string]interface{}) string {
	ep, ok := status["edge_policies"].(map[string]interface{})
	if !ok || ep == nil {
		return "warn"
	}
	if wl, _ := ep["whitelist_enabled"].(bool); wl {
		return "pass"
	}
	if sa, _ := ep["slow_attack"].(bool); sa {
		return "pass"
	}
	if api, _ := ep["api_rate_limit"].(bool); api {
		return "pass"
	}
	if hl, _ := ep["hotlink"].(bool); hl {
		return "pass"
	}
	if bh, _ := ep["block_headless_bots"].(bool); bh {
		return "pass"
	}
	return "warn"
}

func (s *Service) panelIPWhitelistStatus() string {
	if s.settings == nil {
		return "warn"
	}
	all, _ := s.settings.GetAll()
	enabled := all["panel_ip_whitelist_enabled"] == "true"
	if !enabled {
		return "warn"
	}
	if strings.TrimSpace(all["panel_ip_whitelist"]) == "" {
		return "fail"
	}
	return "pass"
}

func (s *Service) loginAuditStatus() string {
	return "pass"
}

func (s *Service) strongPasswordStatus() string {
	if s.settings == nil {
		return "warn"
	}
	all, _ := s.settings.GetAll()
	if all["password_require_strong"] != "false" {
		return "pass"
	}
	return "warn"
}

func (s *Service) dbRemoteStatus() string {
	if s.appstore == nil {
		return "warn"
	}
	found := false
	for _, key := range []string{"mysql", "mariadb"} {
		app, err := s.appstore.Get(key)
		if err != nil || !app.Installed {
			continue
		}
		found = true
		raw, err := s.appstore.ReadConfigRaw(key)
		if err != nil {
			return "warn"
		}
		bind := parseBindAddress(raw)
		if bind == "" {
			return "warn"
		}
		if isLocalDBBind(bind) {
			continue
		}
		return "fail"
	}
	if !found {
		return "pass"
	}
	return "pass"
}

func (s *Service) sslStatus() string {
	if s.settings != nil {
		all, _ := s.settings.GetAll()
		if all["panel_ssl"] == "true" {
			return "pass"
		}
	}
	if s.db != nil {
		var active int64
		s.db.Model(&models.SSLCertificate{}).Where("status = ?", "active").Count(&active)
		if active > 0 {
			return "pass"
		}
	}
	return "warn"
}

func parseBindAddress(config string) string {
	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "bind-address") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func isLocalDBBind(addr string) bool {
	addr = strings.Trim(strings.TrimSpace(addr), "'\"")
	switch addr {
	case "127.0.0.1", "localhost", "::1":
		return true
	case "0.0.0.0", "*", "::":
		return false
	}
	return strings.HasPrefix(addr, "127.")
}
