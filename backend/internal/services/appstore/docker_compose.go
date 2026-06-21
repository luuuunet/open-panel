package appstore

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/luuuunet/owpanel/internal/dockercompose"
)

func dockerComposeArgv(extra ...string) ([]string, error) {
	name, args, err := dockercompose.Argv(extra...)
	if err != nil {
		return nil, err
	}
	return append([]string{name}, args...), nil
}

func hasDockerComposeV2() bool {
	return dockercompose.HasV2()
}

func runDockerComposeInDir(dir string, args ...string) error {
	name, cmdArgs, err := dockercompose.Argv(args...)
	if err != nil {
		return err
	}
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
	return dockercompose.PSRunning(dir)
}
