package extension

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Event names extensions can subscribe to via manifest hooks.
const (
	EventPanelStartup    = "panel.startup"
	EventWebsiteCreated  = "website.created"
	EventWebsiteDeleted  = "website.deleted"
	EventAppInstalled    = "app.installed"
	EventAppUninstalled  = "app.uninstalled"
	EventBackupCompleted = "backup.completed"
)

// Emit runs all hook scripts registered for an event (async, best-effort).
func (r *Registry) Emit(event string, payload map[string]interface{}) {
	r.mu.RLock()
	items := append([]loadedExtension(nil), r.items...)
	r.mu.RUnlock()
	if len(items) == 0 {
		return
	}
	data, _ := json.Marshal(payload)
	go func() {
		for _, ext := range items {
			if !ext.enabled {
				continue
			}
			scripts, ok := ext.manifest.Hooks[event]
			if !ok {
				continue
			}
			for _, script := range scripts {
				r.runHookScript(ext.dir, event, script, data)
			}
		}
	}()
}

func (r *Registry) runHookScript(extDir, event, script string, payload []byte) {
	script = strings.TrimSpace(script)
	if script == "" {
		return
	}
	path := script
	if !filepath.IsAbs(path) {
		path = filepath.Join(extDir, script)
	}
	if _, err := os.Stat(path); err != nil {
		log.Printf("[extension] hook script missing %s: %v", path, err)
		return
	}
	ctxFile := filepath.Join(r.dataDir, "extensions", ".hook-payload.json")
	_ = os.WriteFile(ctxFile, payload, 0600)

	cmd := exec.Command(path)
	cmd.Dir = extDir
	cmd.Env = append(os.Environ(),
		"OWPANEL_DATA_DIR="+r.dataDir,
		"OWPANEL_HOOK_EVENT="+event,
		"OWPANEL_HOOK_PAYLOAD="+ctxFile,
	)
	if runtime.GOOS == "windows" {
		if strings.HasSuffix(strings.ToLower(path), ".ps1") {
			cmd = exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", path)
			cmd.Dir = extDir
		} else if strings.HasSuffix(strings.ToLower(path), ".bat") || strings.HasSuffix(strings.ToLower(path), ".cmd") {
			cmd = exec.Command("cmd", "/C", path)
			cmd.Dir = extDir
		}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		if err != nil {
			log.Printf("[extension] hook %s %s: %v", event, path, err)
		}
	case <-time.After(2 * time.Minute):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		log.Printf("[extension] hook timeout %s %s", event, path)
	}
}
