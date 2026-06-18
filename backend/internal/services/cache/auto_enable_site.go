package cache

import (
	"fmt"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/appstore"
	"github.com/luuuunet/owpanel/internal/services/webserver"
)

type AutoEnableSiteResult struct {
	WebsiteID     uint     `json:"website_id"`
	Domain        string   `json:"domain"`
	CacheEnabled  bool     `json:"cache_enabled"`
	GlobalEnabled bool     `json:"global_enabled"`
	Steps         []string `json:"steps"`
	Message       string   `json:"message"`
}

func (s *Service) SetAutoEnableHooks(apps *appstore.Service, ws *webserver.Manager, applyVhost func(uint) error) {
	s.apps = apps
	s.webserver = ws
	s.applyVhost = applyVhost
}

func (s *Service) ensureWebServer(steps *[]string) (string, error) {
	if s.webserver != nil {
		return s.webserver.EnsureRunning(steps)
	}
	if s.apps == nil {
		return "", fmt.Errorf("应用商店不可用")
	}
	s.apps.ReconcileInstalledFromSystem()
	return "", fmt.Errorf("未能安装或启动 Nginx/OpenResty，请先在软件商店安装 Web 服务器")
}

func (s *Service) AutoEnableSite(websiteID uint) (*AutoEnableSiteResult, error) {
	var site models.Website
	if err := s.db.First(&site, websiteID).Error; err != nil {
		return nil, fmt.Errorf("网站不存在")
	}
	steps := []string{fmt.Sprintf("检测站点 %s 的 CDN 缓存加速 …", site.Domain)}

	if _, err := s.ensureWebServer(&steps); err != nil {
		return nil, err
	}

	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	globalEnabled := cfg.Enabled
	if !cfg.Enabled {
		steps = append(steps, "开启全局 CDN 缓存 …")
		if _, err := s.UpdateConfig(&models.CacheConfig{Enabled: true}); err != nil {
			return nil, err
		}
		globalEnabled = true
	}

	enabled := true
	if err := s.UpdateSiteCache(websiteID, &enabled, nil, nil, nil); err != nil {
		return nil, err
	}
	steps = append(steps, "已为该站点开启缓存")

	applyRes, err := s.Apply()
	if err != nil {
		return nil, fmt.Errorf("应用缓存配置失败: %w", err)
	}
	steps = append(steps, applyRes.Message)

	if s.applyVhost != nil {
		if err := s.applyVhost(websiteID); err != nil {
			return nil, fmt.Errorf("更新站点 Nginx 配置失败: %w", err)
		}
		steps = append(steps, "站点虚拟主机已更新")
	}

	return &AutoEnableSiteResult{
		WebsiteID:     websiteID,
		Domain:        site.Domain,
		CacheEnabled:  true,
		GlobalEnabled: globalEnabled,
		Steps:         steps,
		Message:       fmt.Sprintf("站点 %s 的 CDN 缓存加速已开启", site.Domain),
	}, nil
}
