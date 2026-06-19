package api

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/auth"
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/bastion"
)

func (s *Server) registerTotpRoutes(api *gin.RouterGroup) {
	api.POST("/auth/totp/setup", s.handleTotpSetup)
	api.POST("/auth/totp/verify", s.handleTotpVerify)
	api.POST("/auth/totp/disable", s.handleTotpDisable)
}

func (s *Server) totpEncrypt(plain string) (string, error) {
	return bastion.EncryptCredential(s.cfg.JWTSecret, plain)
}

func (s *Server) totpDecrypt(enc string) (string, error) {
	return bastion.DecryptCredential(s.cfg.JWTSecret, enc)
}

func (s *Server) handleTotpSetup(c *gin.Context) {
	uid := c.GetUint("user_id")
	res, err := s.authSvc.SetupTotp(uid, s.totpEncrypt)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, res)
}

func (s *Server) handleTotpVerify(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.authSvc.VerifyAndEnableTotp(c.GetUint("user_id"), req.Code, s.totpDecrypt); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Message(c, "2FA enabled")
}

func (s *Server) handleTotpDisable(c *gin.Context) {
	var req struct {
		UserID   uint   `json:"user_id"`
		Password string `json:"password"`
	}
	_ = c.ShouldBindJSON(&req)
	role, _ := c.Get("role")
	roleStr, _ := role.(string)
	targetID := c.GetUint("user_id")
	isAdmin := roleStr == "admin"
	if isAdmin && req.UserID > 0 {
		targetID = req.UserID
	}
	if err := s.authSvc.DisableTotp(targetID, req.Password, isAdmin && req.UserID > 0); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Message(c, "2FA disabled")
}

func (s *Server) handleTotpLogin(c *gin.Context) {
	var req struct {
		TempToken string `json:"temp_token" binding:"required"`
		Code      string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}

	ip := c.ClientIP()
	ua := c.GetHeader("User-Agent")
	claims, _ := s.authSvc.ParseToken(req.TempToken)
	username := ""
	if claims != nil {
		username = claims.Username
	}

	if err := auth.CheckLoginIPAllowed(ip); err != nil {
		response.Error(c, 429, err.Error())
		return
	}
	auth.RecordLoginIPAttempt(ip)
	if username != "" {
		if err := auth.CheckLoginAllowed(ip, username); err != nil {
			auth.RecordLoginEvent(s.db, username, ip, ua, false, "locked")
			s.enterprise.Recorder().Login(username, ip, ua, false, "locked")
			if s.syslog != nil {
				s.syslog.LoginFailure(username, ip, "locked")
			}
			response.Error(c, 429, err.Error())
			return
		}
	}

	token, user, err := s.authSvc.CompleteTotpLogin(req.TempToken, req.Code, s.totpDecrypt)
	if err != nil {
		if err == auth.ErrInvalidTotpCode {
			if username == "" && user != nil {
				username = user.Username
			}
			if username != "" {
				auth.RecordLoginFailure(ip, username)
				auth.RecordLoginEvent(s.db, username, ip, ua, false, "invalid_totp")
				s.enterprise.Recorder().Login(username, ip, ua, false, "invalid_totp")
				if s.syslog != nil {
					s.syslog.LoginFailure(username, ip, "invalid_totp")
				}
			}
			response.Error(c, 401, "invalid TOTP code")
			return
		}
		response.Error(c, 401, err.Error())
		return
	}
	auth.RecordLoginSuccess(ip, user.Username)
	auth.RecordLoginEvent(s.db, user.Username, ip, ua, true, "ok_totp")
	s.enterprise.Recorder().Login(user.Username, ip, ua, true, "ok_totp")
	if s.syslog != nil {
		s.syslog.LoginSuccess(user.Username, ip)
	}
	response.OK(c, gin.H{
		"token": token,
		"user": gin.H{
			"id": user.ID, "username": user.Username, "role": user.Role,
			"must_change_password": user.MustChangePassword,
			"permissions": user.Permissions, "totp_enabled": user.TotpEnabled,
		},
	})
}

