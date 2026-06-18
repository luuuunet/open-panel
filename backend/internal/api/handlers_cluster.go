package api

import (
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/middleware"
	"github.com/luuuunet/owpanel/internal/services/aichat"
	"github.com/luuuunet/owpanel/internal/services/cluster"
)

func (s *Server) registerClusterRoutes(api *gin.RouterGroup) {
	api.GET("/cluster/overview", s.handleClusterOverview)
	api.GET("/cluster/nodes", s.handleListClusterNodes)
	api.POST("/cluster/nodes", s.handleCreateClusterNode)
	api.PUT("/cluster/nodes/:id", s.handleUpdateClusterNode)
	api.DELETE("/cluster/nodes/:id", s.handleDeleteClusterNode)
	api.POST("/cluster/nodes/:id/test", s.handleTestClusterNode)
	api.POST("/cluster/nodes/:id/sync", s.handleSyncClusterNode)
	api.POST("/cluster/nodes/:id/ssh/test", s.handleTestClusterNodeSSH)
	api.POST("/cluster/nodes/:id/provision", s.handleProvisionClusterNode)
	api.GET("/cluster/nodes/:id/monitor", s.handleClusterNodeMonitor)
	api.POST("/cluster/nodes/sync-all", s.handleSyncAllClusterNodes)

	api.GET("/cluster/agent/token", middleware.RequireAdmin(), s.handleGetClusterAgentToken)
	api.POST("/cluster/agent/regenerate-token", middleware.RequireAdmin(), s.handleRegenerateClusterAgentToken)

	api.GET("/cluster/join-info", s.handleClusterJoinInfo)

	api.GET("/load-balancers", s.handleListLoadBalancers)
	api.POST("/load-balancers", s.handleCreateLoadBalancer)
	api.GET("/load-balancers/:id", s.handleGetLoadBalancer)
	api.PUT("/load-balancers/:id", s.handleUpdateLoadBalancer)
	api.DELETE("/load-balancers/:id", s.handleDeleteLoadBalancer)
	api.POST("/load-balancers/:id/apply", s.handleApplyLoadBalancer)
	api.POST("/load-balancers/:id/backends", s.handleAddLoadBalancerBackend)
	api.DELETE("/load-balancers/:id/backends/:bid", s.handleDeleteLoadBalancerBackend)

	api.GET("/cluster/workflow", s.handleGetClusterWorkflow)
	api.PUT("/cluster/workflow", s.handleSaveClusterWorkflow)
	api.POST("/cluster/workflow/run", s.handleRunClusterWorkflow)
	api.POST("/cluster/workflow/sync-nodes", s.handleSyncWorkflowNodes)
	api.POST("/cluster/workflow/ai/suggest", s.handleClusterWorkflowAISuggest)

	api.POST("/cluster/quick/lb", s.handleQuickCreateLB)
	api.POST("/cluster/quick/replication", s.handleQuickReplication)
}

func (s *Server) handleClusterAgentPing(c *gin.Context) {
	token := c.GetHeader("X-Cluster-Token")
	if !s.cluster.ValidateAgentToken(token) {
		response.Error(c, http.StatusUnauthorized, "invalid cluster token")
		return
	}
	response.OK(c, s.cluster.AgentInfo())
}

func (s *Server) handleClusterOverview(c *gin.Context) {
	o, err := s.cluster.Overview()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	token, _ := s.cluster.AgentToken()
	response.OK(c, gin.H{"overview": o, "has_agent_token": token != ""})
}

