package pm2

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Manager struct {
	dataDir string
}

func NewManager(dataDir string) *Manager {
	return &Manager{dataDir: dataDir}
}

func (m *Manager) Installed() bool {
	if _, err := exec.LookPath("pm2"); err == nil {
		return true
	}
	return fileExists(filepath.Join(m.dataDir, "server", "pm2", ".owpanel-installed"))
}

type StartOptions struct {
	Name    string
	Cwd     string
	Script  string
	Port    int
	Env     map[string]string
	Shell   bool // run script as shell command via pm2 --interpreter bash/sh
}

func (m *Manager) Start(name, cwd, script string, port int) (string, error) {
	return m.StartWithOptions(StartOptions{Name: name, Cwd: cwd, Script: script, Port: port})
}

func (m *Manager) StartWithOptions(opts StartOptions) (string, error) {
	if !m.Installed() {
		return "", fmt.Errorf("PM2 未安装，请先在软件商店安装 PM2")
	}
	cwd := opts.Cwd
	if cwd == "" {
		return "", fmt.Errorf("项目路径不能为空")
	}
	script := strings.TrimSpace(opts.Script)
	if script == "" {
		for _, candidate := range []string{"app.js", "index.js", "server.js", "main.js"} {
			if fileExists(filepath.Join(cwd, candidate)) {
				script = candidate
				break
			}
		}
		if script == "" {
			script = "index.js"
		}
	}
	_ = m.Stop(opts.Name)
	env := mergeProcessEnv(opts.Env)
	if opts.Port > 0 {
		env = setEnvVar(env, "PORT", fmt.Sprintf("%d", opts.Port))
	}
	var cmd *exec.Cmd
	if opts.Shell || strings.Contains(script, " ") {
		shell := "bash"
		if runtime.GOOS == "windows" {
			shell = "cmd"
		}
		if runtime.GOOS == "windows" {
			cmd = exec.Command("pm2", "start", shell, "--name", opts.Name, "--cwd", cwd, "--", "/c", script)
		} else {
			cmd = exec.Command("pm2", "start", shell, "--name", opts.Name, "--cwd", cwd, "--", "-lc", script)
		}
	} else {
		cmd = exec.Command("pm2", "start", script, "--name", opts.Name, "--cwd", cwd)
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("PM2 启动失败: %s", strings.TrimSpace(string(out)))
	}
	_ = exec.Command("pm2", "save").Run()
	return strings.TrimSpace(string(out)), nil
}

func (m *Manager) Stop(name string) error {
	if !m.Installed() {
		return nil
	}
	_ = exec.Command("pm2", "delete", name).Run()
	return nil
}

func (m *Manager) Status(name string) string {
	if !m.Installed() {
		return "stopped"
	}
	out, err := exec.Command("pm2", "jlist").Output()
	if err != nil {
		return "stopped"
	}
	text := strings.ToLower(string(out))
	if strings.Contains(text, strings.ToLower(name)) && strings.Contains(text, "online") {
		return "running"
	}
	return "stopped"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func mergeProcessEnv(overrides map[string]string) []string {
	base := os.Environ()
	if len(overrides) == 0 {
		return base
	}
	out := make([]string, 0, len(base)+len(overrides))
	seen := map[string]bool{}
	for k, v := range overrides {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
		seen[strings.ToUpper(k)] = true
	}
	for _, entry := range base {
		key := entry
		if i := strings.Index(entry, "="); i > 0 {
			key = entry[:i]
		}
		if seen[strings.ToUpper(key)] {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func setEnvVar(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			continue
		}
		out = append(out, entry)
	}
	return append(out, prefix+value)
}

func DefaultScript(cwd string) string {
	for _, f := range []string{"app.js", "index.js", "server.js", "main.js"} {
		if fileExists(filepath.Join(cwd, f)) {
			return f
		}
	}
	if pkg, err := os.ReadFile(filepath.Join(cwd, "package.json")); err == nil {
		lower := strings.ToLower(string(pkg))
		if strings.Contains(lower, `"next"`) || strings.Contains(lower, `"start"`) {
			return "npm start"
		}
	}
	if runtime.GOOS == "windows" {
		return "index.js"
	}
	return "index.js"
}
