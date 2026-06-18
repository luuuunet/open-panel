package aisite

import (
	"fmt"

	"github.com/luuuunet/owpanel/internal/services/aichat"
)

type SiteDiagnoseRepairResult struct {
	Summary        string   `json:"summary"`
	Diagnosis      string   `json:"diagnosis"`
	ActionsApplied []string `json:"actions_applied"`
	FailedActions  []string `json:"failed_actions,omitempty"`
	Logs           []string `json:"logs"`
	Fixed          bool     `json:"fixed"`
}

func (s *Service) DiagnoseRepair(siteID uint) (*SiteDiagnoseRepairResult, error) {
	if err := s.aichat.EnsureConfigured(); err != nil {
		return nil, err
	}

	res := &SiteDiagnoseRepairResult{Logs: []string{}}
	log := func(msg string) {
		res.Logs = append(res.Logs, msg)
	}

	log("正在收集站点诊断信息…")
	bundleJSON, err := s.website.DiagnosticJSON(siteID)
	if err != nil {
		return nil, err
	}
	log("诊断信息收集完成")

	log("正在调用 AI 分析…")
	plan, err := s.aichat.AnalyzeSiteRepair(bundleJSON)
	if err != nil {
		log("AI 分析失败: " + err.Error() + "，将使用默认修复方案")
		plan = aichat.DefaultSiteRepairPlan()
	}
	res.Summary = plan.Summary
	res.Diagnosis = plan.Diagnosis
	log("AI 诊断: " + plan.Summary)
	if plan.Diagnosis != "" {
		log(plan.Diagnosis)
	}

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

	res.Fixed = len(res.FailedActions) == 0 && len(res.ActionsApplied) > 0
	if len(res.ActionsApplied) == 0 {
		return res, fmt.Errorf("未能执行任何修复动作")
	}
	log("修复流程结束")
	return res, nil
}

func repairActionLabel(action string) string {
	labels := map[string]string{
		"apply_vhost":         "重建虚拟主机",
		"reload_webserver":    "重载 Web 服务器",
		"fix_dir_permissions": "修复目录权限",
		"ensure_index_files":  "设置默认索引文件",
		"start_site":          "启用站点",
		"start_php_fpm":       "启动 PHP-FPM",
		"create_root_dir":     "创建网站根目录",
	}
	if l, ok := labels[action]; ok {
		return l
	}
	return action
}

func (s *Service) AIAssistantStatus() aichat.AssistantStatus {
	return s.aichat.AssistantStatus()
}
