package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/services/docker"
)

func (s *Server) registerDockerExtraRoutes(dockerGroup *gin.RouterGroup) {
	dockerGroup.GET("/docker/containers/:id", s.handleInspectContainer)
	dockerGroup.GET("/docker/containers/:id/logs", s.handleContainerLogs)
	dockerGroup.GET("/docker/containers/:id/domain", s.handleGetContainerDomain)
	dockerGroup.PUT("/docker/containers/:id/domain", s.handleBindContainerDomain)
	dockerGroup.DELETE("/docker/containers/:id/domain", s.handleUnbindContainerDomain)
	dockerGroup.POST("/docker/containers/:id/restart", s.handleRestartContainer)
	dockerGroup.POST("/docker/containers/:id/recreate", s.handleRecreateContainer)
	dockerGroup.POST("/docker/containers/run", s.handleRunContainer)
	dockerGroup.POST("/docker/images/pull", s.handlePullImage)
	dockerGroup.DELETE("/docker/images/:id", s.handleRemoveImage)
	dockerGroup.POST("/docker/images/prune", s.handlePruneImages)
	dockerGroup.POST("/docker/volumes", s.handleCreateVolume)
	dockerGroup.DELETE("/docker/volumes/:name", s.handleRemoveVolume)
	dockerGroup.POST("/docker/volumes/prune", s.handlePruneVolumes)
	dockerGroup.POST("/docker/networks", s.handleCreateNetwork)
	dockerGroup.DELETE("/docker/networks/:id", s.handleRemoveNetwork)
	dockerGroup.POST("/docker/networks/prune", s.handlePruneNetworks)
}

func (s *Server) handleInspectContainer(c *gin.Context) {
	detail, err := s.docker.InspectContainer(c.Param("id"))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	response.OK(c, detail)
}

func (s *Server) handleContainerLogs(c *gin.Context) {
	tail, _ := strconv.Atoi(c.DefaultQuery("tail", "300"))
	logs, err := s.docker.ContainerLogs(c.Param("id"), tail)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"content": logs})
}

func (s *Server) handleRestartContainer(c *gin.Context) {
	if err := s.docker.Restart(c.Param("id")); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "restarted")
}

func (s *Server) handleRecreateContainer(c *gin.Context) {
	var req docker.RecreateContainerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	newID, err := s.docker.RecreateContainer(c.Param("id"), req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"id": newID, "message": "recreated"})
}

func (s *Server) handleRunContainer(c *gin.Context) {
	var req docker.RunContainerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	id, err := s.docker.RunContainer(req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"id": id})
}

func (s *Server) handlePullImage(c *gin.Context) {
	var req struct {
		Image string `json:"image"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.docker.PullImage(req.Image); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "pulled")
}

func (s *Server) handleRemoveImage(c *gin.Context) {
	force := c.Query("force") == "1" || c.Query("force") == "true"
	if err := s.docker.RemoveImage(c.Param("id"), force); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "removed")
}

func (s *Server) handlePruneImages(c *gin.Context) {
	msg, err := s.docker.PruneImages()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"message": msg})
}

func (s *Server) handleCreateVolume(c *gin.Context) {
	var req struct {
		Name   string `json:"name"`
		Driver string `json:"driver"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.docker.CreateVolume(req.Name, req.Driver); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "created")
}

func (s *Server) handleRemoveVolume(c *gin.Context) {
	force := c.Query("force") == "1" || c.Query("force") == "true"
	if err := s.docker.RemoveVolume(c.Param("name"), force); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "removed")
}

func (s *Server) handlePruneVolumes(c *gin.Context) {
	msg, err := s.docker.PruneVolumes()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"message": msg})
}

func (s *Server) handleCreateNetwork(c *gin.Context) {
	var req struct {
		Name    string `json:"name"`
		Driver  string `json:"driver"`
		Subnet  string `json:"subnet"`
		Gateway string `json:"gateway"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.docker.CreateNetwork(docker.CreateNetworkOpts{
		Name:    req.Name,
		Driver:  req.Driver,
		Subnet:  req.Subnet,
		Gateway: req.Gateway,
	}); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "created")
}

func (s *Server) handleRemoveNetwork(c *gin.Context) {
	if err := s.docker.RemoveNetwork(c.Param("id")); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "removed")
}

func (s *Server) handlePruneNetworks(c *gin.Context) {
	msg, err := s.docker.PruneNetworks()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"message": msg})
}

func (s *Server) handleGetContainerDomain(c *gin.Context) {
	binding, err := s.docker.GetBinding(c.Param("id"))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	if binding == nil {
		response.OK(c, gin.H{"domain": "", "host_port": 0})
		return
	}
	response.OK(c, gin.H{
		"domain":    binding.Domain,
		"host_port": binding.HostPort,
		"access_url": "http://" + binding.Domain,
	})
}

func (s *Server) handleBindContainerDomain(c *gin.Context) {
	var req docker.BindDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	binding, err := s.docker.BindDomain(c.Param("id"), req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{
		"domain":     binding.Domain,
		"host_port":  binding.HostPort,
		"access_url": "http://" + binding.Domain,
		"message":    "bound",
	})
}

func (s *Server) handleUnbindContainerDomain(c *gin.Context) {
	if err := s.docker.UnbindDomain(c.Param("id")); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "unbound")
}
