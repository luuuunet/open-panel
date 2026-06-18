package wordpress

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/domaincheck"
)

type DeployJob struct {
	ID          string     `json:"job_id"`
	SiteID      uint       `json:"site_id"`
	Domain      string     `json:"domain"`
	Status      string     `json:"status"` // running, success, failed
	Logs        []string   `json:"logs"`
	Error       string     `json:"error,omitempty"`
	FtpUser     string     `json:"ftp_user,omitempty"`
	FtpPassword string     `json:"ftp_password,omitempty"`
	DbName      string     `json:"db_name,omitempty"`
	DbUser      string     `json:"db_user,omitempty"`
	DbPassword  string     `json:"db_password,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
	mu          sync.Mutex
}

var deployJobs sync.Map

type DeployLogger struct {
	job *DeployJob
}

func newDeployJob(domain string, siteID uint) *DeployJob {
	id := randomJobID()
	job := &DeployJob{
		ID: id, SiteID: siteID, Domain: domain,
		Status: "running", StartedAt: time.Now(),
	}
	deployJobs.Store(id, job)
	return job
}

func randomJobID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func GetDeployJob(id string) (*DeployJob, bool) {
	v, ok := deployJobs.Load(id)
	if !ok {
		return nil, false
	}
	job := v.(*DeployJob)
	job.mu.Lock()
	defer job.mu.Unlock()
	snapshot := *job
	snapshot.Logs = append([]string(nil), job.Logs...)
	return &snapshot, true
}

func (l *DeployLogger) log(level, msg string) {
	if l == nil || l.job == nil {
		return
	}
	line := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	l.job.mu.Lock()
	l.job.Logs = append(l.job.Logs, line)
	l.job.mu.Unlock()
}

func (l *DeployLogger) Info(msg string)  { l.log("INFO", msg) }
func (l *DeployLogger) Warn(msg string)  { l.log("WARN", msg) }
func (l *DeployLogger) Error(msg string) { l.log("ERROR", msg) }

func (l *DeployLogger) finish(status, errMsg string) {
	if l == nil || l.job == nil {
		return
	}
	now := time.Now()
	l.job.mu.Lock()
	l.job.Status = status
	l.job.Error = errMsg
	l.job.EndedAt = &now
	if errMsg != "" {
		l.job.Logs = append(l.job.Logs, fmt.Sprintf("[%s] 部署失败: %s", now.Format("15:04:05"), errMsg))
	} else {
		l.job.Logs = append(l.job.Logs, fmt.Sprintf("[%s] ✓ 部署完成", now.Format("15:04:05")))
	}
	l.job.mu.Unlock()
}

func (l *DeployLogger) setCredentials(r *ProvisionResult) {
	if l == nil || l.job == nil || r == nil {
		return
	}
	l.job.mu.Lock()
	l.job.FtpUser = r.FtpUser
	l.job.FtpPassword = r.FtpPassword
	l.job.DbName = r.DbName
	l.job.DbUser = r.DbUser
	l.job.DbPassword = r.DbPassword
	l.job.mu.Unlock()
}

func (s *Service) StartDeploy(req *CreateRequest) (*DeployJob, error) {
	site, extras, err := s.prepareSite(req)
	if err != nil {
		return nil, err
	}

	allDomains := collectAllDomains(site.Domain, extras)
	if err := domaincheck.AssertAvailable(s.db, allDomains, domaincheck.Scope{}); err != nil {
		// Allow redeploy hint when a failed WP site already owns the domain.
		host := domaincheck.HostOnly(site.Domain)
		var existing models.WordPressSite
		if s.db.Where("domain = ?", host).First(&existing).Error == nil && existing.Status == "error" {
			return nil, fmt.Errorf("域名 %s 已有失败的 WordPress 站点（ID %d），请删除后重新部署，或在站点列表点击「修复」", host, existing.ID)
		}
		return nil, err
	}
	if jobID, busy := s.domainDeployInProgress(site.Domain); busy {
		return nil, fmt.Errorf("域名 %s 正在部署中（任务 %s），请勿重复点击", site.Domain, jobID)
	}

	site.Status = "deploying"
	if err := s.db.Create(site).Error; err != nil {
		return nil, err
	}
	req.ID = site.ID
	s.ensurePrimaryDomain(site)

	for _, d := range extras {
		d = domaincheck.HostOnly(normalizeDomain(d))
		if d == "" || strings.EqualFold(d, site.Domain) {
			continue
		}
		_ = s.db.Create(&models.WordPressDomain{
			SiteID: site.ID, Domain: d, Type: "alias", Enabled: true,
		}).Error
	}

	return s.startDeployJob(site, extras, databaseOptionsFromRequest(req))
}

func (s *Service) Redeploy(id uint) (*DeployJob, error) {
	site, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if jobID, busy := s.domainDeployInProgress(site.Domain); busy {
		return nil, fmt.Errorf("域名 %s 正在部署中（任务 %s）", site.Domain, jobID)
	}
	if site.Status == "deploying" {
		return nil, fmt.Errorf("站点正在部署中，请稍候")
	}
	s.db.Model(site).Update("status", "deploying")
	var extras []string
	for _, d := range site.Domains {
		if d.Enabled && !strings.EqualFold(d.Domain, site.Domain) {
			extras = append(extras, d.Domain)
		}
	}
	return s.startDeployJob(site, extras, defaultDatabaseOptions(site))
}

func (s *Service) startDeployJob(site *models.WordPressSite, extras []string, dbOpts DatabaseOptions) (*DeployJob, error) {
	job := newDeployJob(site.Domain, site.ID)
	logger := &DeployLogger{job: job}
	logger.Info(fmt.Sprintf("开始部署 WordPress 站点: %s", site.Domain))
	logger.Info(fmt.Sprintf("WP 版本: %s | PHP: %s | Nginx: %s", site.Version, site.PhpVersion, site.NginxVersion))
	logger.Info(fmt.Sprintf("网站根目录: %s", site.RootPath))
	if len(extras) > 0 {
		logger.Info(fmt.Sprintf("附加域名: %s", strings.Join(extras, ", ")))
	}
	go s.runDeploy(site, extras, dbOpts, logger)
	return job, nil
}

func collectAllDomains(primary string, extras []string) []string {
	var all []string
	all = append(all, primary)
	all = append(all, extras...)
	return all
}

func (s *Service) domainDeployInProgress(domain string) (jobID string, busy bool) {
	host := domaincheck.HostOnly(domain)
	var found string
	deployJobs.Range(func(_, v any) bool {
		job := v.(*DeployJob)
		if job.Status == "running" && domaincheck.HostOnly(job.Domain) == host {
			found = job.ID
			return false
		}
		return true
	})
	return found, found != ""
}

func (s *Service) runDeploy(site *models.WordPressSite, extras []string, dbOpts DatabaseOptions, logger *DeployLogger) {
	defer func() {
		if r := recover(); r != nil {
			logger.finish("failed", fmt.Sprintf("内部错误: %v", r))
			s.db.Model(site).Update("status", "error")
		}
	}()

	if result, err := s.provisionWithLog(site, dbOpts, logger); err != nil {
		s.db.Model(site).Update("status", "error")
		logger.finish("failed", err.Error())
		return
	} else {
		logger.setCredentials(result)
	}

	for _, d := range extras {
		d = normalizeDomain(d)
		if d == "" {
			continue
		}
		logger.Info(fmt.Sprintf("同步网站记录: %s", d))
		_ = s.syncWebsiteAlias(site, d)
	}

	logger.Info("正在更新 Nginx 虚拟主机（多域名）...")
	if err := s.regenerateVhost(site.ID); err != nil {
		s.db.Model(site).Update("status", "error")
		logger.finish("failed", err.Error())
		return
	}

	if site.AutoSSL && !site.CloudflareCDN {
		fresh, err := s.Get(site.ID)
		if err == nil {
			logger.Info("正在申请 SSL 证书（Let's Encrypt）…")
			if err := s.issueSSL(fresh, logger, fresh.SSLEmail); err != nil {
				logger.Warn("SSL 申请失败: " + err.Error())
			}
		}
	}

	if fresh, err := s.Get(site.ID); err == nil && fresh.CloudflareCDN {
		_ = s.applyCDNMode(fresh)
	}

	logger.finish("success", "")
}
