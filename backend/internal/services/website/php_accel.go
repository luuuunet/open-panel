package website

import (
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/php"
)

type PHPAccelResult struct {
	WebsiteID       uint     `json:"website_id"`
	Domain          string   `json:"domain"`
	PhpAccelEnabled bool     `json:"php_accel_enabled"`
	CacheEnabled    bool     `json:"cache_enabled"`
	PhpKey          string   `json:"php_key,omitempty"`
	Steps           []string `json:"steps"`
	Message         string   `json:"message"`
}

func phpKeyFromVersion(version string) string {
	v := strings.TrimSpace(version)
	if v == "" || v == "static" {
		return ""
	}
	return "php" + strings.ReplaceAll(v, ".", "")
}

func (s *Service) isPHPSite(site *models.Website) bool {
	return site.PHP && site.PhpVersion != "" && site.PhpVersion != "static"
}

// EnablePHPAccel turns on OPcache/realpath tuning for the site's PHP version and FastCGI page cache.
func (s *Service) EnablePHPAccel(siteID uint, restartPHP func(key string) error) (*PHPAccelResult, error) {
	site, err := s.Get(siteID)
	if err != nil {
		return nil, err
	}
	if !s.isPHPSite(site) {
		return nil, fmt.Errorf("该站点不是 PHP 站点")
	}
	key := phpKeyFromVersion(site.PhpVersion)
	if key == "" {
		return nil, fmt.Errorf("无法识别 PHP 版本")
	}

	steps := []string{fmt.Sprintf("为站点 %s 开启 PHP 加速 …", site.Domain)}

	mgr := php.NewManager(s.dataDir)
	if err := mgr.SetExtension(key, "opcache", true); err != nil {
		steps = append(steps, "OPcache 扩展配置: "+err.Error())
	} else {
		steps = append(steps, "已启用 OPcache 扩展")
	}

	directives := map[string]interface{}{
		"opcache.enable":                  "1",
		"opcache.enable_cli":              "0",
		"opcache.memory_consumption":      "128",
		"opcache.interned_strings_buffer": "16",
		"opcache.max_accelerated_files":   "10000",
		"opcache.revalidate_freq":         "2",
		"opcache.validate_timestamps":     "1",
		"realpath_cache_size":             "4096k",
		"realpath_cache_ttl":              "600",
	}
	if strings.HasPrefix(site.PhpVersion, "8.") {
		directives["opcache.jit_buffer_size"] = "64M"
		directives["opcache.jit"] = "1255"
	}
	if err := mgr.ApplyDirectives(key, directives); err != nil {
		return nil, fmt.Errorf("写入 PHP 配置失败: %w", err)
	}
	steps = append(steps, "已优化 OPcache / realpath 参数")

	if restartPHP != nil {
		if err := restartPHP(key); err != nil {
			steps = append(steps, "PHP 重启: "+err.Error())
		} else {
			steps = append(steps, "PHP 运行时已重载")
		}
	}

	cacheEnabled := site.CacheEnabled
	if s.cache != nil {
		res, err := s.cache.AutoEnableSite(siteID)
		if err != nil {
			return nil, fmt.Errorf("开启 FastCGI 页面缓存失败: %w", err)
		}
		cacheEnabled = true
		for _, step := range res.Steps {
			steps = append(steps, step)
		}
	}

	if err := s.db.Model(site).Updates(map[string]interface{}{
		"php_accel_enabled": true,
		"cache_enabled":     cacheEnabled,
	}).Error; err != nil {
		return nil, err
	}

	return &PHPAccelResult{
		WebsiteID:       siteID,
		Domain:          site.Domain,
		PhpAccelEnabled: true,
		CacheEnabled:    cacheEnabled,
		PhpKey:          key,
		Steps:           steps,
		Message:         fmt.Sprintf("站点 %s 的 PHP 加速已开启（OPcache + FastCGI 缓存）", site.Domain),
	}, nil
}

// TogglePHPAccel enables or disables PHP acceleration for a website.
func (s *Service) TogglePHPAccel(siteID uint, enabled *bool, restartPHP func(key string) error) (*PHPAccelResult, error) {
	site, err := s.Get(siteID)
	if err != nil {
		return nil, err
	}
	on := !site.PhpAccelEnabled
	if enabled != nil {
		on = *enabled
	}
	if on {
		return s.EnablePHPAccel(siteID, restartPHP)
	}
	if !s.isPHPSite(site) {
		return nil, fmt.Errorf("该站点不是 PHP 站点")
	}
	updates := map[string]interface{}{"php_accel_enabled": false}
	if err := s.db.Model(site).Updates(updates).Error; err != nil {
		return nil, err
	}
	return &PHPAccelResult{
		WebsiteID:       siteID,
		Domain:          site.Domain,
		PhpAccelEnabled: false,
		CacheEnabled:    site.CacheEnabled,
		Steps:           []string{"已关闭站点 PHP 加速"},
		Message:         fmt.Sprintf("站点 %s 的 PHP 加速已关闭（CDN/FastCGI 缓存仍保留，可在缓存页单独管理）", site.Domain),
	}, nil
}
