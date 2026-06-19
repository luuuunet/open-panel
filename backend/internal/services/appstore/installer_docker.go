package appstore

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/secrets"
)

func tryDockerInstall(key, version, installPath, dataDir string) (bool, error) {
	if ok, err := tryOpenpanelInstall(key, version, installPath, dataDir); ok {
		return true, err
	}
	if key == "docker" {
		_ = version
		_ = installPath
		return true, installDockerEnginePackage()
	}
	if key == "kafka" {
		return false, nil
	}
	spec, ok := dockerSpec(key)
	if !ok {
		return false, nil
	}
	_ = version
	_ = installPath
	return true, installDockerApp(key, spec, dataDir)
}

func tryDockerUninstall(key, dataDir string) (bool, error) {
	if ok, err := tryOpenpanelUninstall(key, dataDir); ok {
		return true, err
	}
	spec, ok := dockerSpec(key)
	if !ok {
		return false, nil
	}
	_ = dockerRemove(spec.Container)
	return true, removeSimulatedInstall(key, dataDir)
}

func tryDockerServiceAction(key, action string) (bool, error) {
	spec, ok := dockerSpec(key)
	if !ok {
		return false, nil
	}
	return true, dockerServiceAction(spec.Container, action)
}

func tryDockerStatus(key string) (bool, string) {
	if ok, status := tryOpenpanelStatus(key); ok {
		return true, status
	}
	spec, ok := dockerSpec(key)
	if !ok {
		return false, ""
	}
	if !dockerContainerExists(spec.Container) {
		return false, ""
	}
	if dockerRunning(spec.Container) {
		return true, "running"
	}
	return true, "stopped"
}

func dockerContainerExists(name string) bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	return exec.Command("docker", "inspect", name).Run() == nil
}

func dockerEngineReady() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	return exec.Command("docker", "info").Run() == nil
}

// ensureDockerEngine installs and starts Docker when missing (Kafka and other Docker apps).
func ensureDockerEngine(dataDir string) error {
	_ = dataDir
	if dockerEngineReady() {
		return nil
	}
	logInstallLine("Docker 未安装或未运行，正在自动安装 Docker …")
	if err := installDockerEnginePackage(); err != nil {
		return fmt.Errorf("docker 未安装且自动安装失败: %w", err)
	}
	if !dockerEngineReady() {
		return fmt.Errorf("docker 安装后仍不可用，请检查 systemctl status docker")
	}
	logInstallLine("Docker 引擎已就绪")
	return nil
}

