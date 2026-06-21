package appstore

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/platform"
)

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

func resolveStackScriptDir() string {
	candidates := []string{
		filepath.Join(os.Getenv("OWPANEL_HOME"), "scripts", "stack"),
		"/opt/owpanel/scripts/stack",
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "scripts", "stack"))
	}
	for _, dir := range candidates {
		if fileExists(filepath.Join(dir, "fallback.sh")) {
			return dir
		}
	}
	return ""
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
	logInstallLine(fmt.Sprintf("apt 安装失败，尝试 stack 多通道安装 %s …", component))

	scriptDir := resolveStackScriptDir()
	if scriptDir != "" {
		script := filepath.Join(scriptDir, "fallback.sh")
		return runCommand("bash", script, component)
	}

	logInstallLine("本地 stack 脚本不可用，从 GitHub 拉取 …")
	if _, err := exec.LookPath("curl"); err != nil {
		return fmt.Errorf("curl 不可用，无法拉取 stack 脚本")
	}
	url := stackFallbackRemoteBase + "/fallback.sh"
	cmd := fmt.Sprintf("curl -fsSL '%s' | bash -s -- %s", url, component)
	return runCommand("bash", "-c", cmd)
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
