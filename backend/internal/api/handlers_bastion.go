package api

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/middleware"
	"github.com/luuuunet/owpanel/internal/services/bastion"
	"github.com/luuuunet/owpanel/internal/services/terminal"
)

func (s *Server) registerBastionRoutes(api *gin.RouterGroup) {
	bastionAPI := api.Group("")
	bastionAPI.Use(middleware.RequirePermission("bastion"))

	bastionAPI.GET("/bastion/assets", s.handleListBastionAssets)
	bastionAPI.GET("/bastion/connect-targets", s.handleBastionConnectTargets)
	bastionAPI.GET("/bastion/accounts", s.handleListBastionAccounts)
	bastionAPI.GET("/bastion/accounts/rotation-logs", s.handleListBastionRotationLogs)
	bastionAPI.GET("/bastion/sessions", s.handleListBastionSessions)
	bastionAPI.GET("/bastion/sessions/:id", s.handleGetBastionSession)
	bastionAPI.GET("/bastion/sessions/:id/log", s.handleBastionSessionLog)
	bastionAPI.GET("/bastion/sessions/:id/download", s.handleBastionSessionDownload)
	bastionAPI.GET("/bastion/sessions/:id/commands", s.handleBastionSessionCommands)

	admin := bastionAPI.Group("")
	admin.Use(middleware.RequireAdmin())
	{
		admin.POST("/bastion/assets", s.handleCreateBastionAsset)
		admin.PUT("/bastion/assets/:id", s.handleUpdateBastionAsset)
		admin.DELETE("/bastion/assets/:id", s.handleDeleteBastionAsset)
		admin.POST("/bastion/assets/import/cluster/:nodeId", s.handleImportBastionFromCluster)

		admin.GET("/bastion/groups", s.handleListBastionGroups)
		admin.POST("/bastion/groups", s.handleCreateBastionGroup)
		admin.PUT("/bastion/groups/:id", s.handleUpdateBastionGroup)
		admin.DELETE("/bastion/groups/:id", s.handleDeleteBastionGroup)

		admin.GET("/bastion/permissions", s.handleListBastionPermissions)
		admin.POST("/bastion/permissions", s.handleCreateBastionPermission)
		admin.DELETE("/bastion/permissions/:id", s.handleDeleteBastionPermission)

		admin.GET("/bastion/command-policy", s.handleGetBastionCommandPolicy)
		admin.PUT("/bastion/command-policy", s.handleUpdateBastionCommandPolicy)
		admin.GET("/bastion/command-audits", s.handleListBastionCommandAudits)

		admin.GET("/bastion/active-sessions", s.handleListBastionActiveSessions)
		admin.POST("/bastion/active-sessions/:key/kill", s.handleKillBastionSession)

		admin.GET("/bastion/accounts/vault/export", s.handleExportBastionVault)
		admin.POST("/bastion/accounts/vault/import", s.handleImportBastionVault)
		admin.POST("/bastion/accounts/discover/:assetId", s.handleDiscoverBastionAccounts)
		admin.POST("/bastion/accounts/rotate-batch", s.handleRotateBastionBatch)
		admin.POST("/bastion/accounts", s.handleCreateBastionAccount)
		admin.PUT("/bastion/accounts/:id", s.handleUpdateBastionAccount)
		admin.DELETE("/bastion/accounts/:id", s.handleDeleteBastionAccount)
		admin.POST("/bastion/accounts/:id/rotate", s.handleRotateBastionAccount)
		admin.POST("/bastion/accounts/:id/push", s.handlePushBastionAccount)
		admin.POST("/bastion/accounts/:id/test", s.handleTestBastionAccount)

		// Ops center (自动化运维) — templates/jobs/runs admin-only
		admin.GET("/bastion/ops/templates", s.handleListOpsTemplates)
		admin.POST("/bastion/ops/templates", s.handleCreateOpsTemplate)
		admin.PUT("/bastion/ops/templates/:id", s.handleUpdateOpsTemplate)
		admin.DELETE("/bastion/ops/templates/:id", s.handleDeleteOpsTemplate)

		admin.GET("/bastion/ops/jobs", s.handleListOpsJobs)
		admin.POST("/bastion/ops/jobs", s.handleCreateOpsJob)
		admin.PUT("/bastion/ops/jobs/:id", s.handleUpdateOpsJob)
		admin.DELETE("/bastion/ops/jobs/:id", s.handleDeleteOpsJob)
		admin.POST("/bastion/ops/jobs/:id/run", s.handleRunOpsJob)
		admin.GET("/bastion/ops/jobs/:id/runs", s.handleListOpsJobRuns)

		admin.GET("/bastion/ops/runs/:id", s.handleGetOpsRun)
	}

	// Adhoc ops: non-admin may run on authorized assets; history scoped by user
	bastionAPI.POST("/bastion/ops/adhoc", s.handleOpsAdhoc)
	bastionAPI.GET("/bastion/ops/adhoc/history", s.handleOpsAdhocHistory)

	s.registerBastionPamRoutes(bastionAPI, admin)
}