func (s *Server) registerBastionPamRoutes(api, admin *gin.RouterGroup) {
	api.GET("/bastion/access-requests", s.handleListAccessRequests)
	api.POST("/bastion/access-requests", s.handleCreateAccessRequest)
	admin.POST("/bastion/access-requests/:id/approve", s.handleApproveAccessRequest)
	admin.POST("/bastion/access-requests/:id/reject", s.handleRejectAccessRequest)

	admin.GET("/bastion/compliance/score", s.handleBastionComplianceScore)
	admin.POST("/bastion/compliance/export", s.handleBastionComplianceExport)
	admin.GET("/bastion/compliance/download/:filename", s.handleBastionComplianceDownload)

	admin.GET("/bastion/known-hosts", s.handleListKnownHosts)
	admin.POST("/bastion/known-hosts/capture/:assetId", s.handleCaptureKnownHost)
	admin.POST("/bastion/known-hosts/:assetId/accept", s.handleAcceptKnownHost)

	admin.GET("/security/syslog", s.handleSyslogSettingsGet)
	admin.PUT("/security/syslog", s.handleSyslogSettingsPut)
}

func (s *Server) handleListAccessRequests(c *gin.Context) {
	uid, _, role := bastionUser(c)
	list, err := s.bastion.ListAccessRequests(uid, role)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateAccessRequest(c *gin.Context) {
	var in bastion.AccessRequestInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	uid, _, _ := bastionUser(c)
	req, err := s.bastion.CreateAccessRequest(uid, in)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, req)
}

func (s *Server) handleApproveAccessRequest(c *gin.Context) {
	uid, un, _ := bastionUser(c)
	req, err := s.bastion.ApproveAccessRequest(parseID(c), uid, func(r *models.BastionAccessRequest) {
		if s.syslog != nil {
			s.syslog.AccessRequestApproved(r.ID, r.Username, r.AssetName, un)
		}
	})
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, req)
}

func (s *Server) handleRejectAccessRequest(c *gin.Context) {
	uid, _, _ := bastionUser(c)
	req, err := s.bastion.RejectAccessRequest(parseID(c), uid)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, req)
}

func (s *Server) handleBastionComplianceScore(c *gin.Context) {
	response.OK(c, s.bastion.ComputeComplianceScore())
}

func (s *Server) handleBastionComplianceExport(c *gin.Context) {
	var req struct {
		From  string   `json:"from"`
		To    string   `json:"to"`
		Types []string `json:"types"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	in := bastion.ComplianceExportInput{Types: req.Types}
	if req.From != "" {
		if t, err := time.Parse(time.RFC3339, req.From); err == nil {
			in.From = t
		}
	}
	if req.To != "" {
		if t, err := time.Parse(time.RFC3339, req.To); err == nil {
			in.To = t
		}
	}
	name, err := s.bastion.ExportCompliance(in)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"filename": name, "download_url": "/bastion/compliance/download/" + name})
}

func (s *Server) handleBastionComplianceDownload(c *gin.Context) {
	path, err := s.bastion.ComplianceExportPath(c.Param("filename"))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	c.FileAttachment(path, c.Param("filename"))
}

func (s *Server) handleListKnownHosts(c *gin.Context) {
	list, err := s.bastion.ListKnownHosts()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCaptureKnownHost(c *gin.Context) {
	assetID := parseIDParam(c.Param("assetId"))
	kh, err := s.bastion.CaptureHostKey(assetID)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, kh)
}

func (s *Server) handleAcceptKnownHost(c *gin.Context) {
	assetID := parseIDParam(c.Param("assetId"))
	kh, err := s.bastion.AcceptKnownHost(assetID)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, kh)
}

func (s *Server) handleSyslogSettingsGet(c *gin.Context) {
	all, err := s.settings.GetAll()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{
		"syslog_enabled":  all["syslog_enabled"],
		"syslog_host":     all["syslog_host"],
		"syslog_port":     all["syslog_port"],
		"syslog_protocol": all["syslog_protocol"],
	})
}

func (s *Server) handleSyslogSettingsPut(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	allowed := map[string]bool{
		"syslog_enabled": true, "syslog_host": true,
		"syslog_port": true, "syslog_protocol": true,
	}
	data := map[string]string{}
	for k, v := range req {
		if allowed[k] {
			data[k] = v
		}
	}
	if len(data) == 0 {
		response.Error(c, 400, "no valid fields")
		return
	}
	if err := s.settings.Update(data); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "updated")
}
