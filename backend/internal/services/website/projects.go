package website

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/webserver"
)

type ProjectRow struct {
	ID           uint       `json:"id"`
	Source       string     `json:"source"`
	ProjectType  string     `json:"project_type"`
	Domain       string     `json:"domain"`
	Status       string     `json:"status"`
	BackupStatus string     `json:"backup_status"`
	RootPath     string     `json:"root_path"`
	Security     bool       `json:"security"`
	ExpiresAt    *time.Time `json:"expires_at"`
	ExpiresLabel string     `json:"expires_label"`
	Remark       string     `json:"remark"`
	PhpVersion   string     `json:"php_version"`
	PhpVersionValue string  `json:"php_version_value"`
	NodeVersion  string     `json:"node_version,omitempty"`
	SSLStatus    string     `json:"ssl_status"`
	SSL          bool       `json:"ssl"`
	Traffic      int64      `json:"traffic"`
	TrafficToday int64      `json:"traffic_today"`
	Port         int        `json:"port"`
	WebServer    string     `json:"web_server"`
	CacheEnabled            bool `json:"cache_enabled"`
	CrossSiteProtectEnabled bool `json:"cross_site_protect_enabled"`
	PhpAccelEnabled         bool `json:"php_accel_enabled"`
	PhpEnabled              bool `json:"php_enabled"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (s *Service) ListProjects(projectType, search string) ([]ProjectRow, error) {
	projectType = strings.ToLower(strings.TrimSpace(projectType))
	search = strings.TrimSpace(strings.ToLower(search))

	var rows []ProjectRow

	switch projectType {
	case "node":
		rows = append(rows, s.nodeProjects(search)...)
	case "java":
		rows = append(rows, s.javaProjects(search)...)
	default:
		rows = append(rows, s.phpProjects(search)...)
	}
	return rows, nil
}

func (s *Service) phpProjects(search string) []ProjectRow {
	var sites []models.Website
	q := s.db.Preload("Aliases").Order("id desc")
	if search != "" {
		like := "%" + search + "%"
		q = q.Where("domain LIKE ? OR remark LIKE ? OR root_path LIKE ?", like, like, like)
	}
	_ = q.Find(&sites).Error

	certMap := s.sslCertMap()
	var rows []ProjectRow
	for _, site := range sites {
		pt := site.ProjectType
		if pt == "" {
			if site.PhpVersion == "static" || !site.PHP {
				pt = "static"
			} else {
				pt = "php"
			}
		}
		if pt == "node" || pt == "java" {
			continue
		}
		rows = append(rows, s.websiteToRow(&site, certMap))
	}
	return rows
}

func (s *Service) javaProjects(search string) []ProjectRow {
	var projects []models.JavaProject
	q := s.db.Order("id desc")
	if search != "" {
		like := "%" + search + "%"
		q = q.Where("name LIKE ? OR domain LIKE ? OR path LIKE ? OR remark LIKE ?", like, like, like, like)
	}
	_ = q.Find(&projects).Error

	certMap := s.sslCertMap()
	var rows []ProjectRow
	for _, p := range projects {
		sslStatus := "none"
		if c, ok := certMap[strings.ToLower(p.Domain)]; ok {
			sslStatus = c.Status
		}
		total, today := s.siteTrafficStats(p.Domain)
		rows = append(rows, ProjectRow{
			ID:           p.ID,
			Source:       "java",
			ProjectType:  "java",
			Domain:       p.Domain,
			Status:       p.Status,
			BackupStatus: "none",
			RootPath:     p.Path,
			ExpiresLabel: "永久",
			Remark:       p.Remark,
			PhpVersion:   "Java-" + p.JavaVer,
			SSLStatus:    sslStatus,
			Traffic:      total,
			TrafficToday: today,
			Port:         p.Port,
			WebServer:    s.activeWebServer(),
			CreatedAt:    p.CreatedAt,
		})
	}
	return rows
}

func (s *Service) nodeProjects(search string) []ProjectRow {
	var nodes []models.NodeProject
	q := s.db.Order("id desc")
	if search != "" {
		like := "%" + search + "%"
		q = q.Where("name LIKE ? OR domain LIKE ? OR path LIKE ? OR remark LIKE ?", like, like, like, like)
	}
	_ = q.Find(&nodes).Error

	var rows []ProjectRow
	for _, n := range nodes {
		domain := n.Domain
		if domain == "" {
			domain = n.Name
		}
		rows = append(rows, ProjectRow{
			ID:           n.ID,
			Source:       "node",
			ProjectType:  "node",
			Domain:       domain,
			Status:       n.Status,
			BackupStatus: "none",
			RootPath:     n.Path,
			Security:     false,
			ExpiresLabel: "永久",
			Remark:       n.Remark,
			NodeVersion:  n.NodeVer,
			SSLStatus:    "none",
			Traffic:      0,
			TrafficToday: 0,
			Port:         n.Port,
			WebServer:    s.activeWebServer(),
			CreatedAt:    n.CreatedAt,
		})
	}
	return rows
}

func (s *Service) websiteToRow(site *models.Website, certMap map[string]models.SSLCertificate) ProjectRow {
	pt := site.ProjectType
	if pt == "" {
		if site.PhpVersion == "static" || !site.PHP {
			pt = "static"
		} else {
			pt = "php"
		}
	}
	ws := site.WebServer
	if ws == "" {
		ws = s.activeWebServer()
	}
	sslStatus := "none"
	if site.SSL {
		sslStatus = "enabled"
	}
	if cert, ok := certMap[strings.ToLower(site.Domain)]; ok {
		if cert.Status == "active" || cert.Status == "valid" {
			sslStatus = "active"
		} else {
			sslStatus = cert.Status
		}
	}
	expLabel := expiresLabel(site.ExpiresAt)
	phpLabel := site.PhpVersion
	if phpLabel == "" || phpLabel == "static" {
		phpLabel = "静态"
	} else {
		phpLabel = "PHP-" + phpLabel
	}
	total, today := s.siteTrafficStats(site.Domain)
	return ProjectRow{
		ID:           site.ID,
		Source:       "website",
		ProjectType:  pt,
		Domain:       site.Domain,
		Status:       site.Status,
		BackupStatus: site.BackupStatus,
		RootPath:     site.RootPath,
		Security:     false,
		ExpiresAt:    site.ExpiresAt,
		ExpiresLabel: expLabel,
		Remark:       site.Remark,
		PhpVersion:              phpLabel,
		PhpVersionValue:         site.PhpVersion,
		SSLStatus:               sslStatus,
		SSL:          site.SSL,
		Traffic:      total,
		TrafficToday: today,
		Port:         site.Port,
		WebServer:    ws,
		CacheEnabled:            site.CacheEnabled,
		CrossSiteProtectEnabled: site.CrossSiteProtectEnabled,
		PhpAccelEnabled:         site.PhpAccelEnabled,
		PhpEnabled:              site.PHP && site.PhpVersion != "" && site.PhpVersion != "static",
		CreatedAt:    site.CreatedAt,
	}
}

func (s *Service) sslCertMap() map[string]models.SSLCertificate {
	var certs []models.SSLCertificate
	s.db.Find(&certs)
	m := map[string]models.SSLCertificate{}
	for _, c := range certs {
		m[strings.ToLower(c.Domain)] = c
	}
	return m
}

func (s *Service) siteTrafficStats(domain string) (total, today int64) {
	todayPrefix := time.Now().Format("02/Jan/2006")
	for _, suffix := range []string{"", "_ssl"} {
		logPath := filepath.Join(s.dataDir, "logs", domain+suffix+"_access.log")
		addAccessLogBytes(logPath, todayPrefix, &total, &today)
	}
	return total, today
}

var nginxAccessLogRe = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) (\S+) [^"]*" (\d+) (\d+)`)

