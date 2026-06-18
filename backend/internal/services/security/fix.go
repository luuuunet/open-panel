package security

import (
	"fmt"

	"github.com/luuuunet/owpanel/internal/models"
)

type FixResult struct {
	Success       bool   `json:"success"`
	NeedsGuide    bool   `json:"needs_guide"`
	GuideMessage  string `json:"guide_message,omitempty"`
	RedirectPath  string `json:"redirect_path,omitempty"`
	InstallJobKey string `json:"install_job_key,omitempty"`
	Message       string `json:"message,omitempty"`
}

type FixAllResult struct {
	Results []FixAllItem `json:"results"`
}

type FixAllItem struct {
	Key    string     `json:"key"`
	Result *FixResult `json:"result"`
	Error  string     `json:"error,omitempty"`
}

func (s *Service) Fix(key string) (*FixResult, error) {
	items := s.Scan()
	var item *RiskItem
	for i := range items {
		if items[i].Key == key {
			item = &items[i]
			break
		}
	}
	if item == nil {
		return nil, fmt.Errorf("unknown check key: %s", key)
	}
	if item.Status == "pass" {
		return &FixResult{Success: true, Message: "already passing"}, nil
	}
	if item.FixType == "" || item.FixType == "none" {
		return nil, fmt.Errorf("check item %s cannot be auto-fixed", key)
	}

	switch key {
	case "rate_limit":
		return s.fixEnableAndApply(func(cfg *models.SecurityConfig) { cfg.RateLimitEnabled = true },
			"已启用流量控制并应用 WAF 配置")
	case "conn_limit":
		return s.fixEnableAndApply(func(cfg *models.SecurityConfig) { cfg.ConnLimitEnabled = true },
			"已启用并发连接限制并应用 WAF 配置")
	case "filter_rules":
		s.waf.SeedFilterRules()
		return s.fixEnableAndApply(func(cfg *models.SecurityConfig) {
			cfg.FilterEnabled = true
			cfg.BlockBadUserAgent = true
			cfg.BlockScannerUA = true
		}, "已启用恶意请求过滤并应用 WAF 配置")
	case "headers":
		return s.fixEnableAndApply(func(cfg *models.SecurityConfig) { cfg.HeadersEnabled = true },
			"已启用安全 Headers 并应用 WAF 配置")
	case "security_log":
		return s.fixEnableAndApply(func(cfg *models.SecurityConfig) { cfg.LogFormatEnabled = true },
			"已启用安全日志格式并应用 WAF 配置")
	case "nginx_waf":
		if _, err := s.waf.Apply(); err != nil {
			return nil, err
		}
		return &FixResult{Success: true, Message: "Nginx WAF 配置已生成并应用"}, nil
	case "geo_block":
		return s.fixGeoBlock()
	case "fail2ban":
		return s.fixFail2ban()
	case "ip_blacklist":
		return &FixResult{
			Success:      false,
			NeedsGuide:   true,
			GuideMessage: "请在 WAF 页面维护 IP 黑名单",
			RedirectPath: "/protection?tab=waf",
		}, nil
	case "edge_policies":
		return &FixResult{
			Success:      false,
			NeedsGuide:   true,
			GuideMessage: "请在 WAF「边缘策略」标签页配置白名单、Bot 管理、慢速攻击、API 限速或防盗链",
			RedirectPath: "/protection?tab=waf",
		}, nil
	case "ssh_port":
		return &FixResult{
			Success:      false,
			NeedsGuide:   true,
			GuideMessage: "请编辑 /etc/ssh/sshd_config：修改 Port、设置 PermitRootLogin no、PasswordAuthentication no，然后 systemctl restart sshd",
			RedirectPath: "/terminal",
		}, nil
	case "panel_entry":
		return &FixResult{
			Success:      false,
			NeedsGuide:   true,
			GuideMessage: "请在「面板设置」查看安全入口，或在终端运行 sudo op config 重新生成",
			RedirectPath: "/settings",
		}, nil
	case "panel_ip_whitelist", "strong_password":
		return &FixResult{
			Success:      false,
			NeedsGuide:   true,
			GuideMessage: "请在安全页「面板访问控制」标签启用 IP 白名单或强密码策略",
			RedirectPath: "/protection?tab=security",
		}, nil
	default:
		return nil, fmt.Errorf("no fix handler for key: %s", key)
	}
}

