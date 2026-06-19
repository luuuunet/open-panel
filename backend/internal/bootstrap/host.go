package bootstrap

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/luuuunet/owpanel/internal/services/appstore"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"github.com/luuuunet/owpanel/internal/services/webserver"
)

const hostBootstrapKey = "host_bootstrap_v1"

// Host reconciles OS packages with the app catalog and prepares nginx/vhost layout on first start.
func Host(apps *appstore.Service, ws *webserver.Manager, settingsSvc *settings.Service, dataDir string) {
	if settingsSvc == nil || apps == nil {
		return
	}
	all, err := settingsSvc.GetAll()
	if err == nil && all[hostBootstrapKey] == "done" {
		return
	}

	log.Println("[bootstrap] preparing host environment (first run)...")
	start := time.Now()

	TuneMemory(settingsSvc)
	ensureDataLayout(dataDir)
	catalogCount := 0
	for attempt := 0; attempt < 5 && catalogCount == 0; attempt++ {
		catalogCount = apps.SyncCatalog()
		if catalogCount == 0 && attempt < 4 {
			time.Sleep(time.Second)
		}
	}
	if catalogCount > 0 {
		log.Printf("[bootstrap] software store catalog ready (%d apps)", catalogCount)
	} else {
		log.Println("[bootstrap] software store catalog empty — leaving bootstrap pending for retry")
	}
	go func() {
		if _, err := apps.RefreshStoreVersions(); err != nil {
			log.Printf("[bootstrap] refresh store versions: %v", err)
		}
	}()
	apps.ReconcileInstalledFromSystem()

	if ws != nil && runtime.GOOS == "linux" {
		ensureWebServer(ws, apps, dataDir)
	}
	startInstalledServices(apps)

	settingsPatch := map[string]string{
		"website_path": filepath.Join(dataDir, "wwwroot"),
	}
	if catalogCount > 0 {
		settingsPatch[hostBootstrapKey] = "done"
	}
	if err := settingsSvc.Update(settingsPatch); err != nil {
		log.Printf("[bootstrap] save settings: %v", err)
	} else {
		log.Printf("[bootstrap] host ready in %s", time.Since(start).Round(time.Millisecond))
	}
}

func ensureDataLayout(dataDir string) {
	for _, sub := range []string{"wwwroot", "nginx/vhosts", "logs", "backups", "server"} {
		_ = os.MkdirAll(filepath.Join(dataDir, sub), 0755)
	}
	panelConf := filepath.Join(dataDir, "nginx", "owpanel.conf")
	if _, err := os.Stat(panelConf); os.IsNotExist(err) {
		vhostDir := filepath.Join(dataDir, "nginx", "vhosts")
		content := "# OWPanel auto-generated\ninclude " + filepath.ToSlash(vhostDir) + "/*.conf;\n"
		_ = os.WriteFile(panelConf, []byte(content), 0644)
	}
}

func ensureWebServer(ws *webserver.Manager, apps *appstore.Service, dataDir string) {
	active := ws.GetActive()
	if active == "" {
		active = "nginx"
	}
	order := []string{active, "nginx", "openresty", "apache"}
	seen := map[string]bool{}
	for _, key := range order {
		if seen[key] || key == "" {
			continue
		}
		seen[key] = true
		if !webserver.IsWebServerKey(key) {
			continue
		}
		app, err := apps.Get(key)
		if err != nil || !app.Installed {
			if !appstore.SystemPackagePresent(key, dataDir) {
				continue
			}
		}
		if err := ws.Bootstrap(key); err != nil {
			log.Printf("[bootstrap] webserver %s config: %v", key, err)
			continue
		}
		if apps.LiveStatus(key) != "running" {
			if err := ws.StartExclusive(key); err != nil {
				log.Printf("[bootstrap] start %s: %v", key, err)
			}
		}
		return
	}
}

func startInstalledServices(apps *appstore.Service) {
	keys := []string{"php83", "php82", "php81", "mysql", "mariadb", "pureftpd", "redis"}
	for _, key := range keys {
		app, err := apps.Get(key)
		if err != nil || !app.Installed {
			continue
		}
		if apps.LiveStatus(key) == "running" {
			continue
		}
		if err := apps.ServiceAction(key, "start"); err != nil {
			log.Printf("[bootstrap] start %s: %v", key, err)
		}
	}
}
