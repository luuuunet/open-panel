package appstore

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/platform"
	"github.com/luuuunet/owpanel/internal/stackscripts"
)

// stackFallbackRemoteBase is the default GitHub raw URL (main branch).
const stackFallbackRemoteBase = "https://raw.githubusercontent.com/luuuunet/owpanel/main/scripts/stack"

// stackFallbackComponents lists apps that have multi-channel stack install scripts.
var stackFallbackComponents = map[string]bool{
	"nginx": true, "openresty": true, "apache": true,
	"mariadb": true, "mysql": true,
	"postgresql": true, "redis": true, "mongodb": true,
	"docker": true, "certbot": true,
	"memcached": true, "fail2ban": true, "supervisor": true,
	"pureftpd": true, "postfix": true, "dovecot": true,
}

func stackFallbackSupported(key string) bool {
	if stackFallbackComponents[key] {
		return true
	}
	return strings.HasPrefix(key, "php") && key != "phpmyadmin"
}

func stackFallbackComponent(key string) string {
	if strings.HasPrefix(key, "php") && key != "phpmyadmin" {
		return key
	}
	if key == "mysql" {
		return "mariadb"
	}
	return key
}

func stackScriptsCacheDir() string {
	if d := strings.TrimSpace(os.Getenv("OWPANEL_STACK_DIR")); d != "" {
		return d
	}
	if d := strings.TrimSpace(os.Getenv("OWPANEL_DATA")); d != "" {
		return filepath.Join(d, "cache", "stack-scripts")
	}
	return "/opt/owpanel/data/cache/stack-scripts"
}

func resolveStackScriptDir() string {
	candidates := []string{
		os.Getenv("OWPANEL_STACK_DIR"),
		filepath.Join(os.Getenv("OWPANEL_HOME"), "scripts", "stack"),
		"/opt/owpanel/scripts/stack",
		stackScriptsCacheDir(),
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(exe), "scripts", "stack"),
			filepath.Join(filepath.Dir(exe), "..", "scripts", "stack"),
		)
	}
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		if fileExists(filepath.Join(dir, "fallback.sh")) {
			return dir
		}
	}
	if err := stackscripts.ExtractTo(stackScriptsCacheDir()); err == nil {
		if fileExists(filepath.Join(stackScriptsCacheDir(), "fallback.sh")) {
			return stackScriptsCacheDir()
		}
	}
	return ""
}

func ensureStackScriptDir() (string, error) {
	if dir := resolveStackScriptDir(); dir != "" {
		return dir, nil
	}
	dest := stackScriptsCacheDir()
	logInstallLine(fmt.Sprintf("从 GitHub / 内置资源获取 stack 安装脚本 → %s …", dest))
	if err := stackscripts.DownloadTo(dest); err != nil {
		return "", err
	}
	if !fileExists(filepath.Join(dest, "fallback.sh")) {
		return "", fmt.Errorf("stack 脚本不完整（缺少 fallback.sh）")
	}
	return dest, nil
}

func runStackFallback(key string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("stack fallback only supported on Linux")
	}
	platform.SanitizeBrokenAptRepos()
	if detectLinuxPkgMgr() == "apt" {
		if err := platform.AptGetUpdate("-qq"); err != nil {
			logInstallLine("apt update 警告（已尝试修复源）: " + err.Error())
		}
	}
	component := stackFallbackComponent(key)
	logInstallLine(fmt.Sprintf("使用 stack 多通道安装 %s …", component))

	scriptDir, err := ensureStackScriptDir()
	if err == nil {
		script := filepath.Join(scriptDir, "fallback.sh")
		return runCommand("bash", script, component)
	}
	logInstallLine("本地/缓存 stack 不可用: " + err.Error())

	logInstallLine("从 GitHub 拉取 fallback.sh …")
	if _, lookErr := exec.LookPath("curl"); lookErr != nil {
		return fmt.Errorf("curl 不可用，无法拉取 stack 脚本: %w", err)
	}
	base := stackscripts.RemoteBase()
	url := base + "/fallback.sh"
	cmd := fmt.Sprintf("curl -fsSL '%s' | bash -s -- %s", url, component)
	if runErr := runCommand("bash", "-c", cmd); runErr == nil {
		return nil
	} else {
		err = runErr
	}
	// Last resort: main branch
	if base != stackFallbackRemoteBase {
		url = stackFallbackRemoteBase + "/fallback.sh"
		cmd = fmt.Sprintf("curl -fsSL '%s' | bash -s -- %s", url, component)
		if runErr := runCommand("bash", "-c", cmd); runErr == nil {
			return nil
		} else {
			err = runErr
		}
	}
	return err
}

func installLinuxPackagesWithFallback(key string, spec packageSpec) error {
	platform.SanitizeBrokenAptRepos()
	// Database engines: use stack scripts first on apt (handles repo quirks + broken third-party lists).
	if detectLinuxPkgMgr() == "apt" && stackFallbackSupported(key) {
		switch key {
		case "mongodb", "redis", "postgresql", "mariadb", "mysql":
			if err := runStackFallback(key); err == nil {
				return nil
			}
		}
	}
	// Ubuntu/Debian no longer ship a usable "mongodb" meta package; use stack script directly.
	if key == "mongodb" && detectLinuxPkgMgr() == "apt" && stackFallbackSupported(key) {
		if err := runStackFallback(key); err != nil {
			return err
		}
		return nil
	}
	err := installLinuxPackages(spec)
	if err == nil {
		return nil
	}
	if !stackFallbackSupported(key) {
		return err
	}
	if fbErr := runStackFallback(key); fbErr != nil {
		return fmt.Errorf("apt install: %w; stack fallback: %v", err, fbErr)
	}
	return nil
}
