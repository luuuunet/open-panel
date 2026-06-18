package aisite

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/services/aichat"
)

type SiteLogChatRequest struct {
	Message    string          `json:"message"`
	AccessLog  string          `json:"access_log"`
	ErrorLog   string          `json:"error_log"`
	AccessPath string          `json:"access_path"`
	ErrorPath  string          `json:"error_path"`
	History    []aichat.Message `json:"history"`
}

type SiteLogRepairRequest struct {
	AccessLog string `json:"access_log"`
	ErrorLog  string `json:"error_log"`
}

type SiteLogRepairResult struct {
	Summary        string   `json:"summary"`
	Diagnosis      string   `json:"diagnosis"`
	ActionsApplied []string `json:"actions_applied"`
	FilesWritten   []string `json:"files_written"`
	FailedActions  []string `json:"failed_actions,omitempty"`
	Logs           []string `json:"logs"`
	Fixed          bool     `json:"fixed"`
}

func (s *Service) SiteLogChat(siteID uint, req SiteLogChatRequest) (*aichat.LogChatResult, error) {
	site, err := s.website.Get(siteID)
	if err != nil {
		return nil, err
	}
	return s.aichat.SiteLogChat(aichat.SiteLogChatRequest{
		Message:    req.Message,
		Domain:     site.Domain,
		RootPath:   site.RootPath,
		AccessLog:  req.AccessLog,
		ErrorLog:   req.ErrorLog,
		AccessPath: req.AccessPath,
		ErrorPath:  req.ErrorPath,
		History:    req.History,
	})
}

func (s *Service) SiteLogChatStream(siteID uint, req SiteLogChatRequest, c *gin.Context) {
	site, err := s.website.Get(siteID)
	if err != nil {
		aichat.StreamError(c, err.Error())
		return
	}
	s.aichat.StreamSiteLogChat(aichat.SiteLogChatRequest{
		Message:    req.Message,
		Domain:     site.Domain,
		RootPath:   site.RootPath,
		AccessLog:  req.AccessLog,
		ErrorLog:   req.ErrorLog,
		AccessPath: req.AccessPath,
		ErrorPath:  req.ErrorPath,
		History:    req.History,
	}, c)
}

func (s *Service) SiteLogRepair(siteID uint, req SiteLogRepairRequest) (*SiteLogRepairResult, error) {
	if err := s.aichat.EnsureConfigured(); err != nil {
		return nil, err
	}

	res := &SiteLogRepairResult{Logs: []string{}}
	log := func(msg string) {
		res.Logs = append(res.Logs, msg)
	}

	log("正在收集站点诊断与日志…")
	bundle, err := s.website.DiagnosticJSON(siteID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.AccessLog) != "" || strings.TrimSpace(req.ErrorLog) != "" {
		bundle += "\n\n--- 当前页面日志 ---\n访问日志:\n" + trimTail(req.AccessLog, 12000)
		bundle += "\n\n错误日志:\n" + trimTail(req.ErrorLog, 12000)
	}

	log("正在调用 AI 分析日志并生成修复方案…")
	plan, err := s.aichat.AnalyzeSiteLogRepair(bundle)
	if err != nil {
		log("AI 分析失败: " + err.Error() + "，使用默认方案")
		plan = aichat.DefaultSiteLogRepairPlan()
	}
	res.Summary = plan.Summary
	res.Diagnosis = plan.Diagnosis
	log("AI: " + plan.Summary)

	for _, action := range plan.Actions {
		label := repairActionLabel(action)
		log("执行: " + label)
		if err := s.website.ApplyRepairAction(siteID, action); err != nil {
			log("  ✗ " + err.Error())
			res.FailedActions = append(res.FailedActions, action)
			continue
		}
		log("  ✓ 完成")
		res.ActionsApplied = append(res.ActionsApplied, action)
	}

	for _, fw := range plan.FileWrites {
		log(fmt.Sprintf("写入文件: %s", fw.RelativePath))
		if err := s.website.WriteSiteFile(siteID, fw.RelativePath, fw.Content); err != nil {
			log("  ✗ " + err.Error())
			res.FailedActions = append(res.FailedActions, "write:"+fw.RelativePath)
			continue
		}
		log("  ✓ 已写入")
		res.FilesWritten = append(res.FilesWritten, fw.RelativePath)
	}

	res.Fixed = len(res.FailedActions) == 0 && (len(res.ActionsApplied) > 0 || len(res.FilesWritten) > 0)
	if !res.Fixed && len(res.ActionsApplied) == 0 && len(res.FilesWritten) == 0 {
		return res, fmt.Errorf("未能执行任何修复动作")
	}
	log("日志 AI 修复完成")
	return res, nil
}

func trimTail(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return "...(truncated)\n" + s[len(s)-max:]
}