func addAccessLogBytes(logPath, todayPrefix string, total, today *int64) {
	f, err := os.Open(logPath)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		m := nginxAccessLogRe.FindStringSubmatch(sc.Text())
		if len(m) < 7 {
			continue
		}
		n, err := strconv.ParseInt(m[6], 10, 64)
		if err != nil || n <= 0 {
			continue
		}
		*total += n
		if strings.HasPrefix(m[2], todayPrefix) {
			*today += n
		}
	}
}

func (s *Service) activeWebServer() string {
	if s.ws != nil {
		return s.ws.GetActive()
	}
	var row models.PanelSetting
	if s.db.Where("key = ?", "active_web_server").First(&row).Error == nil && row.Value != "" {
		return row.Value
	}
	return "nginx"
}

func (s *Service) ToggleSite(id uint, status string) (*models.Website, error) {
	status = strings.TrimSpace(status)
	if status != "running" && status != "stopped" {
		return nil, errBadStatus
	}
	site, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if status == "running" && IsSiteExpired(site.ExpiresAt) {
		return nil, errSiteExpired
	}
	if err := s.db.Model(site).Update("status", status).Error; err != nil {
		return nil, err
	}
	site.Status = status
	if err := s.applyVhost(site); err != nil {
		return nil, err
	}
	return site, nil
}

func (s *Service) BatchDelete(ids []uint) error {
	var firstErr error
	for _, id := range ids {
		if err := s.Delete(id); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

var errBadStatus = &siteError{msg: "状态只能是 running 或 stopped"}
var errSiteExpired = &siteError{msg: "网站已到期，请先延长到期时间后再启动"}

type siteError struct{ msg string }

func (e *siteError) Error() string { return e.msg }

func (s *Service) WebServerOverview() (interface{}, error) {
	if s.ws == nil {
		return nil, nil
	}
	return s.ws.Overview()
}

func (s *Service) StartWebServer(key string) error {
	if s.ws == nil {
		return errNoWebServerMgr
	}
	return s.ws.StartExclusive(key)
}

var errNoWebServerMgr = &siteError{msg: "web server manager unavailable"}

func (s *Service) vhostDir(webServer string) string {
	return webserver.VhostDir(s.dataDir, webServer)
}