func installDockerEnginePackage() error {
	switch runtime.GOOS {
	case "linux":
		if err := installDockerEngineLinux(); err != nil {
			return err
		}
		spec, ok := packageSpecs["docker"]
		if !ok {
			return fmt.Errorf("缺少 Docker 安装规格")
		}
		if svc := serviceName(spec); svc != "" {
			_ = runCommand("systemctl", "enable", svc)
			if err := runCommand("systemctl", "start", svc); err != nil {
				return fmt.Errorf("启动 Docker 服务: %w", err)
			}
		}
		return nil
	case "windows":
		spec, ok := packageSpecs["docker"]
		if !ok {
			return fmt.Errorf("缺少 Docker 安装规格")
		}
		if err := installWindowsPackages(spec); err != nil {
			return fmt.Errorf("安装 Docker Desktop: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("当前系统不支持自动安装 Docker")
	}
}

func installDockerEngineLinux() error {
	mgr := detectLinuxPkgMgr()
	switch mgr {
	case "apt":
		if err := runCommand("apt-get", "update", "-qq"); err != nil {
			return fmt.Errorf("apt update: %w", err)
		}
		if err := runCommand("apt-get", "install", "-y", "docker.io"); err != nil {
			logInstallLine("apt 安装 docker.io 失败，尝试 stack 脚本 …")
			if err2 := runStackFallback("docker"); err2 != nil {
				logInstallLine("stack 脚本失败，尝试 Docker 官方脚本 …")
				if err3 := installDockerViaOfficialScript(); err3 != nil {
					return fmt.Errorf("apt install docker.io: %w; stack: %v; 官方脚本: %v", err, err2, err3)
				}
			}
		}
		installDockerComposeOptional(mgr)
		return nil
	case "dnf", "yum":
		if mgr == "dnf" {
			if err := runCommand("dnf", "install", "-y", "docker"); err != nil {
				return fmt.Errorf("dnf install docker: %w", err)
			}
		} else {
			if err := runCommand("yum", "install", "-y", "docker"); err != nil {
				return fmt.Errorf("yum install docker: %w", err)
			}
		}
		installDockerComposeOptional(mgr)
		return nil
	default:
		return fmt.Errorf("unsupported linux package manager (need apt/dnf/yum)")
	}
}

func installDockerComposeOptional(mgr string) {
	switch mgr {
	case "apt":
		if err := runCommand("apt-get", "install", "-y", "docker-compose-plugin"); err == nil {
			return
		}
		if err := runCommand("apt-get", "install", "-y", "docker-compose"); err == nil {
			return
		}
	case "dnf", "yum":
		if err := runCommand(mgr, "install", "-y", "docker-compose-plugin"); err == nil {
			return
		}
		if err := runCommand(mgr, "install", "-y", "docker-compose"); err == nil {
			return
		}
	}
	logInstallLine("提示: docker compose 插件未安装（可选）。Docker 引擎已可用，可稍后在软件商店重试或使用 get.docker.com 脚本。")
}

func installDockerViaOfficialScript() error {
	if _, err := exec.LookPath("curl"); err != nil {
		return fmt.Errorf("curl 不可用")
	}
	logInstallLine("$ curl -fsSL https://get.docker.com | sh")
	cmd := exec.Command("sh", "-c", "curl -fsSL https://get.docker.com | sh")
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if text != "" {
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				logInstallLine(line)
			}
		}
	}
	if err != nil {
		if text != "" {
			return fmt.Errorf("%v: %s", err, text)
		}
		return err
	}
	return nil
}

func installDockerApp(key string, spec dockerAppSpec, dataDir string) error {
	if err := ensureDockerEngine(dataDir); err != nil {
		return err
	}
	_ = dockerRemove(spec.Container)
	if err := runCommand("docker", "pull", spec.Image); err != nil {
		return fmt.Errorf("docker pull %s: %w", spec.Image, err)
	}
	envVars := materializeDockerEnv(key, dataDir, spec.Env)
	args := []string{"run", "-d", "--name", spec.Container, "--restart", "unless-stopped"}
	if spec.Port != "" {
		args = append(args, "-p", spec.Port)
	}
	for _, e := range envVars {
		args = append(args, "-e", e)
	}
	for _, v := range spec.Volumes {
		args = append(args, "-v", v)
	}
	args = append(args, spec.Image)
	if err := runCommand("docker", args...); err != nil {
		return fmt.Errorf("docker run %s: %w", spec.Container, err)
	}
	return nil
}

func materializeDockerEnv(key, dataDir string, env []string) []string {
	out := make([]string, len(env))
	var credLines []string
	for i, e := range env {
		if strings.Contains(e, "openpanel123") {
			pass, err := secrets.GeneratePassword(20)
			if err != nil {
				pass, _ = secrets.GeneratePassword(16)
			}
			e = strings.ReplaceAll(e, "openpanel123", pass)
			credLines = append(credLines, e)
		}
		out[i] = e
	}
	if len(credLines) > 0 {
		dir := filepath.Join(dataDir, "docker-secrets")
		_ = os.MkdirAll(dir, 0700)
		path := filepath.Join(dir, key+".env")
		body := strings.Join(credLines, "\n") + "\n"
		if err := os.WriteFile(path, []byte(body), 0600); err == nil {
			logInstallLine(fmt.Sprintf("Docker 凭据已写入 %s", path))
		}
	}
	return out
}
