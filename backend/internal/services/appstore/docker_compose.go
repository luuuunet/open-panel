package appstore

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// dockerComposeArgv returns the command prefix for Compose v2 (docker compose) or v1 (docker-compose).
func dockerComposeArgv(extra ...string) ([]string, error) {
	if hasDockerComposeV2() {
		return append([]string{"docker", "compose"}, extra...), nil
	}
	if path, err := exec.LookPath("docker-compose"); err == nil {
		return append([]string{path}, extra...), nil
	}
	return nil, fmt.Errorf("未找到 docker compose（请安装 docker-compose-plugin 或 docker-compose）")
}

func hasDockerComposeV2() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	out, err := exec.Command("docker", "compose", "version").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "compose")
}

func runDockerComposeInDir(dir string, args ...string) error {
	argv, err := dockerComposeArgv(args...)
	if err != nil {
		return err
	}
	name := argv[0]
	cmdArgs := argv[1:]
	cmdLine := fmt.Sprintf("$ (cd %s) %s %s", dir, name, strings.Join(cmdArgs, " "))
	logInstallLine(cmdLine)
	cmd := exec.Command(name, cmdArgs...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
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

func dockerComposePSRunning(dir string) bool {
	argv, err := dockerComposeArgv("ps", "--status", "running", "-q")
	if err != nil {
		return false
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}
