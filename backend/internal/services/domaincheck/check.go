package domaincheck

import (
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

type Scope struct {
	IgnoreWebsiteID         uint
	IgnoreWPSiteID          uint
	IgnoreAppKey            string
	IgnoreDockerContainerID string
}

type Conflict struct {
	Domain string `json:"domain"`
	Owner  string `json:"owner"`
	Type   string `json:"type"`
	ID     uint   `json:"id"`
}

func Normalize(raw string) string {
	d := strings.TrimSpace(strings.ToLower(raw))
	d = strings.TrimPrefix(d, "http://")
	d = strings.TrimPrefix(d, "https://")
	if idx := strings.Index(d, "/"); idx >= 0 {
		d = d[:idx]
	}
	return d
}

// HostOnly strips :port from domain strings like www.example.com:8080
func HostOnly(raw string) string {
	d := Normalize(raw)
	if idx := strings.LastIndex(d, ":"); idx > 0 && !strings.Contains(d, "]") {
		portPart := d[idx+1:]
		allDigit := portPart != ""
		for _, c := range portPart {
			if c < '0' || c > '9' {
				allDigit = false
				break
			}
		}
		if allDigit {
			d = d[:idx]
		}
	}
	return d
}

func Find(db *gorm.DB, domain string, scope Scope) *Conflict {
	host := HostOnly(domain)
	if host == "" {
		return nil
	}

	var w models.Website
	q := db.Where("domain = ?", host)
	if scope.IgnoreWebsiteID > 0 {
		q = q.Where("id <> ?", scope.IgnoreWebsiteID)
	}
	if q.First(&w).Error == nil {
		return &Conflict{Domain: host, Owner: fmt.Sprintf("网站「%s」", w.Domain), Type: "website", ID: w.ID}
	}

	var alias models.WebsiteAlias
	q2 := db.Where("domain = ?", host)
	if scope.IgnoreWebsiteID > 0 {
		q2 = q2.Where("website_id <> ?", scope.IgnoreWebsiteID)
	}
	if q2.First(&alias).Error == nil {
		return &Conflict{Domain: host, Owner: fmt.Sprintf("网站绑定域名（站点 ID %d）", alias.WebsiteID), Type: "website_alias", ID: alias.WebsiteID}
	}

	var wp models.WordPressSite
	q3 := db.Where("domain = ?", host)
	if scope.IgnoreWPSiteID > 0 {
		q3 = q3.Where("id <> ?", scope.IgnoreWPSiteID)
	}
	if q3.First(&wp).Error == nil {
		status := wp.Status
		if status == "deploying" {
			return &Conflict{Domain: host, Owner: fmt.Sprintf("WordPress 站点「%s」正在部署中", wp.Domain), Type: "wordpress_deploying", ID: wp.ID}
		}
		return &Conflict{Domain: host, Owner: fmt.Sprintf("WordPress 站点「%s」", wp.Domain), Type: "wordpress", ID: wp.ID}
	}

	var wpd models.WordPressDomain
	q4 := db.Where("domain = ? AND enabled = ?", host, true)
	if scope.IgnoreWPSiteID > 0 {
		q4 = q4.Where("site_id <> ?", scope.IgnoreWPSiteID)
	}
	if q4.First(&wpd).Error == nil {
		return &Conflict{Domain: host, Owner: fmt.Sprintf("WordPress 绑定域名（站点 ID %d）", wpd.SiteID), Type: "wp_domain", ID: wpd.SiteID}
	}

	var app models.App
	q5 := db.Where("bind_domain = ? AND installed = ?", host, true)
	if scope.IgnoreAppKey != "" {
		q5 = q5.Where("app_key <> ?", scope.IgnoreAppKey)
	}
	if q5.First(&app).Error == nil {
		return &Conflict{Domain: host, Owner: fmt.Sprintf("软件商店「%s」", app.Name), Type: "software_app", ID: app.ID}
	}

	var dockerBind models.DockerContainerBinding
	q6 := db.Where("domain = ?", host)
	if scope.IgnoreDockerContainerID != "" {
		q6 = q6.Where("container_id <> ?", scope.IgnoreDockerContainerID)
	}
	if q6.First(&dockerBind).Error == nil {
		owner := dockerBind.ContainerName
		if owner == "" {
			owner = dockerBind.ContainerID
		}
		return &Conflict{Domain: host, Owner: fmt.Sprintf("Docker 容器「%s」", owner), Type: "docker_container", ID: dockerBind.ID}
	}

	return nil
}

func CheckList(db *gorm.DB, domains []string, scope Scope) []Conflict {
	seen := map[string]bool{}
	var conflicts []Conflict
	for _, raw := range domains {
		host := HostOnly(raw)
		if host == "" {
			continue
		}
		if seen[host] {
			conflicts = append(conflicts, Conflict{Domain: host, Owner: "本次提交中重复", Type: "duplicate"})
			continue
		}
		seen[host] = true
		if c := Find(db, host, scope); c != nil {
			conflicts = append(conflicts, *c)
		}
	}
	return conflicts
}

func AssertAvailable(db *gorm.DB, domains []string, scope Scope) error {
	conflicts := CheckList(db, domains, scope)
	if len(conflicts) == 0 {
		return nil
	}
	c := conflicts[0]
	return fmt.Errorf("域名 %s 已被占用：%s", c.Domain, c.Owner)
}