func (s *Service) FixAll() (*FixAllResult, error) {
	items := s.Scan()
	out := &FixAllResult{Results: []FixAllItem{}}
	for _, item := range items {
		if item.Status == "pass" || item.FixType == "" || item.FixType == "none" {
			continue
		}
		result, err := s.Fix(item.Key)
		entry := FixAllItem{Key: item.Key, Result: result}
		if err != nil {
			entry.Error = err.Error()
		}
		out.Results = append(out.Results, entry)
	}
	return out, nil
}

func (s *Service) fixEnableAndApply(mutate func(*models.SecurityConfig), message string) (*FixResult, error) {
	cfg, err := s.waf.GetConfig()
	if err != nil {
		return nil, err
	}
	mutate(cfg)
	if _, err := s.waf.UpdateConfig(cfg); err != nil {
		return nil, err
	}
	if _, err := s.waf.Apply(); err != nil {
		return nil, err
	}
	return &FixResult{Success: true, Message: message}, nil
}

func (s *Service) fixGeoBlock() (*FixResult, error) {
	status := map[string]interface{}{}
	if s.waf != nil {
		status = s.waf.StatusSummary()
	}
	dbOK, _ := status["geo_db_exists"].(bool)
	if !dbOK {
		if _, err := s.waf.InstallGeoDB(); err != nil {
			return nil, err
		}
		status = s.waf.StatusSummary()
		dbOK, _ = status["geo_db_exists"].(bool)
		if !dbOK {
			return nil, fmt.Errorf("GeoIP database install did not complete")
		}
	}

	countries, _ := status["geo_countries"].(int)
	geoEnabled, _ := status["geo_block"].(bool)
	if geoEnabled && countries > 0 && dbOK {
		return &FixResult{Success: true, Message: "国家/地区访问限制已就绪"}, nil
	}

	msg := "GeoLite2 数据库已安装。请在 WAF「国家/地区」标签页启用 GeoIP 过滤并选择要限制的国家/地区，然后点击「应用配置」。"
	if !geoEnabled {
		msg = "GeoIP 数据库已就绪。请在 WAF「国家/地区」标签页启用 GeoIP 过滤、选择国家/地区，并应用配置。"
	} else if countries == 0 {
		msg = "GeoIP 过滤已启用但未选择国家/地区。请在 WAF「国家/地区」标签页选择国家/地区并应用配置。"
	}
	return &FixResult{
		Success:      false,
		NeedsGuide:   true,
		GuideMessage: msg,
		RedirectPath: "/protection?tab=waf",
	}, nil
}

func (s *Service) fixFail2ban() (*FixResult, error) {
	if s.appstore == nil {
		return nil, fmt.Errorf("software store unavailable")
	}
	app, err := s.appstore.Get("fail2ban")
	if err == nil && app.Installed {
		if s.appstore.LiveStatus("fail2ban") != "running" {
			if err := s.appstore.ServiceAction("fail2ban", "start"); err != nil {
				return &FixResult{
					Success:      false,
					NeedsGuide:   true,
					GuideMessage: "Fail2ban 已安装但未运行，请在软件商店中启动服务",
					RedirectPath: "/software",
				}, nil
			}
		}
		return &FixResult{Success: true, Message: "Fail2ban 已在运行"}, nil
	}
	if err := s.appstore.Install("fail2ban", ""); err != nil {
		return nil, err
	}
	return &FixResult{
		Success:       true,
		InstallJobKey: "fail2ban",
		Message:       "Fail2ban 安装已开始",
	}, nil
}
