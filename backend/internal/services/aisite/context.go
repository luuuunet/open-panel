package aisite

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/appstore"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"github.com/luuuunet/owpanel/internal/services/website"
)

type PanelContext struct {
	OS              string   `json:"os"`
	Arch            string   `json:"arch"`
	WebServer       string   `json:"web_server"`
	WebsiteRoot     string   `json:"website_root"`
	PHPVersions     []string `json:"php_versions"`
	InstalledApps   []string `json:"installed_apps"`
	GitAvailable    bool     `json:"git_available"`
	DockerAvailable bool     `json:"docker_available"`
	NodeAvailable   bool     `json:"node_available"`
	MySQLAvailable  bool     `json:"mysql_available"`
	ComposerAvail   bool     `json:"composer_available"`
	NPMAvailable    bool     `json:"npm_available"`
}

func (s *Service) collectPanelContext() PanelContext {
	ctx := PanelContext{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}
	all, _ := s.settings.GetAll()
	if all != nil {
		ctx.WebsiteRoot = all["website_path"]
	}
	if ctx.WebsiteRoot == "" {
		ctx.WebsiteRoot = settings.DefaultWebsitePath(s.dataDir)
	}
	if s.website != nil {
		// active web server via settings
		if all != nil {
			ctx.WebServer = all["active_web_server"]
		}
		if ctx.WebServer == "" {
			ctx.WebServer = "nginx"
		}
	}
	ctx.PHPVersions = []string{"static", "8.3", "8.2", "8.1", "7.4"}
	if apps, err := s.appstore.List(); err == nil {
		for _, a := range apps {
			if !a.Installed {
				continue
			}
			ctx.InstalledApps = append(ctx.InstalledApps, a.Key)
			switch a.Key {
			case "mysql", "mariadb":
				ctx.MySQLAvailable = true
			case "docker":
				ctx.DockerAvailable = true
			case "nodejs", "nodejs20", "nodejs18":
				ctx.NodeAvailable = true
			}
			if strings.HasPrefix(a.Key, "php") {
				ver := strings.TrimPrefix(a.Key, "php")
				if ver != "" && !containsStr(ctx.PHPVersions, ver) {
					ctx.PHPVersions = append(ctx.PHPVersions, ver)
				}
			}
		}
	}
	_, err := exec.LookPath("git")
	ctx.GitAvailable = err == nil || gitAvailable()
	_, err = exec.LookPath("composer")
	ctx.ComposerAvail = err == nil
	if !ctx.ComposerAvail && appstore.ComposerBinary(s.dataDir) != "" {
		ctx.ComposerAvail = true
	}
	_, err = exec.LookPath("npm")
	ctx.NPMAvailable = err == nil
	if _, err := exec.LookPath("docker"); err == nil {
		ctx.DockerAvailable = true
	}
	return ctx
}

func containsStr(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func panelContextJSON(ctx PanelContext) string {
	var b strings.Builder
	b.WriteString("OS: ")
	b.WriteString(ctx.OS)
	b.WriteString("\nWeb server: ")
	b.WriteString(ctx.WebServer)
	b.WriteString("\nWebsite root pattern: ")
	b.WriteString(ctx.WebsiteRoot)
	b.WriteString("/{domain}\n")
	b.WriteString("PHP versions available: ")
	b.WriteString(strings.Join(ctx.PHPVersions, ", "))
	b.WriteString("\n")
	b.WriteString("Git: ")
	b.WriteString(boolYes(ctx.GitAvailable))
	b.WriteString(", Composer: ")
	b.WriteString(boolYes(ctx.ComposerAvail))
	b.WriteString(", NPM: ")
	b.WriteString(boolYes(ctx.NPMAvailable))
	b.WriteString(", Docker: ")
	b.WriteString(boolYes(ctx.DockerAvailable))
	b.WriteString(", MySQL: ")
	b.WriteString(boolYes(ctx.MySQLAvailable))
	b.WriteString("\nInstalled apps: ")
	b.WriteString(strings.Join(ctx.InstalledApps, ", "))
	return b.String()
}

func boolYes(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func buildSiteContextJSON(site *models.Website, createRes *website.CreateResult, panel PanelContext, dataDir string) string {
	var b strings.Builder
	b.WriteString(panelContextJSON(panel))
	b.WriteString("\n\n--- Site (already created) ---\n")
	b.WriteString(fmt.Sprintf("Site ID: %d\n", site.ID))
	b.WriteString(fmt.Sprintf("Domain: %s\n", site.Domain))
	b.WriteString(fmt.Sprintf("Install directory (site root, cwd for deploy_script): %s\n", site.RootPath))
	b.WriteString(fmt.Sprintf("PHP version: %s\n", site.PhpVersion))
	if site.NginxConf != "" {
		b.WriteString(fmt.Sprintf("Nginx vhost config: %s\n", site.NginxConf))
	}
	if createRes != nil && createRes.DbName != "" {
		b.WriteString("\nDatabase (MySQL):\n")
		b.WriteString("  DB_HOST=127.0.0.1\n")
		b.WriteString(fmt.Sprintf("  DB_DATABASE=%s\n", createRes.DbName))
		b.WriteString(fmt.Sprintf("  DB_USERNAME=%s\n", createRes.DbUser))
		b.WriteString(fmt.Sprintf("  DB_PASSWORD=%s\n", createRes.DbPassword))
	}
	if panel.ComposerAvail {
		b.WriteString("Composer: available in PATH\n")
	} else if bin := appstore.ComposerBinary(dataDir); bin != "" {
		b.WriteString(fmt.Sprintf("Composer: %s\n", bin))
	} else {
		b.WriteString("Composer: not installed; deploy_script may download composer.phar\n")
	}
	return b.String()
}
