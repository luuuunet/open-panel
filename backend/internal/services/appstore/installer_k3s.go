package appstore

import (
	"fmt"
	"os"
	"runtime"
)

const K3sKubeConfig = "/etc/rancher/k3s/k3s.yaml"

func tryK3sInstall(key, version, installPath, dataDir string) (bool, error) {
	if key != "k3s" {
		return false, nil
	}
	_ = version
	_ = installPath
	_ = dataDir
	if runtime.GOOS != "linux" {
		return true, fmt.Errorf("k3s 仅支持 Linux 服务器")
	}
	if K3sRunning() {
		logInstallLine("k3s 已在运行")
		return true, nil
	}
	logInstallLine("正在安装 k3s（轻量 Kubernetes）…")
	script := `curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--write-kubeconfig-mode 644" sh -`
	if err := runCommand("bash", "-c", script); err != nil {
		return true, fmt.Errorf("k3s 安装失败: %w", err)
	}
	logInstallLine("k3s 安装完成")
	return true, nil
}

func K3sRunning() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if _, err := os.Stat(K3sKubeConfig); err != nil {
		return false
	}
	return runCommand("systemctl", "is-active", "--quiet", "k3s") == nil
}

func tryK3sServiceAction(key, action string) (bool, error) {
	if key != "k3s" {
		return false, nil
	}
	if runtime.GOOS != "linux" {
		return true, fmt.Errorf("k3s 仅支持 Linux 服务器")
	}
	switch action {
	case "start", "stop", "restart":
		return true, runCommand("systemctl", action, "k3s")
	}
	return true, nil
}

func tryK3sStatus(key string) (bool, string) {
	if key != "k3s" {
		return false, ""
	}
	if K3sRunning() {
		return true, "running"
	}
	if _, err := os.Stat(K3sKubeConfig); err == nil {
		return true, "stopped"
	}
	return true, "stopped"
}

func RunK3sInstall(dataDir string) error {
	ok, err := tryK3sInstall("k3s", "latest", "", dataDir)
	if !ok {
		return fmt.Errorf("k3s installer unavailable")
	}
	return err
}
