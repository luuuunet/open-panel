package website

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

func (s *Service) writeApacheVhost(site *models.Website) (string, error) {
	if site.Status == "stopped" {
		return s.writeStoppedVhost(site, "apache")
	}
	confDir := s.vhostDir("apache")
	_ = os.MkdirAll(confDir, 0755)
	confPath := filepath.Join(confDir, site.Domain+".conf")
	root := filepath.ToSlash(site.RootPath)

	entries := s.allDomainEntries(site)
	if len(entries) == 0 {
		entries = []domainEntry{{Host: site.Domain, Port: site.Port}}
	}
	groups := groupByPort(entries)

	var blocks []string
	for port, hosts := range groups {
		block, err := s.apacheVirtualHost(site, root, port, hosts)
		if err != nil {
			return "", err
		}
		blocks = append(blocks, block)
	}
	content := fmt.Sprintf("# OWPanel — %s\n%s\n", site.Domain, strings.Join(blocks, "\n"))
	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		return "", err
	}
	return confPath, nil
}

func (s *Service) apacheVirtualHost(site *models.Website, root string, port int, hosts []string) (string, error) {
	if site.Status == "stopped" {
		return fmt.Sprintf(`# Site stopped: %s
<VirtualHost *:%d>
    ServerName %s
    DocumentRoot "%s"
    ErrorDocument 403 /503.html
</VirtualHost>`, site.Domain, port, site.Domain, root), nil
	}

	names := strings.Join(hosts, " ")
	accessLog := filepath.ToSlash(filepath.Join(s.dataDir, "logs", site.Domain+"_access.log"))
	errorLog := filepath.ToSlash(filepath.Join(s.dataDir, "logs", site.Domain+"_error.log"))

	indexLine := strings.TrimSpace(site.IndexFiles)
	if indexLine == "" {
		indexLine = "index.html"
		if site.PHP && site.PhpVersion != "static" {
			indexLine = "index.php index.html"
		}
	}

	phpBlock := ""
	if site.PHP && site.PhpVersion != "" && site.PhpVersion != "static" {
		fcgiPort := phpPort(site.PhpVersion)
		phpBlock = fmt.Sprintf(`
    <FilesMatch \.php$>
        SetHandler "proxy:fcgi://127.0.0.1:%d"
    </FilesMatch>`, fcgiPort)
	}

	rewriteBlock := ""
	if rules := strings.TrimSpace(site.RewriteRules); rules != "" {
		rewriteBlock = "\n    " + strings.ReplaceAll(rules, "\n", "\n    ") + "\n"
	}

	crossSiteBlock := ""
	if site.CrossSiteProtectEnabled {
		crossSiteBlock = `
    Header always set X-Frame-Options "SAMEORIGIN"
    Header always set Content-Security-Policy "frame-ancestors 'self'"
    Header always set Referrer-Policy "strict-origin-when-cross-origin"`
	}

	proxyBlock := ""
	if proxy := strings.TrimSpace(site.ProxyPass); proxy != "" {
		proxyBlock = fmt.Sprintf(`
    ProxyPreserveHost On
    ProxyPass / %s/
    ProxyPassReverse / %s/`, proxy, proxy)
	} else if redirect := strings.TrimSpace(site.RedirectURL); redirect != "" {
		proxyBlock = fmt.Sprintf(`
    RedirectMatch 301 ^/(.*)$ %s`, redirect)
	}

	return fmt.Sprintf(`<VirtualHost *:%d>
    ServerName %s
    DocumentRoot "%s"
    DirectoryIndex %s
    ErrorLog "%s"
    CustomLog "%s" combined
%s%s%s%s
    <Directory "%s">
        Options Indexes FollowSymLinks
        AllowOverride All
        Require all granted
    </Directory>
</VirtualHost>`, port, names, root, indexLine, errorLog, accessLog, rewriteBlock, crossSiteBlock, proxyBlock, phpBlock, root), nil
}

func (s *Service) removeApacheVhost(domain string) {
	confPath := filepath.Join(s.vhostDir("apache"), domain+".conf")
	_ = os.Remove(confPath)
}
