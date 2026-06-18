package website

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/php"
	sslpkg "github.com/luuuunet/owpanel/internal/services/ssl"
)

type sslOpts struct {
	enabled   bool
	fullchain string
	privkey   string
}

func (s *Service) writeNginxVhost(site *models.Website) (string, error) {
	if site.Status == "stopped" {
		return s.writeStoppedVhost(site, "nginx")
	}
	features, err := s.buildNginxFeatures(site)
	if err != nil {
		return "", err
	}
	confDir := s.vhostDir("nginx")
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
		if site.SSL && site.ForceHTTPS && port == 80 {
			blocks = append(blocks, s.httpRedirectBlock(site, hosts))
			continue
		}
		block, err := s.renderServerBlock(site, root, port, hosts, sslOpts{}, features)
		if err != nil {
			return "", err
		}
		blocks = append(blocks, block)
	}
	if site.SSL {
		if sslBlock := s.sslServerBlock(site, root, site.Domain, features); sslBlock != "" {
			blocks = append(blocks, sslBlock)
		}
	}
	content := fmt.Sprintf("# OWPanel — %s\n%s\n", site.Domain, strings.Join(blocks, "\n"))
	logDir := filepath.Join(s.dataDir, "logs")
	_ = os.MkdirAll(logDir, 0755)
	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		return "", err
	}
	return confPath, nil
}

func (s *Service) httpRedirectBlock(site *models.Website, hosts []string) string {
	names := strings.Join(hosts, " ")
	root := filepath.ToSlash(site.RootPath)
	return fmt.Sprintf(`server {
    listen 80;
    server_name %s;
    location ^~ /.well-known/acme-challenge/ {
        root %s;
        allow all;
    }
    location / {
        return 301 https://$host$request_uri;
    }
}`, names, root)
}

func (s *Service) renderServerBlock(site *models.Website, root string, port int, hosts []string, ssl sslOpts, features *nginxFeatureBlocks) (string, error) {
	names := strings.Join(hosts, " ")
	logSuffix := ""
	if ssl.enabled {
		logSuffix = "_ssl"
	}
	accessLog := filepath.ToSlash(filepath.Join(s.dataDir, "logs", site.Domain+logSuffix+"_access.log"))
	errorLog := filepath.ToSlash(filepath.Join(s.dataDir, "logs", site.Domain+logSuffix+"_error.log"))

	phpBlock := ""
	cachePHP := ""
	if site.PHP && site.PhpVersion != "" && site.PhpVersion != "static" {
		fcgi := php.FastCGIBackend(site.PhpVersion)
		if s.cache != nil {
			cachePHP = s.cache.PHPLocationDirectives(site)
		}
		phpBlock = fmt.Sprintf(`
    location ~ \.php$ {
        fastcgi_pass %s;
        fastcgi_index index.php;
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;%s
    }`, fcgi, cachePHP)
	}

	cacheProxy, cacheStatic, cacheRoot, cacheServer, cacheAccessLog := "", "", "", "", ""
	if s.cache != nil {
		cacheProxy = s.cache.ProxyLocationDirectives(site)
		cacheStatic = s.cache.StaticLocationDirectives(site)
		cacheRoot = s.cache.RootLocationDirectives(site)
		cacheServer = s.cache.ServerBlockDirectives(site)
		if s.cache.SiteEnabled(site) {
			cacheAccessLog = fmt.Sprintf("\n    access_log %s opanel_cache;",
				filepath.ToSlash(s.cache.SiteCacheLogPath(site.Domain)))
		}
	}

	indexLine, tryFallback := s.indexConfig(site)

	rewriteBlock := ""
	if rules := strings.TrimSpace(site.RewriteRules); rules != "" {
		rewriteBlock = "\n    " + strings.ReplaceAll(rules, "\n", "\n    ") + "\n"
	}
	extraBlock := ""
	if extra := strings.TrimSpace(site.ExtraNginxConf); extra != "" {
		extraBlock = "\n    " + strings.ReplaceAll(extra, "\n", "\n    ") + "\n"
	}

	cacheRuleLocs := ""
	if s.cache != nil {
		cacheRuleLocs = s.cache.RuleLocationBlocks(site)
	}

	edgeWorkerBlock := ""
	if s.edgeWorker != nil {
		edgeWorkerBlock = s.edgeWorker.ServerBlockDirectives(site)
	}

	locationBlock := s.mainLocationBlock(site, tryFallback, cacheProxy, cacheRoot)
	if rewriteRulesDefineRootLocation(site.RewriteRules) {
		locationBlock = ""
	}

	listenLine := fmt.Sprintf("listen %d;", port)
	sslLines := ""
	if ssl.enabled {
		listenLine = "listen 443 ssl;"
		sslLines = fmt.Sprintf(`
    ssl_certificate %s;
    ssl_certificate_key %s;`, ssl.fullchain, ssl.privkey)
	}

	return fmt.Sprintf(`server {
    %s
    server_name %s;
    root %s;
    %s;
%s%s%s%s%s
    access_log %s;
    error_log %s;%s%s
%s%s%s%s%s%s%s%s
    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff2?|webp|avif|mp4|webm|pdf|zip|bmp)$ {
        access_log off;%s%s
    }
}`, listenLine, names, root, indexLine, sslLines, features.access, features.geo, features.bots, features.traffic,
		accessLog, errorLog, cacheAccessLog, features.crossSite, cacheServer, rewriteBlock, extraBlock, features.subdirs, cacheRuleLocs, edgeWorkerBlock, locationBlock, phpBlock, cacheStatic, features.staticEx), nil
}

