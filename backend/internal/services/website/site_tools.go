package website

import (
	"fmt"

	comprunner "github.com/luuuunet/owpanel/internal/services/composer"
)

func (s *Service) RunComposer(siteID uint, command string) (*comprunner.Result, error) {
	site, err := s.Get(siteID)
	if err != nil {
		return nil, err
	}
	if !comprunner.Installed(s.dataDir) {
		return nil, fmt.Errorf("Composer 未安装，请先在软件商店安装")
	}
	return comprunner.Run(s.dataDir, site.RootPath, command)
}
