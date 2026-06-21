package dockercompose

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// HasV2 reports whether `docker compose` (Compose plugin) is available.
func HasV2() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	out, err := exec.Command("docker", "compose", "version").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "compose")
}

// Argv returns the executable and args prefix for Compose v2 (docker compose) or v1 (docker-compose).
func Argv(extra ...string) (string, []string, error) {
	if HasV2() {
		return "docker", append([]string{"compose"}, extra...), nil
	}
	if path, err := exec.LookPath("docker-compose"); err == nil {
		return path, extra, nil
	}
	return "", nil, fmt.Errorf("未找到 docker compose（请安装 docker-compose-plugin 或 docker-compose）")
}

// RunInDir executes compose in dir with optional explicit compose file (-f).
func RunInDir(dir, composeFile string, args ...string) (string, error) {
	if composeFile != "" {
		args = append([]string{"-f", composeFile}, args...)
	}
	name, cmdArgs, err := Argv(args...)
	if err != nil {
		return "", err
	}
	cmd := exec.Command(name, cmdArgs...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return text, fmt.Errorf("%s", text)
	}
	return text, nil
}

// PSRunning reports whether any service is running in dir (default compose file).
func PSRunning(dir string) bool {
	name, cmdArgs, err := Argv("ps", "--status", "running", "-q")
	if err != nil {
		return false
	}
	cmd := exec.Command(name, cmdArgs...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}
