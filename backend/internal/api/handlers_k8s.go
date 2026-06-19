package api

import (
	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
)

func (s *Server) registerK8sRoutes(admin *gin.RouterGroup) {
	admin.GET("/k8s/dashboard", s.handleK8sDashboard)
	admin.GET("/k8s/status", s.handleK8sStatus)
	admin.GET("/k8s/join-info", s.handleK8sJoinInfo)
	admin.GET("/k8s/nodes", s.handleK8sNodes)
	admin.GET("/k8s/pods", s.handleK8sPods)
	admin.GET("/k8s/deployments", s.handleK8sDeployments)
	admin.GET("/k8s/namespaces", s.handleK8sNamespaces)
	admin.POST("/k8s/install", s.handleK8sInstall)
	admin.POST("/k8s/wizard", s.handleK8sWizard)
}

func (s *Server) handleK8sDashboard(c *gin.Context) {
	d, err := s.k8s.Dashboard()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, d)
}

func (s *Server) handleK8sStatus(c *gin.Context) {
	st, err := s.k8s.Status()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, st)
}

func (s *Server) handleK8sJoinInfo(c *gin.Context) {
	info, err := s.k8s.JoinInfo()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, info)
}

func (s *Server) handleK8sNodes(c *gin.Context) {
	items, err := s.k8s.ListNodes()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, items)
}

func (s *Server) handleK8sPods(c *gin.Context) {
	items, err := s.k8s.ListPods()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, items)
}

func (s *Server) handleK8sDeployments(c *gin.Context) {
	items, err := s.k8s.ListDeployments()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, items)
}

func (s *Server) handleK8sNamespaces(c *gin.Context) {
	items, err := s.k8s.ListNamespaces()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, items)
}

func (s *Server) handleK8sInstall(c *gin.Context) {
	res, err := s.k8s.Install()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, res)
}

func (s *Server) handleK8sWizard(c *gin.Context) {
	var req struct {
		DeploySample bool `json:"deploy_sample"`
	}
	_ = c.ShouldBindJSON(&req)
	res, err := s.k8s.RunWizard(req.DeploySample)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, res)
}