func bastionUser(c *gin.Context) (uint, string, string) {
	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")
	role, _ := c.Get("role")
	uid, _ := userID.(uint)
	un, _ := username.(string)
	r, _ := role.(string)
	return uid, un, r
}

func (s *Server) handleListBastionAssets(c *gin.Context) {
	uid, _, role := bastionUser(c)
	list, err := s.bastion.ListAssets(uid, role)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleBastionConnectTargets(c *gin.Context) {
	uid, _, role := bastionUser(c)
	list, err := s.bastion.ConnectTargets(uid, role)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateBastionAsset(c *gin.Context) {
	var in bastion.AssetInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	a, err := s.bastion.CreateAsset(in)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, a)
}

func (s *Server) handleUpdateBastionAsset(c *gin.Context) {
	var in bastion.AssetInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	a, err := s.bastion.UpdateAsset(parseID(c), in)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, a)
}

func (s *Server) handleDeleteBastionAsset(c *gin.Context) {
	if err := s.bastion.DeleteAsset(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleImportBastionFromCluster(c *gin.Context) {
	nodeID := parseIDParam(c.Param("nodeId"))
	if nodeID == 0 {
		response.Error(c, 400, "invalid node id")
		return
	}
	a, err := s.bastion.ImportFromClusterNode(nodeID)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, a)
}

func (s *Server) handleListBastionGroups(c *gin.Context) {
	list, err := s.bastion.ListGroups()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateBastionGroup(c *gin.Context) {
	var req struct {
		Name     string `json:"name"`
		Remark   string `json:"remark"`
		ParentID *uint  `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	g, err := s.bastion.CreateGroup(req.Name, req.Remark, req.ParentID)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, g)
}

func (s *Server) handleUpdateBastionGroup(c *gin.Context) {
	var req struct {
		Name   string `json:"name"`
		Remark string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.bastion.UpdateGroup(parseID(c), req.Name, req.Remark); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Message(c, "updated")
}

func (s *Server) handleDeleteBastionGroup(c *gin.Context) {
	if err := s.bastion.DeleteGroup(parseID(c)); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleListBastionPermissions(c *gin.Context) {
	list, err := s.bastion.ListPermissions()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateBastionPermission(c *gin.Context) {
	var in bastion.PermissionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	uid, _, _ := bastionUser(c)
	p, err := s.bastion.CreatePermission(in, uid)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, p)
}

func (s *Server) handleDeleteBastionPermission(c *gin.Context) {
	if err := s.bastion.DeletePermission(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleListBastionSessions(c *gin.Context) {
	uid, _, role := bastionUser(c)
	q := c.Query("q")
	list, err := s.bastion.ListSessions(uid, role, q, 100)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleGetBastionSession(c *gin.Context) {
	uid, _, role := bastionUser(c)
	rec, err := s.bastion.GetSession(parseID(c), uid, role)
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	response.OK(c, rec)
}

func (s *Server) handleBastionSessionLog(c *gin.Context) {
	uid, _, role := bastionUser(c)
	text, err := s.bastion.ReadSessionLog(parseID(c), uid, role)
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	response.OK(c, gin.H{"log": text})
}

func (s *Server) handleBastionSessionDownload(c *gin.Context) {
	uid, _, role := bastionUser(c)
	path, name, err := s.bastion.SessionLogPath(parseID(c), uid, role)
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	c.FileAttachment(path, name)
}

func (s *Server) handleBastionSessionCommands(c *gin.Context) {
	uid, _, role := bastionUser(c)
	cmds, err := s.bastion.GetSessionCommands(parseID(c), uid, role)
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	response.OK(c, cmds)
}

func (s *Server) handleGetBastionCommandPolicy(c *gin.Context) {
	response.OK(c, s.bastion.LoadCommandPolicy())
}

func (s *Server) handleUpdateBastionCommandPolicy(c *gin.Context) {
	var p bastion.CommandPolicy
	if err := c.ShouldBindJSON(&p); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.bastion.SaveCommandPolicy(p); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, p)
}

func (s *Server) handleListBastionCommandAudits(c *gin.Context) {
	list, err := s.bastion.ListCommandAudits(100)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleListBastionActiveSessions(c *gin.Context) {
	list, err := s.bastion.EnrichActiveWithDB()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleKillBastionSession(c *gin.Context) {
	key := c.Param("key")
	if err := s.bastion.KillSession(key); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Message(c, "killed")
}

func (s *Server) terminalHandlerOptions(c *gin.Context) terminal.HandlerOptions {
	uid, un, role := bastionUser(c)
	ctx := terminal.SessionContext{UserID: uid, Username: un, Role: role}
	opts := terminal.HandlerOptions{
		Context: ctx,
		Resolver: terminal.Resolver{
			ResolveNode: func(nodeID uint) (terminal.Config, error) {
				if role != "admin" {
					return terminal.Config{}, fmt.Errorf("需要管理员权限")
				}
				node, err := s.cluster.GetNode(nodeID)
				if err != nil {
					return terminal.Config{}, fmt.Errorf("节点不存在")
				}
				host := strings.TrimSpace(node.SSHHost)
				if host == "" {
					host = node.Host
				}
				port := node.SSHPort
				if port <= 0 {
					port = 22
				}
				user := strings.TrimSpace(node.SSHUser)
				if user == "" {
					user = "root"
				}
				cfg := terminal.Config{Host: host, Port: port, User: user}
				if strings.TrimSpace(node.SSHPassword) != "" {
					cfg.Password = node.SSHPassword
					cfg.AuthMethod = "password"
				}
				return cfg, nil
			},
			ResolveKey: func(keyID uint) (string, error) {
				if role != "admin" {
					return "", fmt.Errorf("需要管理员权限")
				}
				return s.sshmgr.PrivateKey(keyID)
			},
			ResolveAsset: func(assetID uint, tctx terminal.SessionContext) (terminal.Config, error) {
				host, port, user, password, pk, auth, err := s.bastion.ResolveAssetConfig(assetID, tctx.AccountID, tctx.UserID, tctx.Role)
				if err != nil {
					return terminal.Config{}, err
				}
				return terminal.Config{
					Host: host, Port: port, User: user,
					Password: password, PrivateKey: pk, AuthMethod: auth,
				}, nil
			},
		},
		Hooks: &terminal.SessionHooks{
			OnConnected: func(cfg terminal.Config, tctx terminal.SessionContext) (terminal.SessionRecorder, error) {
				assetID := tctx.AssetID
				assetName := ""
				if assetID > 0 {
					if a, err := s.bastion.GetAsset(assetID); err == nil {
						assetName = a.Name
					}
				}
				readonly := false
				if assetID > 0 && role != "admin" {
					if p, _ := s.bastion.GetUserAssetPermission(uid, assetID); p == "readonly" {
						readonly = true
					}
				}
				rec, err := s.bastion.StartRecorder(uid, un, assetID, tctx.AccountID, assetName, cfg.Host, cfg.Port, readonly)
				if err != nil {
					return nil, err
				}
				return rec, nil
			},
		},
		BeforeConnect: func(msg terminal.ConnectMessage, tctx terminal.SessionContext) error {
			if role == "admin" {
				return nil
			}
			if msg.AssetID == 0 {
				return fmt.Errorf("非管理员请通过堡垒机资产连接")
			}
			return nil
		},
	}
	return opts
}

func (s *Server) handleTerminalWS(c *gin.Context) {
	opts := s.terminalHandlerOptions(c)
	terminal.HandleWebSocket(c.Writer, c.Request, opts)
}

func (s *Server) handleTerminalWSAuth(c *gin.Context) {
	s.handleTerminalWS(c)
}

func (s *Server) handleListOpsTemplates(c *gin.Context) {
	list, err := s.bastion.ListTemplates()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateOpsTemplate(c *gin.Context) {
	var in bastion.TemplateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	tpl, err := s.bastion.CreateTemplate(in)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, tpl)
}

func (s *Server) handleUpdateOpsTemplate(c *gin.Context) {
	var in bastion.TemplateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	tpl, err := s.bastion.UpdateTemplate(parseID(c), in)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, tpl)
}

func (s *Server) handleDeleteOpsTemplate(c *gin.Context) {
	if err := s.bastion.DeleteTemplate(parseID(c)); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleListOpsJobs(c *gin.Context) {
	uid, _, role := bastionUser(c)
	list, err := s.bastion.ListJobs(uid, role)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateOpsJob(c *gin.Context) {
	var in bastion.JobInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	job, err := s.bastion.CreateJob(in)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, job)
}

func (s *Server) handleUpdateOpsJob(c *gin.Context) {
	var in bastion.JobInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	job, err := s.bastion.UpdateJob(parseID(c), in)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, job)
}

func (s *Server) handleDeleteOpsJob(c *gin.Context) {
	if err := s.bastion.DeleteJob(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleRunOpsJob(c *gin.Context) {
	uid, un, _ := bastionUser(c)
	run, err := s.bastion.RunJob(parseID(c), uid, "manual", un)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, run)
}

func (s *Server) handleListOpsJobRuns(c *gin.Context) {
	list, err := s.bastion.ListJobRuns(parseID(c), 50)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleGetOpsRun(c *gin.Context) {
	run, err := s.bastion.GetRun(parseID(c))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	response.OK(c, run)
}

func (s *Server) handleOpsAdhoc(c *gin.Context) {
	var in bastion.AdhocInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	uid, un, role := bastionUser(c)
	run, err := s.bastion.RunAdhoc(in, uid, role, un)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, run)
}

func (s *Server) handleOpsAdhocHistory(c *gin.Context) {
	uid, _, role := bastionUser(c)
	list, err := s.bastion.ListAdhocHistory(uid, role, 30)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func parseAssetIDQuery(c *gin.Context) uint {
	v, _ := strconv.ParseUint(strings.TrimSpace(c.Query("asset_id")), 10, 64)
	return uint(v)
}

func (s *Server) handleListBastionAccounts(c *gin.Context) {
	uid, _, role := bastionUser(c)
	list, err := s.bastion.ListAccounts(uid, role, parseAssetIDQuery(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleListBastionRotationLogs(c *gin.Context) {
	list, err := s.bastion.ListRotationLogs(200)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateBastionAccount(c *gin.Context) {
	var in bastion.AccountInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	a, err := s.bastion.CreateAccount(in)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, a)
}

func (s *Server) handleUpdateBastionAccount(c *gin.Context) {
	var in bastion.AccountInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	a, err := s.bastion.UpdateAccount(parseID(c), in)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, a)
}

func (s *Server) handleDeleteBastionAccount(c *gin.Context) {
	if err := s.bastion.DeleteAccount(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleDiscoverBastionAccounts(c *gin.Context) {
	assetID := parseIDParam(c.Param("assetId"))
	if assetID == 0 {
		response.Error(c, 400, "invalid asset id")
		return
	}
	list, err := s.bastion.DiscoverAccounts(assetID)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleRotateBastionAccount(c *gin.Context) {
	a, err := s.bastion.RotateAccount(parseID(c))
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, a)
}

func (s *Server) handleRotateBastionBatch(c *gin.Context) {
	var in bastion.RotateBatchInput
	_ = c.ShouldBindJSON(&in)
	ok, fail, errs := s.bastion.RotateBatch(in.AccountIDs)
	response.OK(c, gin.H{"success": ok, "failed": fail, "errors": errs})
}

func (s *Server) handlePushBastionAccount(c *gin.Context) {
	var in bastion.PushAccountInput
	_ = c.ShouldBindJSON(&in)
	if err := s.bastion.PushAccount(parseID(c), in); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Message(c, "pushed")
}

func (s *Server) handleTestBastionAccount(c *gin.Context) {
	ok, msg, err := s.bastion.TestAccount(parseID(c))
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"success": ok, "message": msg})
}

func (s *Server) handleExportBastionVault(c *gin.Context) {
	data, err := s.bastion.ExportVault()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	c.Header("Content-Disposition", "attachment; filename=bastion-vault-backup.json")
	c.Data(200, "application/json", data)
}

func (s *Server) handleImportBastionVault(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	imported, skipped, err := s.bastion.ImportVault(body)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"imported": imported, "skipped": skipped})
}
