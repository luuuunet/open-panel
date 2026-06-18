package aisite

import (
	"os/exec"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/appstore"
)

func runShell(script, cwd string) (string, error) {
	return runShellEnv(script, cwd, shellEnvWithGit())
}

func runDeployShell(dataDir, script, cwd string) (string, error) {
	return runShellEnv(script, cwd, shellEnvForDeploy(dataDir))
}

func shellCommand(script string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/C", script)
	}
	// Deploy scripts use bash features (pipefail, arrays); /bin/sh is often dash on Debian/Ubuntu.
	return exec.Command("bash", "-c", script)
}

func runShellEnv(script, cwd string, env []string) (string, error) {
	if cwd == "" {
		cwd = "."
	}
	cmd := shellCommand(script)
	cmd.Dir = cwd
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func shellEnvForDeploy(dataDir string) []string {
	appstore.EnsureComposerWrapper(dataDir)
	_ = appstore.EnsureNodeMajor(dataDir, 20)
	prepend := deployToolDirs(dataDir)
	return prependPath(shellEnvWithGit(), prepend...)
}

func deployToolDirs(dataDir string) []string {
	var dirs []string
	addDir := func(p string) {
		if p == "" {
			return
		}
		d := filepath.Dir(p)
		if d != "" && d != "." {
			dirs = append(dirs, d)
		}
	}
	for _, major := range []int{20, 18} {
		if dir := appstore.NodeBinDir(dataDir, major); dir != "" {
			dirs = append(dirs, dir)
		}
	}
	composerDir := filepath.Join(dataDir, "server", "composer")
	if _, err := os.Stat(filepath.Join(composerDir, "composer.phar")); err == nil {
		dirs = append(dirs, composerDir)
	}
	if bin := appstore.ComposerBinary(dataDir); bin != "" {
		if strings.Contains(bin, " ") {
			parts := strings.Fields(bin)
			if len(parts) >= 2 {
				addDir(parts[0])
				addDir(parts[1])
			}
		} else {
			addDir(bin)
		}
	}
	for _, name := range []string{"php"} {
		if p, err := exec.LookPath(name); err == nil {
			addDir(p)
		}
	}
	return dedupeStrings(dirs)
}

func dedupeStrings(list []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(list))
	for _, s := range list {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
