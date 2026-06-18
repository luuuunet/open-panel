package appstore

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func javaPackagePresent(key, dataDir string) bool {
	if fileExists(filepath.Join(dataDir, "server", key, ".owpanel-installed")) {
		return true
	}
	if javaBinaryForKey(key) != "" {
		return true
	}
	if hostJavaProcessMatchesVersion(strings.TrimPrefix(key, "java")) {
		return true
	}
	spec, ok := resolvePackageSpec(key)
	if !ok {
		return false
	}
	return linuxPackageInstalled(spec)
}

func javaBinaryForKey(key string) string {
	ver := strings.TrimPrefix(key, "java")
	for _, bin := range enumerateJavaBinaries() {
		if javaVersionMatch(runJavaVersion(bin), ver) {
			return bin
		}
	}
	return ""
}

func enumerateJavaBinaries() []string {
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
	for _, p := range []string{"/usr/bin/java", "/usr/local/bin/java"} {
		bins = append(bins, add(p)...)
	}
	if p, err := exec.LookPath("java"); err == nil {
		bins = append(bins, add(p)...)
	}
	for _, pattern := range []string{"/opt/*/bin/java", "/usr/lib/jvm/*/bin/java", "/usr/local/jdk*/bin/java"} {
		for _, p := range globPaths(pattern) {
			bins = append(bins, add(p)...)
		}
	}
	return bins
}

func globPaths(pattern string) []string {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	return matches
}

func runJavaVersion(bin string) string {
	out, err := exec.Command(bin, "-version").CombinedOutput()
	if err != nil {
		return ""
	}
	return string(out)
}

func hostJavaProcessMatchesVersion(ver string) bool {
	if runtime.GOOS != "linux" {
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
		if !strings.Contains(strings.ToLower(cmd), "java") {
			continue
		}
		for _, part := range strings.Fields(cmd) {
			base := filepath.Base(part)
			if base != "java" {
				continue
			}
			if javaVersionMatch(runJavaVersion(part), ver) {
				return true
			}
		}
	}
	return false
}

func linuxPackageInstalled(spec packageSpec) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	mgr := detectLinuxPkgMgr()
	switch mgr {
	case "apt":
		for _, pkg := range spec.Apt {
			out, err := exec.Command("dpkg-query", "-W", "-f=${Status}", pkg).Output()
			if err == nil && strings.Contains(string(out), "install ok installed") {
				return true
			}
		}
	case "dnf", "yum":
		pkgs := spec.Dnf
		if len(pkgs) == 0 {
			pkgs = spec.Apt
		}
		for _, pkg := range pkgs {
			if err := exec.Command("rpm", "-q", pkg).Run(); err == nil {
				return true
			}
		}
	}
	return false
}
