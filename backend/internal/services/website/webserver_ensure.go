package website

import (
	"fmt"
	"log"
)

// ensureWebServerForSite installs, configures, and starts Nginx/OpenResty before publishing a site.
func (s *Service) ensureWebServerForSite() error {
	if s.ws == nil {
		return fmt.Errorf("Web 服务器组件不可用，请先在软件商店安装 Nginx")
	}
	steps := []string{}
	key, err := s.ws.EnsureRunning(&steps)
	if err != nil {
		return err
	}
	if err := s.ws.Bootstrap(key); err != nil {
		return fmt.Errorf("Web 服务器配置失败: %w", err)
	}
	if err := s.ws.Reload(key); err != nil {
		log.Printf("[website] reload webserver %s after bootstrap: %v", key, err)
	}
	return nil
}
