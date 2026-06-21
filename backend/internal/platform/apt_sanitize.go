package platform

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// SanitizeBrokenAptRepos fixes or removes third-party apt lists that break apt-get update
// (e.g. MongoDB noble suite before MongoDB publishes packages for Ubuntu 24.04).
func SanitizeBrokenAptRepos() {
	if runtime.GOOS != "linux" || PackageManager() != "apt" {
		return
	}
	dir := "/etc/apt/sources.list.d"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)
		orig := content
		if strings.Contains(content, "repo.mongodb.org") {
			content = strings.ReplaceAll(content, "/ubuntu noble/mongodb-org/", "/ubuntu jammy/mongodb-org/")
			content = strings.ReplaceAll(content, "/ubuntu noble/", "/ubuntu jammy/")
			content = strings.ReplaceAll(content, "/debian trixie/mongodb-org/", "/debian bookworm/mongodb-org/")
			content = strings.ReplaceAll(content, "/debian trixie/", "/debian bookworm/")
		}
		if content != orig {
			_ = os.WriteFile(path, []byte(content), 0644)
			log.Printf("[apt] fixed broken repository file: %s", e.Name())
		}
	}
	// If a MongoDB list still references an unsupported suite, remove it entirely.
	entries, _ = os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)
		if !strings.Contains(content, "repo.mongodb.org") {
			continue
		}
		if strings.Contains(content, "/ubuntu noble/") || strings.Contains(content, "/ubuntu mantic/") {
			_ = os.Remove(path)
			log.Printf("[apt] removed unsupported MongoDB repository file: %s", e.Name())
		}
	}
}

// AptGetUpdate runs apt-get update after sanitizing known-bad repository entries.
func AptGetUpdate(extraArgs ...string) error {
	if runtime.GOOS != "linux" || PackageManager() != "apt" {
		return nil
	}
	SanitizeBrokenAptRepos()
	args := append([]string{"update"}, extraArgs...)
	cmd := exec.Command("apt-get", args...)
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err == nil {
		return nil
	}
	if strings.Contains(text, "mongodb.org") || strings.Contains(text, "does not have a Release file") {
		removeMongoDBAptLists()
		SanitizeBrokenAptRepos()
		cmd = exec.Command("apt-get", args...)
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
		out, err = cmd.CombinedOutput()
		text = strings.TrimSpace(string(out))
		if err == nil {
			return nil
		}
	}
	if text != "" {
		return fmt.Errorf("%s", text)
	}
	return err
}

func removeMongoDBAptLists() {
	dir := "/etc/apt/sources.list.d"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Name()), "mongodb-org") {
			_ = os.Remove(filepath.Join(dir, e.Name()))
			log.Printf("[apt] removed broken MongoDB apt list: %s", e.Name())
		}
	}
}