func (s *Service) indexConfig(site *models.Website) (indexLine, tryFallback string) {
	indexLine = strings.TrimSpace(site.IndexFiles)
	tryFallback = "/index.html"
	if indexLine == "" {
		indexLine = "index index.html"
		if site.PHP && site.PhpVersion != "static" {
			indexLine = "index index.php index.html"
			tryFallback = "/index.php?$args"
		}
	} else {
		if !strings.HasPrefix(indexLine, "index ") {
			indexLine = "index " + indexLine
		}
		if strings.Contains(indexLine, "index.php") {
			tryFallback = "/index.php?$args"
		}
	}
	return indexLine, tryFallback
}

func (s *Service) mainLocationBlock(site *models.Website, tryFallback, cacheProxy, cacheRoot string) string {
	if proxy := strings.TrimSpace(site.ProxyPass); proxy != "" {
		return fmt.Sprintf(`
    location / {
        proxy_pass %s;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffer_size 128k;
        proxy_buffers 8 256k;
        proxy_busy_buffers_size 256k;%s
    }`, proxy, cacheProxy)
	}
	if redirect := strings.TrimSpace(site.RedirectURL); redirect != "" {
		return fmt.Sprintf(`
    location / {
        return 301 %s;
    }`, redirect)
	}
	return fmt.Sprintf(`
    location / {
        try_files $uri $uri/ %s;%s
    }`, tryFallback, cacheRoot)
}

func (s *Service) sslServerBlock(site *models.Website, root, primary string, features *nginxFeatureBlocks) string {
	fullchain, privkey, ok := sslpkg.CertPaths(s.dataDir, primary)
	if !ok {
		return ""
	}
	fullchain = filepath.ToSlash(fullchain)
	privkey = filepath.ToSlash(privkey)
	names := primary
	if len(site.Aliases) == 0 {
		s.db.Where("website_id = ?", site.ID).Find(&site.Aliases)
	}
	for _, a := range site.Aliases {
		if a.Type != "primary" {
			names += " " + a.Domain
		}
	}
	block, err := s.renderServerBlock(site, root, 443, strings.Fields(names), sslOpts{
		enabled: true, fullchain: fullchain, privkey: privkey,
	}, features)
	if err != nil {
		return ""
	}
	return block
}

func (s *Service) removeNginxVhost(domain string) {
	confPath := filepath.Join(s.vhostDir("nginx"), domain+".conf")
	_ = os.Remove(confPath)
}

func (s *Service) writeStoppedVhost(site *models.Website, webServer string) (string, error) {
	confDir := s.vhostDir(webServer)
	_ = os.MkdirAll(confDir, 0755)
	confPath := filepath.Join(confDir, site.Domain+".conf")
	if webServer == "apache" {
		content := fmt.Sprintf(`# Site stopped: %s
<VirtualHost *:%d>
    ServerName %s
    DocumentRoot "%s"
</VirtualHost>`, site.Domain, site.Port, site.Domain, filepath.ToSlash(site.RootPath))
		return confPath, os.WriteFile(confPath, []byte(content), 0644)
	}
	content := fmt.Sprintf(`# Site stopped: %s
server {
    listen %d;
    server_name %s;
    return 503;
}`, site.Domain, site.Port, site.Domain)
	return confPath, os.WriteFile(confPath, []byte(content), 0644)
}

func (s *Service) writeVhostOnly(site *models.Website) (string, error) {
	if s.isWordPressManaged(site.ID) {
		ws := site.WebServer
		if ws == "" {
			ws = s.activeWebServer()
		}
		return ws, nil
	}
	ws := site.WebServer
	if ws == "" {
		ws = s.activeWebServer()
	}
	var conf string
	var err error
	if ws == "apache" {
		conf, err = s.writeApacheVhost(site)
		s.removeNginxVhost(site.Domain)
	} else {
		conf, err = s.writeNginxVhost(site)
		s.removeApacheVhost(site.Domain)
	}
	if err != nil {
		return "", err
	}
	if err := s.db.Model(site).Updates(map[string]interface{}{
		"nginx_conf": conf,
		"web_server": ws,
	}).Error; err != nil {
		return "", err
	}
	return ws, nil
}

func (s *Service) reloadWebServer(ws string) error {
	if ws == "" || s.ws == nil {
		return nil
	}
	s.ws.EnsureVhostInclude(ws)
	return s.ws.Reload(ws)
}

func (s *Service) applyVhost(site *models.Website) error {
	ws, err := s.writeVhostOnly(site)
	if err != nil {
		return err
	}
	return s.reloadWebServer(ws)
}

func (s *Service) isWordPressManaged(websiteID uint) bool {
	if websiteID == 0 {
		return false
	}
	var n int64
	s.db.Model(&models.WordPressSite{}).Where("website_id = ?", websiteID).Count(&n)
	return n > 0
}
