package api

import (
	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
)

func (s *Server) registerPhpMyAdminRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/phpmyadmin/access", s.handlePhpMyAdminAccess)
	authorized.POST("/phpmyadmin/setup", s.handlePhpMyAdminSetup)
}

func (s *Server) handlePhpMyAdminAccess(c *gin.Context) {
	app, err := s.appstore.Get("phpmyadmin")
	if err != nil {
		response.Error(c, 404, "phpMyAdmin not in catalog")
		return
	}
	port := app.Port
	if port <= 0 {
		port = 888
	}
	info, err := s.phpmyadmin.AccessInfo(app.InstallPath, port, app.Installed)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, info)
}

func (s *Server) handlePhpMyAdminSetup(c *gin.Context) {
	app, err := s.appstore.Get("phpmyadmin")
	if err != nil || !app.Installed {
		response.Error(c, 400, "请先安装 phpMyAdmin")
		return
	}
	port := app.Port
	if port <= 0 {
		port = 888
	}
	if err := s.database.EnsureMySQLRootPasswordAuth(); err != nil {
		response.Error(c, 500, "MySQL 登录配置失败: "+err.Error())
		return
	}
	if err := s.phpmyadmin.Start(app.InstallPath, port); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	info, _ := s.phpmyadmin.AccessInfo(app.InstallPath, port, app.Installed)
	response.OK(c, info)
}
