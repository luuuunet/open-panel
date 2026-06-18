package appstore

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

var runtimeInstallKeys = map[string]bool{
	"pm2": true, "composer": true, "nodejs20": true, "nodejs18": true,
}

var nodeReleaseVersions = map[string]string{
	"20": "v20.19.0",
	"18": "v18.20.8",
}

func tryRuntimeInstall(key, version, installPath, dataDir string) (bool, error) {
	if !runtimeInstallKeys[key] {
		return false, nil
	}
	switch key {
	case "pm2":
		return true, installPM2(dataDir)
	case "composer":
		return true, installComposer(dataDir)
	case "nodejs20":
		return true, installNodeJS(dataDir, "20")
	case "nodejs18":
		return true, installNodeJS(dataDir, "18")
	default:
		return false, nil
	}
}

func installPM2(dataDir string) error {
	if _, err := exec.LookPath("npm"); err == nil {
		if err := runCommand("npm", "install", "-g", "pm2"); err == nil {
			return nil
		}
	}
	return simulateInstall("pm2", "latest", filepath.Join(dataDir, "server", "pm2"), dataDir)
}

func installComposer(dataDir string) error {
	base := filepath.Join(dataDir, "server", "composer")
	_ = os.MkdirAll(base, 0755)
	dest := filepath.Join(base, "composer.phar")
	if err := downloadFile("https://getcomposer.org/download/latest-stable/composer.phar", dest); err == nil {
		if err := writeComposerWrapper(base, dest); err != nil {
			return err
		}
		marker := filepath.Join(base, ".owpanel-installed")
		return os.WriteFile(marker, []byte("composer.phar\n"), 0644)
	}
	if _, err := exec.LookPath("composer"); err == nil {
		return simulateInstall("composer", "latest", base, dataDir)
	}
	return fmt.Errorf("安装 Composer 失败：无法下载 composer.phar")
}

func writeComposerWrapper(base, pharPath string) error {
	if runtime.GOOS == "windows" {
		wrapper := filepath.Join(base, "composer.cmd")
		return os.WriteFile(wrapper, []byte(fmt.Sprintf("@php \"%s\" %%*\r\n", pharPath)), 0755)
	}
	wrapper := filepath.Join(base, "composer")
	script := fmt.Sprintf("#!/bin/bash\nexec php %q \"$@\"\n", pharPath)
	return os.WriteFile(wrapper, []byte(script), 0755)
}

