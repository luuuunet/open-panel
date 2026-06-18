package appstore

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func nodeMajorFromKey(key string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(key, "nodejs"))
	return n
}

func nodePackagePresent(key, dataDir string) bool {
	major := nodeMajorFromKey(key)
	if major == 0 {
		return false
	}
	base := filepath.Join(dataDir, "server", "nodejs", strconv.Itoa(major))
	if fileExists(filepath.Join(base, ".owpanel-installed")) {
		return true
	}
	if fileExists(filepath.Join(base, "bin", "node")) {
		return true
	}
	return nodeBinaryMajorMatches(key)
}

func nodeBinaryMajorMatches(key string) bool {
	major := nodeMajorFromKey(key)
	if major == 0 {
		return false
	}
	for _, bin := range enumerateNodeBinaries() {
		if nodeBinaryMajor(bin) == major {
			return true
		}
	}
	return false
}

func enumerateNodeBinaries() []string {
	seen := map[string]struct{}{}
	add := func(p string) []string {
		if p == "" {
			return nil
		}
		if _, ok := seen[p]; ok {
			return nil
		}
		if !fileExists(p) {
			return nil
		}
		seen[p] = struct{}{}
		return []string{p}
	}
	var bins []string
	for _, p := range []string{"/usr/bin/node", "/usr/local/bin/node"} {
		bins = append(bins, add(p)...)
	}
	if p, err := exec.LookPath("node"); err == nil {
		bins = append(bins, add(p)...)
	}
	for _, pattern := range []string{"/opt/*/bin/node", "/usr/local/n/versions/node/*/bin/node"} {
		for _, p := range globPaths(pattern) {
			bins = append(bins, add(p)...)
		}
	}
	return bins
}

func hostNodeProcessMatchesVersion(majorVer string) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	want, err := strconv.Atoi(majorVer)
	if err != nil || want == 0 {
		return false
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid := e.Name()
		if _, err := strconv.Atoi(pid); err != nil {
			continue
		}
		cgroup, err := os.ReadFile(filepath.Join("/proc", pid, "cgroup"))
		if err != nil {
			continue
		}
		if strings.Contains(string(cgroup), "docker") {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", pid, "cmdline"))
		if err != nil {
			continue
		}
		cmd := strings.ReplaceAll(string(cmdline), "\x00", " ")
		if !strings.Contains(strings.ToLower(cmd), "node") {
			continue
		}
		for _, part := range strings.Fields(cmd) {
			base := filepath.Base(part)
			if base != "node" {
				continue
			}
			if nodeBinaryMajor(part) == want {
				return true
			}
		}
	}
	return false
}

func detectNodeStatusForInstall(key, dataDir string) string {
	major := nodeMajorFromKey(key)
	if major == 0 {
		return "stopped"
	}
	base := filepath.Join(dataDir, "server", "nodejs", strconv.Itoa(major))
	if fileExists(filepath.Join(base, ".owpanel-installed")) {
		return "running"
	}
	if !nodeBinaryMajorMatches(key) {
		return "stopped"
	}
	if hostNodeProcessMatchesVersion(strconv.Itoa(major)) {
		return "running"
	}
	return "stopped"
}
