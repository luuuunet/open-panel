package api

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/toolbox"
)

func (s *Server) registerToolboxRoutes(admin *gin.RouterGroup) {
	admin.POST("/toolbox/ping", s.handleToolboxPing)
	admin.POST("/toolbox/traceroute", s.handleToolboxTraceroute)
	admin.POST("/toolbox/dns", s.handleToolboxDNS)

	admin.GET("/toolbox/system/overview", s.handleToolboxSystemOverview)
	admin.GET("/toolbox/system/ports", s.handleToolboxListeningPorts)
	admin.GET("/toolbox/system/processes", s.handleToolboxTopProcesses)
	admin.POST("/toolbox/system/processes/:pid/kill", s.handleToolboxKillProcess)
	admin.POST("/toolbox/system/drop-cache", s.handleToolboxDropCache)

	admin.GET("/toolbox/health", s.handleToolboxHealth)
	admin.GET("/toolbox/snippets", s.handleToolboxListSnippets)
	admin.POST("/toolbox/snippets", s.handleToolboxCreateSnippet)
	admin.PUT("/toolbox/snippets/:id", s.handleToolboxUpdateSnippet)
	admin.DELETE("/toolbox/snippets/:id", s.handleToolboxDeleteSnippet)
	admin.POST("/toolbox/snippets/run", s.handleToolboxRunSnippet)
}

func toolboxLang(c *gin.Context) string {
	lang := c.Query("lang")
	if lang == "" {
		lang = c.GetHeader("Accept-Language")
	}
	if strings.HasPrefix(strings.ToLower(lang), "en") {
		return "en"
	}
	return "zh"
}

func (s *Server) handleToolboxSystemOverview(c *gin.Context) {
	data, err := s.toolbox.SystemOverview()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, data)
}

func (s *Server) handleToolboxListeningPorts(c *gin.Context) {
	data, err := s.toolbox.ListeningPorts()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	rules, _ := s.firewall.List()
	for i := range data {
		for _, r := range rules {
			if r.Port == int(data[i].Port) && strings.EqualFold(r.Protocol, data[i].Protocol) && r.Action == "allow" {
				data[i].Firewalled = true
				break
			}
		}
	}
	response.OK(c, data)
}

func (s *Server) handleToolboxTopProcesses(c *gin.Context) {
	limit := 15
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}
	sortBy := strings.ToLower(strings.TrimSpace(c.Query("sort")))
	var (
		data []toolbox.ProcessTop
		err  error
	)
	switch sortBy {
	case "cpu":
		procs, e := s.process.TopByCPU(limit)
		err = e
		if err == nil {
			data = make([]toolbox.ProcessTop, len(procs))
			for i, p := range procs {
				data[i] = toolbox.ProcessTop{
					PID: p.PID, Name: p.Name, User: p.User,
					CPU: p.CPU, Memory: p.Memory, Command: p.Command,
				}
			}
		}
	default:
		data, err = s.toolbox.TopProcesses(limit)
	}
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, data)
}

func (s *Server) handleToolboxKillProcess(c *gin.Context) {
	pid64, err := strconv.ParseInt(c.Param("pid"), 10, 32)
	if err != nil || pid64 <= 0 {
		response.Error(c, 400, "invalid pid")
		return
	}
	pid := int32(pid64)
	if err := s.process.Kill(pid); err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "protected"):
			response.Error(c, 403, msg)
		case strings.Contains(msg, "not found"):
			response.Error(c, 404, msg)
		default:
			response.Error(c, 500, msg)
		}
		return
	}
	response.Message(c, "killed")
}

func (s *Server) handleToolboxDropCache(c *gin.Context) {
	result, err := s.toolbox.DropCaches()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleToolboxHealth(c *gin.Context) {
	report, err := s.toolbox.HealthReport(toolboxLang(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, report)
}

func (s *Server) handleDashboardHealth(c *gin.Context) {
	report, err := s.toolbox.HealthReport(toolboxLang(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, report)
}

func (s *Server) handleToolboxListSnippets(c *gin.Context) {
	list, err := s.toolbox.ListSnippets(toolboxLang(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleToolboxCreateSnippet(c *gin.Context) {
	var req models.CommandSnippet
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.toolbox.CreateSnippet(&req); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, req)
}

func (s *Server) handleToolboxUpdateSnippet(c *gin.Context) {
	id := parseID(c)
	var req struct {
		Name     string `json:"name"`
		Command  string `json:"command"`
		Category string `json:"category"`
		Remark   string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Command != "" {
		updates["command"] = req.Command
	}
	if req.Category != "" {
		updates["category"] = req.Category
	}
	updates["remark"] = req.Remark
	if err := s.toolbox.UpdateSnippet(id, updates); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "updated")
}

func (s *Server) handleToolboxDeleteSnippet(c *gin.Context) {
	if err := s.toolbox.DeleteSnippet(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleToolboxRunSnippet(c *gin.Context) {
	var req struct {
		ID      string `json:"id"`
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	cmd := strings.TrimSpace(req.Command)
	if cmd == "" && req.ID != "" {
		var err error
		cmd, err = s.toolbox.ResolveSnippetCommand(req.ID)
		if err != nil {
			response.Error(c, 400, err.Error())
			return
		}
	}
	if cmd == "" {
		response.Error(c, 400, "command required")
		return
	}
	result, err := s.toolbox.RunCommand(cmd, req.Timeout)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}
