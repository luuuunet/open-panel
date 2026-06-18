package api

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/aichat"
)

type terminalTarget struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	User        string `json:"user"`
	NodeID      uint   `json:"node_id,omitempty"`
	AssetID     uint   `json:"asset_id,omitempty"`
	HasPassword bool   `json:"has_password"`
	IsLocal     bool   `json:"is_local"`
}

func (s *Server) handleTerminalTargets(c *gin.Context) {
	targets := []terminalTarget{
		{
			ID: "local", Label: "本机 (127.0.0.1)", Host: "127.0.0.1", Port: 22, User: "root",
			IsLocal: true,
		},
	}
	nodes, _ := s.cluster.ListNodes()
	for _, n := range nodes {
		if n.IsLocal {
			continue
		}
		host := strings.TrimSpace(n.SSHHost)
		if host == "" {
			host = n.Host
		}
		port := n.SSHPort
		if port <= 0 {
			port = 22
		}
		user := strings.TrimSpace(n.SSHUser)
		if user == "" {
			user = "root"
		}
		targets = append(targets, terminalTarget{
			ID: fmt.Sprintf("node-%d", n.ID), Label: n.Name + " (" + host + ")",
			Host: host, Port: port, User: user, NodeID: n.ID,
			HasPassword: n.HasSSHPassword, IsLocal: false,
		})
	}
	response.OK(c, targets)
}

func (s *Server) handleTerminalKeys(c *gin.Context) {
	list, err := s.sshmgr.List()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateTerminalKey(c *gin.Context) {
	var req struct {
		Name       string `json:"name"`
		PublicKey  string `json:"public_key"`
		PrivateKey string `json:"private_key"`
		Remark     string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		response.Error(c, 400, "名称不能为空")
		return
	}
	if strings.TrimSpace(req.PublicKey) == "" && strings.TrimSpace(req.PrivateKey) == "" {
		response.Error(c, 400, "请至少填写公钥或私钥")
		return
	}
	key := models.SSHKey{
		Name: req.Name, PublicKey: req.PublicKey, PrivateKey: req.PrivateKey, Remark: req.Remark,
	}
	if err := s.sshmgr.Create(&key); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	key.HasPrivate = strings.TrimSpace(key.PrivateKey) != ""
	key.PrivateKey = ""
	response.OK(c, key)
}

func (s *Server) handleDeleteTerminalKey(c *gin.Context) {
	if err := s.sshmgr.Delete(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleTerminalAIChat(c *gin.Context) {
	var req aichat.TerminalChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.aichat.TerminalChat(req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) registerTerminalRoutes(api *gin.RouterGroup) {
	api.GET("/terminal/targets", s.handleTerminalTargets)
	api.GET("/terminal/keys", s.handleTerminalKeys)
	api.POST("/terminal/keys", s.handleCreateTerminalKey)
	api.DELETE("/terminal/keys/:id", s.handleDeleteTerminalKey)
	api.POST("/terminal/ai/chat", s.handleTerminalAIChat)
}
