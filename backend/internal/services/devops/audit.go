package devops

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

type AuditItem struct {
	Category string `json:"category"`
	Target   string `json:"target"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	Solution string `json:"solution,omitempty"`
}

type AuditReport struct {
	Items      []AuditItem `json:"items"`
	PassCount  int         `json:"pass_count"`
	WarnCount  int         `json:"warn_count"`
	FailCount  int         `json:"fail_count"`
}

func (s *Service) ConfigAudit() (*AuditReport, error) {
	var items []AuditItem
	items = append(items, s.auditWebsites()...)
	items = append(items, s.auditSystemPaths()...)
	items = append(items, s.auditPHPSettings()...)

	report := &AuditReport{Items: items}
	for _, it := range items {
		switch it.Status {
		case "pass":
		 report.PassCount++
		case "fail":
		 report.FailCount++
		default:
		 report.WarnCount++
		}
	}
	return report, nil
}

func (s *Service) auditWebsites() []AuditItem {
	var sites []models.Website
	_ = s.db.Find(&sites).Error
	var items []AuditItem
	for _, site := range sites {
		confPath := strings.TrimSpace(site.NginxConf)
		if confPath == "" {
			confPath = filepath.Join(s.dataDir, "nginx", "vhost", site.Domain+".conf")
		}
		data, err := os.ReadFile(confPath)
		if err != nil {
			items = append(items, AuditItem{
				Category: "nginx", Target: site.Domain, Status: "warn",
				Detail: "虚拟主机文件不存在: " + confPath,
				Solution: "在网站设置中点击「应用配置」重新生成",
			})
			continue
		}
		onDisk := md5Hex(data)
		items = append(items, AuditItem{
			Category: "nginx", Target: site.Domain, Status: "pass",
			Detail: fmt.Sprintf("配置文件存在 (md5: %s…)", onDisk[:8]),
		})
		if strings.Contains(string(data), site.RootPath) {
			items = append(items, AuditItem{
				Category: "nginx", Target: site.Domain + " root", Status: "pass",
				Detail: "root 路径与面板一致",
			})
		} else {
			items = append(items, AuditItem{
				Category: "nginx", Target: site.Domain + " root", Status: "fail",
				Detail: "配置中 root 与面板记录不一致，可能被手动修改",
				Solution: "对比面板站点目录与 nginx 配置中的 root 指令",
			})
		}
	}
	return items
}

func (s *Service) auditSystemPaths() []AuditItem {
	var items []AuditItem
	candidates := []struct {
		path, name string
	}{
		{filepath.Join(s.dataDir, "nginx"), "面板 Nginx 配置目录"},
		{"/etc/nginx/nginx.conf", "系统 Nginx 主配置"},
		{"/etc/php/8.3/fpm/php.ini", "PHP 8.3 配置"},
	}
	for _, c := range candidates {
		if _, err := os.Stat(c.path); err != nil {
			continue
		}
		items = append(items, AuditItem{
			Category: "system", Target: c.name, Status: "warn",
			Detail: "检测到系统级配置文件 " + c.path + "，面板外修改可能导致不一致",
			Solution: "使用面板「配置一致性检查」定期核对，或统一由面板管理",
		})
	}
	suspicious := []string{"/etc/cron.d", "/etc/systemd/system"}
	for _, dir := range suspicious {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := strings.ToLower(e.Name())
			if strings.Contains(name, "miner") || strings.Contains(name, "xmr") {
				items = append(items, AuditItem{
					Category: "security", Target: dir+"/"+e.Name(), Status: "fail",
					Detail: "发现可疑定时/服务项",
					Solution: "立即检查并移除未授权脚本",
				})
			}
		}
	}
	return items
}

func (s *Service) auditPHPSettings() []AuditItem {
	var items []AuditItem
	iniPaths := []string{
		filepath.Join(s.dataDir, "php", "php.ini"),
		"/etc/php/8.3/fpm/php.ini",
		"/etc/php/8.2/fpm/php.ini",
	}
	displayErr := regexp.MustCompile(`(?i)display_errors\s*=\s*On`)
	for _, p := range iniPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		content := string(data)
		if displayErr.MatchString(content) {
			items = append(items, AuditItem{
				Category: "php", Target: p, Status: "warn",
				Detail: "display_errors=On 可能泄露敏感信息",
				Solution: "生产环境设置为 Off",
			})
		} else {
			items = append(items, AuditItem{
				Category: "php", Target: filepath.Base(p), Status: "pass",
				Detail: "display_errors 已关闭或未启用",
			})
		}
	}
	return items
}

func md5Hex(data []byte) string {
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}
