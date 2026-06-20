package appstore

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/settings"
)

func tryDotnetInstall(key, _, installPath, dataDir string) (bool, error) {
	if !strings.HasPrefix(key, "dotnet") {
		return false, nil
	}
	channel := dotnetChannelFromKey(key)
	if channel == "" {
		return false, fmt.Errorf("unsupported .NET key: %s", key)
	}
	return true, installDotnet(dataDir, channel, key, installPath)
}

func dotnetChannelFromKey(key string) string {
	switch key {
	case "dotnet10":
		return "10.0"
	case "dotnet8":
		return "8.0"
	default:
		ver := strings.TrimPrefix(key, "dotnet")
		if ver == "" {
			return ""
		}
		if strings.Contains(ver, ".") {
			return ver
		}
		return ver + ".0"
	}
}

func dotnetMajorFromKey(key string) string {
	ch := dotnetChannelFromKey(key)
	if ch == "" {
		return strings.TrimPrefix(key, "dotnet")
	}
	return strings.Split(ch, ".")[0]
}

func installDotnet(dataDir, channel, key, installPath string) error {
	if runtime.GOOS == "windows" {
		return installDotnetWindows(channel, key, installPath, dataDir)
	}
	base := resolveDotnetBase(dataDir, key, installPath)
	if isSimulatedAt(base) {
		logInstallLine("检测到模拟安装标记，正在清理后重新安装…")
		_ = os.RemoveAll(base)
	}
	_ = os.MkdirAll(base, 0755)

	root := filepath.Join(base, "root")
	_ = os.RemoveAll(root)
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}

	logInstallLine(fmt.Sprintf("正在安装 .NET %s ASP.NET Core 运行时…", channel))
	script := fmt.Sprintf(
		"set -euo pipefail; curl -fsSL https://dot.net/v1/dotnet-install.sh | bash /dev/stdin --channel %s --runtime aspnetcore --install-dir %q",
		channel, root,
	)
	cmd := exec.Command("bash", "-c", script)
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if trimmed != "" {
		for _, line := range strings.Split(trimmed, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				logInstallLine(line)
			}
		}
	}
	if err != nil {
		return fmt.Errorf("安装 .NET %s 失败: %w", channel, err)
	}

	dotnetBin := filepath.Join(root, "dotnet")
	if !fileExists(dotnetBin) {
		return fmt.Errorf(".NET 安装完成但未找到 dotnet 可执行文件")
	}

	wrapperDir := filepath.Join(base, "bin")
	_ = os.MkdirAll(wrapperDir, 0755)
	wrapper := filepath.Join(wrapperDir, "dotnet")
	wrapperScript := fmt.Sprintf("#!/bin/bash\nexport DOTNET_ROOT=%q\nexport PATH=%q:$PATH\nexec %q \"$@\"\n", root, root, dotnetBin)
	if err := os.WriteFile(wrapper, []byte(wrapperScript), 0755); err != nil {
		return err
	}

	marker := filepath.Join(base, ".owpanel-installed")
	content := fmt.Sprintf("dotnet=%s\nchannel=%s\nDOTNET_ROOT=%s\n", dotnetBin, channel, root)
	if err := os.WriteFile(marker, []byte(content), 0644); err != nil {
		return err
	}
	logInstallLine(fmt.Sprintf(".NET %s 安装完成: %s", channel, dotnetBin))
	return nil
}

func installDotnetWindows(channel, key, installPath, dataDir string) error {
	wingetID := dotnetWingetID(channel)
	if wingetID == "" {
		return fmt.Errorf(".NET %s 请手动从 https://dotnet.microsoft.com/download 安装", channel)
	}
	if _, err := exec.LookPath("winget"); err != nil {
		return fmt.Errorf("未找到 winget，请手动安装 %s", wingetID)
	}
	logInstallLine(fmt.Sprintf("正在通过 winget 安装 %s …", wingetID))
	if err := runCommand("winget", "install", "-e", "--id", wingetID, "--accept-package-agreements", "--accept-source-agreements"); err != nil {
		return err
	}
	base := resolveDotnetBase(dataDir, key, installPath)
	_ = os.MkdirAll(base, 0755)
	marker := filepath.Join(base, ".owpanel-installed")
	return os.WriteFile(marker, []byte("winget="+wingetID+"\n"), 0644)
}

func dotnetWingetID(channel string) string {
	switch channel {
	case "8.0":
		return "Microsoft.DotNet.AspNetCore.8"
	case "9.0":
		return "Microsoft.DotNet.AspNetCore.9"
	case "10.0":
		return "Microsoft.DotNet.AspNetCore.10"
	default:
		return ""
	}
}

func resolveDotnetBase(dataDir, key, installPath string) string {
	base := filepath.Join(dataDir, "server", key)
	if resolved := settings.ResolvePanelPath(dataDir, installPath); resolved != "" {
		base = resolved
	}
	return base
}

func isSimulatedAt(base string) bool {
	marker := filepath.Join(base, ".owpanel-installed")
	b, err := os.ReadFile(marker)
	if err != nil {
		return false
	}
	return strings.Contains(string(b), "mode=simulated")
}

func detectDotnetStatus(key, dataDir string) string {
	base := resolveDotnetBase(dataDir, key, filepath.Join("server", "dotnet", dotnetMajorFromKey(key)))
	marker := filepath.Join(base, ".owpanel-installed")
	if fileExists(marker) {
		if isSimulatedAt(base) {
			return "stopped"
		}
		if fileExists(filepath.Join(base, "root", "dotnet")) {
			return "running"
		}
		if b, err := os.ReadFile(marker); err == nil && strings.Contains(string(b), "winget=") {
			return "running"
		}
	}
	if _, err := exec.LookPath("dotnet"); err == nil {
		return "running"
	}
	return "stopped"
}

// DotnetBinary returns panel-managed dotnet path for a store key, or system dotnet.
func DotnetBinary(dataDir, key string) string {
	base := resolveDotnetBase(dataDir, key, filepath.Join("server", "dotnet", dotnetMajorFromKey(key)))
	bin := filepath.Join(base, "root", "dotnet")
	if fileExists(bin) {
		return bin
	}
	wrapper := filepath.Join(base, "bin", "dotnet")
	if fileExists(wrapper) {
		return wrapper
	}
	if p, err := exec.LookPath("dotnet"); err == nil {
		return p
	}
	return ""
}