func (s *Server) handleListClusterNodes(c *gin.Context) {
	list, err := s.cluster.ListNodes()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateClusterNode(c *gin.Context) {
	var req cluster.NodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	node, err := s.cluster.CreateNode(req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	s.enterprise.Recorder().FromGin(c, "cluster", "node_create", node.Name, node.Host, "info", true)
	response.OK(c, node)
}

func (s *Server) handleUpdateClusterNode(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req cluster.NodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	node, err := s.cluster.UpdateNode(uint(id), req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	s.enterprise.Recorder().FromGin(c, "cluster", "node_update", node.Name, node.Host, "info", true)
	response.OK(c, node)
}

func (s *Server) handleDeleteClusterNode(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	node, _ := s.cluster.GetNode(uint(id))
	if err := s.cluster.DeleteNode(uint(id)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	name := ""
	if node != nil {
		name = node.Name
	}
	s.enterprise.Recorder().FromGin(c, "cluster", "node_delete", name, c.Param("id"), "warn", true)
	response.Message(c, "deleted")
}

func (s *Server) handleTestClusterNode(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := s.cluster.SyncNode(uint(id)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	node, _ := s.cluster.GetNode(uint(id))
	response.OK(c, node)
}

func (s *Server) handleSyncClusterNode(c *gin.Context) {
	s.handleTestClusterNode(c)
}

func (s *Server) handleSyncAllClusterNodes(c *gin.Context) {
	s.cluster.SyncAllNodes()
	list, _ := s.cluster.ListNodes()
	response.OK(c, list)
}

func (s *Server) handleTestClusterNodeSSH(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	result, err := s.cluster.TestSSH(uint(id))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleProvisionClusterNode(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	result, err := s.cluster.ProvisionNode(uint(id))
	if result != nil {
		response.OK(c, result)
		return
	}
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Error(c, 500, "provision failed")
}

func (s *Server) handleClusterNodeMonitor(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	m, err := s.cluster.CollectMonitor(uint(id))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, m)
}

func (s *Server) handleGetClusterAgentToken(c *gin.Context) {
	token, err := s.cluster.AgentToken()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	sp := s.safePathPrefix()
	response.OK(c, gin.H{
		"token":     token,
		"safe_path": sp,
		"hint":      "Add remote nodes with this token in Agent Token field",
	})
}

func (s *Server) handleRegenerateClusterAgentToken(c *gin.Context) {
	token, err := s.cluster.RegenerateAgentToken()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"token": token})
}

func (s *Server) handleListLoadBalancers(c *gin.Context) {
	list, err := s.cluster.ListLoadBalancers()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateLoadBalancer(c *gin.Context) {
	var req cluster.LBRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	lb, err := s.cluster.CreateLoadBalancer(req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, lb)
}

func (s *Server) handleGetLoadBalancer(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	lb, err := s.cluster.GetLoadBalancer(uint(id))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	response.OK(c, lb)
}

func (s *Server) handleUpdateLoadBalancer(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req cluster.LBRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	lb, err := s.cluster.UpdateLoadBalancer(uint(id), req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, lb)
}

func (s *Server) handleDeleteLoadBalancer(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := s.cluster.DeleteLoadBalancer(uint(id)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleApplyLoadBalancer(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := s.cluster.ApplyLoadBalancer(uint(id)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	lb, _ := s.cluster.GetLoadBalancer(uint(id))
	response.OK(c, lb)
}

func (s *Server) handleAddLoadBalancerBackend(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req cluster.BackendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	b, err := s.cluster.AddBackend(uint(id), req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, b)
}

func (s *Server) handleDeleteLoadBalancerBackend(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	bid, _ := strconv.ParseUint(c.Param("bid"), 10, 64)
	if err := s.cluster.DeleteBackend(uint(id), uint(bid)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleGetClusterWorkflow(c *gin.Context) {
	wf, graph, err := s.cluster.GetWorkflow()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"workflow": wf, "graph": graph})
}

func (s *Server) handleSaveClusterWorkflow(c *gin.Context) {
	var body struct {
		Graph cluster.FlowGraph `json:"graph"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	wf, err := s.cluster.SaveWorkflow(&body.Graph)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"workflow": wf, "graph": body.Graph})
}

func (s *Server) handleRunClusterWorkflow(c *gin.Context) {
	var body struct {
		Graph *cluster.FlowGraph `json:"graph"`
	}
	_ = c.ShouldBindJSON(&body)
	graph := body.Graph
	if graph == nil {
		_, g, err := s.cluster.GetWorkflow()
		if err != nil {
			response.Error(c, 500, err.Error())
			return
		}
		graph = g
	}
	result, err := s.cluster.RunWorkflow(graph)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleSyncWorkflowNodes(c *gin.Context) {
	var body struct {
		Graph *cluster.FlowGraph `json:"graph"`
	}
	_ = c.ShouldBindJSON(&body)
	graph := body.Graph
	if graph == nil {
		_, g, _ := s.cluster.GetWorkflow()
		graph = g
	}
	graph = s.cluster.SyncGraphFromNodes(graph)
	wf, _ := s.cluster.SaveWorkflow(graph)
	response.OK(c, gin.H{"workflow": wf, "graph": graph})
}

func (s *Server) handleClusterWorkflowAISuggest(c *gin.Context) {
	var req aichat.ClusterWorkflowChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.aichat.ClusterWorkflowChat(req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleQuickCreateLB(c *gin.Context) {
	var req cluster.QuickLBRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.cluster.QuickCreateLB(req)
	if err != nil {
		if result != nil {
			response.OK(c, result)
			return
		}
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleQuickReplication(c *gin.Context) {
	var req cluster.QuickReplRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.cluster.QuickReplication(req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func requestHost(c *gin.Context) string {
	host := c.Request.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func (s *Server) handleClusterAgentRegister(c *gin.Context) {
	token := c.GetHeader("X-Cluster-Token")
	if token == "" {
		token = c.Query("token")
	}
	if !s.cluster.ValidateAgentToken(token) {
		response.Error(c, http.StatusUnauthorized, "invalid cluster token")
		return
	}
	var req cluster.AgentRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.cluster.RegisterAgentNode(req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleClusterJoinScript(c *gin.Context) {
	token := c.Query("token")
	if !s.cluster.ValidateAgentToken(token) {
		c.String(http.StatusUnauthorized, "#!/bin/bash\necho 'invalid cluster token'\nexit 1\n")
		return
	}
	role := c.DefaultQuery("role", "worker")
	apiBase := s.cluster.PanelAPIBase(requestHost(c))
	script := s.cluster.GenerateJoinScript(role, token, apiBase)
	c.Header("Content-Type", "application/x-sh")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=owpanel-join-%s.sh", role))
	c.String(http.StatusOK, script)
}

func (s *Server) handleClusterJoinInfo(c *gin.Context) {
	info, err := s.cluster.JoinInfo(requestHost(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, info)
}
