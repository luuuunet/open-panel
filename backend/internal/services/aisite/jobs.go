package aisite

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

type DeployRequest struct {
	RepoURL      string     `json:"repo_url"`
	Branch       string     `json:"branch"`
	GithubToken  string     `json:"github_token"`
	Notes        string     `json:"notes"`
	SelectedApps []string   `json:"selected_apps"`
	Plan         DeployPlan `json:"plan"`
}

type AutoDeployRequest struct {
	RepoURL     string `json:"repo_url"`
	Branch      string `json:"branch"`
	GithubToken string `json:"github_token"`
	Domain      string `json:"domain"`
	Notes       string `json:"notes"`
}

func (s *Service) AutoDeploy(req AutoDeployRequest) (*models.AISiteBootstrapJob, error) {
	domain := strings.TrimSpace(req.Domain)
	if domain == "" {
		domain = suggestDomain("", req.RepoURL)
	}
	return s.Deploy(DeployRequest{
		RepoURL:     req.RepoURL,
		Branch:      req.Branch,
		GithubToken: req.GithubToken,
		Notes:       req.Notes,
		Plan: DeployPlan{
			Domain:         domain,
			EnableWebhook:  true,
			RollbackOnFail: true,
		},
	})
}

func (s *Service) Deploy(req DeployRequest) (*models.AISiteBootstrapJob, error) {
	if strings.TrimSpace(req.RepoURL) == "" {
		return nil, fmt.Errorf("repo_url is required")
	}
	if strings.TrimSpace(req.Plan.Domain) == "" {
		req.Plan.Domain = suggestDomain("", req.RepoURL)
	}
	if strings.TrimSpace(req.Plan.Domain) == "" {
		return nil, fmt.Errorf("请填写域名")
	}
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = "main"
	}

	startLog := fmt.Sprintf("[%s] 任务已启动，目标域名: %s\n[%s] 流水线: 分析 → 规划 → 执行 → 部署", ts(), req.Plan.Domain, ts())
	steps := newPipelineSteps()
	job := models.AISiteBootstrapJob{
		RepoURL:      req.RepoURL,
		Branch:       branch,
		Domain:       req.Plan.Domain,
		Status:       "running",
		CurrentPhase: PhaseAnalyze,
		Log:          startLog,
		StepsJSON:    stepsToJSON(steps),
		PlanJSON:     planToJSON(req.Plan),
		StartedAt:    time.Now(),
	}
	if err := s.db.Create(&job).Error; err != nil {
		return nil, err
	}

	go s.runBootstrap(job.ID, req)
	return &job, nil
}

func (s *Service) GetJob(id uint) (*models.AISiteBootstrapJob, error) {
	var job models.AISiteBootstrapJob
	if err := s.db.First(&job, id).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *Service) ListJobs(limit int) ([]models.AISiteBootstrapJob, error) {
	if limit <= 0 {
		limit = 20
	}
	var jobs []models.AISiteBootstrapJob
	return jobs, s.db.Order("id desc").Limit(limit).Find(&jobs).Error
}

func (s *Service) waitDeployJob(deployID uint, appendLog func(string)) {
	for i := 0; i < 120; i++ {
		var job models.SiteDeployJob
		if s.db.First(&job, deployID).Error != nil {
			return
		}
		if job.Status != "running" {
			return
		}
		time.Sleep(2 * time.Second)
	}
	appendLog("部署任务仍在运行，请稍后在 DevOps 中心查看完整日志")
}

func (s *Service) updateJobLog(jobID uint, log string) {
	_ = s.db.Model(&models.AISiteBootstrapJob{}).Where("id = ?", jobID).Update("log", log).Error
}

func (s *Service) updateJobPlan(jobID uint, plan DeployPlan) {
	_ = s.db.Model(&models.AISiteBootstrapJob{}).Where("id = ?", jobID).Update("plan_json", planToJSON(plan)).Error
}

func (s *Service) finishJob(jobID uint, status, log string, websiteID, deployJobID uint, errMsg string) {
	now := time.Now()
	updates := map[string]interface{}{
		"status":        status,
		"log":           log,
		"error":         errMsg,
		"ended_at":      &now,
		"website_id":    websiteID,
		"current_phase": "",
	}
	if deployJobID > 0 {
		updates["deploy_job_id"] = deployJobID
	}
	_ = s.db.Model(&models.AISiteBootstrapJob{}).Where("id = ?", jobID).Updates(updates).Error
}

func deployJobID(j *models.SiteDeployJob) uint {
	if j == nil {
		return 0
	}
	return j.ID
}

func ts() string {
	return time.Now().Format("15:04:05")
}

func splitJobLog(log string) []string {
	if log == "" {
		return nil
	}
	return strings.Split(log, "\n")
}

func ParsePlanJSON(raw string) (DeployPlan, error) {
	var plan DeployPlan
	err := json.Unmarshal([]byte(raw), &plan)
	return plan, err
}