// EnsureComposerWrapper creates a composer shell wrapper when only composer.phar exists.
func EnsureComposerWrapper(dataDir string) {
	base := filepath.Join(dataDir, "server", "composer")
	phar := filepath.Join(base, "composer.phar")
	wrapper := filepath.Join(base, "composer")
	if !fileExists(phar) {
		return
	}
	if fileExists(wrapper) {
		return
	}
	_ = writeComposerWrapper(base, phar)
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func detectPM2() string {
	if _, err := exec.LookPath("pm2"); err != nil {
		return "stopped"
	}
	out, err := exec.Command("pm2", "ping").CombinedOutput()
	if err == nil && strings.Contains(string(out), "pong") {
		return "running"
	}
	nodeProc := "node"
	if runtime.GOOS == "windows" {
		nodeProc = "node.exe"
	}
	if processExists(nodeProc) {
		return "running"
	}
	return "stopped"
}

func detectComposer(dataDir string) string {
	if _, err := exec.LookPath("composer"); err == nil {
		return "running"
	}
	if fileExists(filepath.Join(dataDir, "server", "composer", "composer.phar")) {
		return "running"
	}
	return "stopped"
}

func detectCertbot() string {
	if _, err := exec.LookPath("certbot"); err == nil {
		return "running"
	}
	return "stopped"
}

func CertbotInstalled(dataDir string) bool {
	if CertbotBinary() != "" {
		return true
	}
	return fileExists(filepath.Join(dataDir, "server", "certbot", ".owpanel-installed"))
}

func ComposerBinary(dataDir string) string {
	if p, err := exec.LookPath("composer"); err == nil {
		return p
	}
	phar := filepath.Join(dataDir, "server", "composer", "composer.phar")
	if fileExists(phar) {
		if php, err := exec.LookPath("php"); err == nil {
			return php + " " + phar
		}
	}
	return ""
}

func PM2Binary() string {
	p, err := exec.LookPath("pm2")
	if err != nil {
		return ""
	}
	return p
}

func CertbotBinary() string {
	p, err := exec.LookPath("certbot")
	if err != nil {
		return ""
	}
	return p
}

func installNodeJS(dataDir, major string) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("Node.js %s 请通过软件商店 winget 安装", major)
	}
	base := filepath.Join(dataDir, "server", "nodejs", major)
	nodeBin := filepath.Join(base, "bin", "node")
	if fileExists(nodeBin) && nodeBinaryMajor(nodeBin) >= atoi(major) {
		marker := filepath.Join(base, ".owpanel-installed")
		return os.WriteFile(marker, []byte(nodeBin+"\n"), 0644)
	}
	_ = os.RemoveAll(base)
	_ = os.MkdirAll(base, 0755)

	ver, ok := nodeReleaseVersions[major]
	if !ok {
		return fmt.Errorf("unsupported Node.js major version %s", major)
	}
	arch := "linux-x64"
	if runtime.GOARCH == "arm64" {
		arch = "linux-arm64"
	}
	folder := fmt.Sprintf("node-%s-%s", ver, arch)
	url := fmt.Sprintf("https://nodejs.org/dist/%s/%s.tar.xz", ver, folder)
	tmpDir, err := os.MkdirTemp("", "op-node-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	tarball := filepath.Join(tmpDir, "node.tar.xz")
	if err := downloadFile(url, tarball); err != nil {
		return fmt.Errorf("下载 Node.js %s 失败: %w", ver, err)
	}
	extractParent := filepath.Join(dataDir, "server", "nodejs")
	_ = os.MkdirAll(extractParent, 0755)
	cmd := exec.Command("tar", "-xJf", tarball, "-C", extractParent)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("解压 Node.js 失败: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	src := filepath.Join(extractParent, folder)
	if !fileExists(filepath.Join(src, "bin", "node")) {
		return fmt.Errorf("Node.js 解压结果异常")
	}
	_ = os.RemoveAll(base)
	if err := os.Rename(src, base); err != nil {
		return fmt.Errorf("安装 Node.js 失败: %w", err)
	}
	marker := filepath.Join(base, ".owpanel-installed")
	return os.WriteFile(marker, []byte(filepath.Join(base, "bin", "node")+"\n"), 0644)
}

func NodeBinDir(dataDir string, major int) string {
	dir := filepath.Join(dataDir, "server", "nodejs", strconv.Itoa(major), "bin")
	if fileExists(filepath.Join(dir, "node")) {
		return dir
	}
	return ""
}

func NodeBinary(dataDir string, major int) string {
	bin := filepath.Join(NodeBinDir(dataDir, major), "node")
	if fileExists(bin) {
		return bin
	}
	return ""
}

func NodeMajorAvailable(dataDir string, major int) bool {
	if bin := NodeBinary(dataDir, major); bin != "" {
		return nodeBinaryMajor(bin) >= major
	}
	if p, err := exec.LookPath("node"); err == nil {
		return nodeBinaryMajor(p) >= major
	}
	return false
}

func EnsureNodeMajor(dataDir string, major int) error {
	if NodeMajorAvailable(dataDir, major) {
		return nil
	}
	return installNodeJS(dataDir, strconv.Itoa(major))
}

func nodeBinaryMajor(bin string) int {
	out, err := exec.Command(bin, "-v").CombinedOutput()
	if err != nil {
		return 0
	}
	v := strings.TrimSpace(string(out))
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) == 0 {
		return 0
	}
	n, _ := strconv.Atoi(parts[0])
	return n
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func TomcatServiceName(key string) string {
	if key == "tomcat10" {
		if runtime.GOOS == "linux" && detectLinuxPkgMgr() != "apt" {
			return "tomcat"
		}
		return "tomcat10"
	}
	return "tomcat9"
}
