package sitepurge

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/domaincheck"
	"github.com/luuuunet/owpanel/internal/services/webserver"
	"gorm.io/gorm"
)

// Options controls filesystem cleanup during domain purge.
type Options struct {
	DataDir       string
	RemoveWWWRoot string // remove this directory when safe; empty skips
}

// UniqueHosts normalizes and deduplicates hostnames.
func UniqueHosts(raw []string) []string {
	seen := map[string]bool{}
	var hosts []string
	for _, r := range raw {
		h := domaincheck.HostOnly(r)
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		hosts = append(hosts, h)
	}
	return hosts
}

// Domains removes vhost files and all active panel records that would block domain reuse.
func Domains(db *gorm.DB, hosts []string, opts Options) {
	for _, host := range UniqueHosts(hosts) {
		removeVhosts(opts.DataDir, host)
		purgeDomainRecords(db, host)
	}
	if opts.RemoveWWWRoot != "" {
		removeWWWRootIfSafe(db, opts.DataDir, opts.RemoveWWWRoot)
	}
}

func removeVhosts(dataDir, host string) {
	for _, ws := range []string{"nginx", "apache"} {
		_ = os.Remove(filepath.Join(webserver.VhostDir(dataDir, ws), host+".conf"))
	}
}

func purgeDomainRecords(db *gorm.DB, host string) {
	var wpSites []models.WordPressSite
	db.Where("domain = ?", host).Find(&wpSites)
	for _, wp := range wpSites {
		db.Where("site_id = ?", wp.ID).Delete(&models.WordPressDomain{})
		db.Where("site_id = ?", wp.ID).Delete(&models.WordPressBackup{})
		if wp.NginxConf != "" {
			_ = os.Remove(wp.NginxConf)
		}
		_ = db.Delete(&wp).Error
	}
	db.Where("domain = ?", host).Delete(&models.WordPressDomain{})

	var websites []models.Website
	db.Where("domain = ?", host).Find(&websites)
	for i := range websites {
		w := &websites[i]
		db.Where("website_id = ?", w.ID).Delete(&models.WebsiteBackup{})
		db.Where("website_id = ?", w.ID).Delete(&models.WebsiteSubdir{})
		_ = db.Select("Aliases").Delete(w).Error
	}
	// Orphan aliases survive when the parent website was deleted without cascading aliases.
	db.Where("domain = ?", host).Delete(&models.WebsiteAlias{})
	db.Where("domain = ?", host).Delete(&models.SSLCertificate{})
}

func removeWWWRootIfSafe(db *gorm.DB, dataDir, root string) {
	root = filepath.Clean(root)
	if root == "" || root == string(filepath.Separator) {
		return
	}
	base := filepath.Clean(filepath.Join(dataDir, "wwwroot"))
	if !strings.HasPrefix(root, base+string(filepath.Separator)) && root != base {
		return
	}
	var count int64
	db.Model(&models.Website{}).Where("root_path = ?", root).Count(&count)
	if count > 0 {
		return
	}
	if err := os.RemoveAll(root); err != nil && !os.IsNotExist(err) {
		log.Printf("[sitepurge] remove wwwroot %s: %v", root, err)
	}
}

// PurgeWebsiteID deletes aliases and child rows tied to a website id (any domain).
func PurgeWebsiteID(db *gorm.DB, websiteID uint) {
	if websiteID == 0 {
		return
	}
	db.Where("website_id = ?", websiteID).Delete(&models.WebsiteAlias{})
	db.Where("website_id = ?", websiteID).Delete(&models.WebsiteBackup{})
	db.Where("website_id = ?", websiteID).Delete(&models.WebsiteSubdir{})
	db.Where("website_id = ?", websiteID).Delete(&models.BotCrawlerRule{})
}
