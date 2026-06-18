package api

import (
	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/services/ssl"
)

func (s *Server) registerSSLRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/ssl", s.handleListSSL)
	authorized.GET("/ssl/status", s.handleSSLStatus)
	authorized.GET("/ssl/:id", s.handleGetSSL)
	authorized.POST("/ssl", s.handleRequestSSL)
	authorized.POST("/ssl/upload", s.handleUploadSSL)
	authorized.POST("/ssl/renew-all", s.handleRenewAllSSL)
	authorized.POST("/ssl/:id/renew", s.handleRenewSSL)
	authorized.POST("/ssl/:id/deploy", s.handleDeploySSL)
	authorized.DELETE("/ssl/:id", s.handleDeleteSSL)
}

func (s *Server) handleListSSL(c *gin.Context) {
	list, err := s.ssl.List()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleSSLStatus(c *gin.Context) {
	st, err := s.ssl.StatusSummary()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, st)
}

func (s *Server) handleGetSSL(c *gin.Context) {
	cert, err := s.ssl.Get(parseID(c))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	response.OK(c, cert)
}

func (s *Server) handleRequestSSL(c *gin.Context) {
	var req ssl.IssueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if req.Deploy {
		s.ssl.SetDeployHook(s.website.DeploySSLForDomain)
	}
	cert, err := s.ssl.Issue(&req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, cert)
}

func (s *Server) handleUploadSSL(c *gin.Context) {
	var req ssl.UploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if req.Deploy {
		s.ssl.SetDeployHook(s.website.DeploySSLForDomain)
	}
	cert, err := s.ssl.Upload(&req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, cert)
}

func (s *Server) handleRenewSSL(c *gin.Context) {
	s.ssl.SetDeployHook(s.website.DeploySSLForDomain)
	cert, err := s.ssl.Renew(parseID(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, cert)
}

func (s *Server) handleRenewAllSSL(c *gin.Context) {
	s.ssl.SetDeployHook(s.website.DeploySSLForDomain)
	n, failed, err := s.ssl.RenewAll()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"renewed": n, "failed": failed})
}

func (s *Server) handleDeploySSL(c *gin.Context) {
	cert, err := s.ssl.Get(parseID(c))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	if err := s.website.DeploySSLForDomain(cert.Domain); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deployed")
}

func (s *Server) handleDeleteSSL(c *gin.Context) {
	if err := s.ssl.Delete(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}
