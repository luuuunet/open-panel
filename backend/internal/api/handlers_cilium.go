package api

import (
	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/services/cilium"
)

func (s *Server) registerCiliumRoutes(admin *gin.RouterGroup) {
	admin.GET("/cilium/dashboard", s.handleCiliumDashboard)
	admin.GET("/cilium/status", s.handleCiliumStatus)
	admin.GET("/cilium/config", s.handleGetCiliumConfig)
	admin.PATCH("/cilium/config", s.handlePatchCiliumConfig)
	admin.POST("/cilium/install-stack", s.handleCiliumInstallStack)
	admin.POST("/cilium/apply", s.handleCiliumApply)
	admin.POST("/cilium/wizard", s.handleCiliumWizard)
	admin.POST("/cilium/audit-mode", s.handleCiliumAuditMode)
	admin.GET("/cilium/policies", s.handleCiliumListPolicies)
	admin.GET("/cilium/presets", s.handleCiliumPresets)
	admin.POST("/cilium/policies", s.handleCiliumApplyPolicy)
	admin.POST("/cilium/policies/preset/:key", s.handleCiliumApplyPreset)
	admin.POST("/cilium/policies/baseline", s.handleCiliumApplyBaseline)
	admin.DELETE("/cilium/policies/:name", s.handleCiliumDeletePolicy)
	admin.GET("/cilium/policy-template/ssh", s.handleCiliumSSHTemplate)
}

func (s *Server) ciliumPanelPort() string {
	all, err := s.settings.GetAll()
	if err != nil {
		return "8888"
	}
	if p := all["panel_port"]; p != "" {
		return p
	}
	return "8888"
}

func (s *Server) handleCiliumDashboard(c *gin.Context) {
	d, err := s.cilium.Dashboard(s.ciliumPanelPort())
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, d)
}

func (s *Server) handleCiliumStatus(c *gin.Context) {
	st, err := s.cilium.Status()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, st)
}

func (s *Server) handleGetCiliumConfig(c *gin.Context) {
	cfg, err := s.cilium.GetConfig()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, cfg)
}

func (s *Server) handlePatchCiliumConfig(c *gin.Context) {
	var req struct {
		HostFirewallEnabled *bool  `json:"host_firewall_enabled"`
		HubbleEnabled       *bool  `json:"hubble_enabled"`
		HubbleUIEnabled     *bool  `json:"hubble_ui_enabled"`
		AuditMode           *bool  `json:"audit_mode"`
		NetworkDevice       string `json:"network_device"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	cfg, err := s.cilium.GetConfig()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	patch := *cfg
	if req.HostFirewallEnabled != nil {
		patch.HostFirewallEnabled = *req.HostFirewallEnabled
	}
	if req.HubbleEnabled != nil {
		patch.HubbleEnabled = *req.HubbleEnabled
	}
	if req.HubbleUIEnabled != nil {
		patch.HubbleUIEnabled = *req.HubbleUIEnabled
	}
	if req.AuditMode != nil {
		patch.AuditMode = *req.AuditMode
	}
	if req.NetworkDevice != "" {
		patch.NetworkDevice = req.NetworkDevice
	}
	updated, err := s.cilium.UpdateConfig(&patch)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, updated)
}

func (s *Server) handleCiliumInstallStack(c *gin.Context) {
	var req struct {
		InstallK3s    bool `json:"install_k3s"`
		InstallCilium bool `json:"install_cilium"`
	}
	_ = c.ShouldBindJSON(&req)
	if !req.InstallK3s && !req.InstallCilium {
		req.InstallK3s = true
		req.InstallCilium = true
	}
	res, err := s.cilium.InstallStack(req.InstallK3s, req.InstallCilium)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, res)
}

func (s *Server) handleCiliumApply(c *gin.Context) {
	cfg, err := s.cilium.ApplyHostFirewall()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"config": cfg, "message": "Cilium 配置已应用"})
}

func (s *Server) handleCiliumWizard(c *gin.Context) {
	res, err := s.cilium.RunWizard(s.ciliumPanelPort())
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, res)
}

func (s *Server) handleCiliumAuditMode(c *gin.Context) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	cfg, err := s.cilium.SetAuditMode(req.Enabled)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"config": cfg, "message": "审计模式已更新"})
}

func (s *Server) handleCiliumListPolicies(c *gin.Context) {
	items, err := s.cilium.ListPolicies()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, items)
}

func (s *Server) handleCiliumPresets(c *gin.Context) {
	items, err := s.cilium.PresetsWithStatus(s.ciliumPanelPort())
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, items)
}

func (s *Server) handleCiliumApplyPolicy(c *gin.Context) {
	var req struct {
		YAML string `json:"yaml"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	msg, err := s.cilium.ApplyPolicyYAML(req.YAML)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"message": msg})
}

func (s *Server) handleCiliumApplyPreset(c *gin.Context) {
	msg, err := s.cilium.ApplyPreset(c.Param("key"), s.ciliumPanelPort())
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"message": msg})
}

func (s *Server) handleCiliumApplyBaseline(c *gin.Context) {
	msgs, err := s.cilium.ApplyBaselinePresets(s.ciliumPanelPort())
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"messages": msgs, "message": "基础策略已应用"})
}

func (s *Server) handleCiliumDeletePolicy(c *gin.Context) {
	kind := c.Query("kind")
	ns := c.Query("namespace")
	if err := s.cilium.DeletePolicy(kind, ns, c.Param("name")); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "deleted"})
}

func (s *Server) handleCiliumSSHTemplate(c *gin.Context) {
	response.OK(c, gin.H{"yaml": cilium.DefaultHostSSHPolicy()})
}
