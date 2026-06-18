package composer

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/appstore"
)

type Result struct {
	Output string `json:"output"`
	OK     bool   `json:"ok"`
}

func Run(dataDir, workDir, command string) (*Result, error) {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return nil, fmt.Errorf("工作目录不能为空")
	}
	if _, err := os.Stat(workDir); err != nil {
		return nil, fmt.Errorf("目录不存在: %s", workDir)
	}

	bin := appstore.ComposerBinary(dataDir)
	if bin == "" {
		return nil, fmt.Errorf("Composer 未安装，请先在软件商店安装 Composer")
	}

	args := parseCommand(command)
	if len(args) == 0 {
		args = []string{"install", "--no-interaction"}
	}

	var cmd *exec.Cmd
	if strings.Contains(bin, " ") {
		parts := strings.Fields(bin)
		all := append(parts, args...)
		cmd = exec.Command(all[0], all[1:]...)
	} else {
		cmd = exec.Command(bin, args...)
	}
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "COMPOSER_ALLOW_SUPERUSER=1", "COMPOSER_NO_INTERACTION=1")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
	return &Result{Output: out, OK: err == nil}, err
}

func parseCommand(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return strings.Fields(raw)
}

func Installed(dataDir string) bool {
	return appstore.ComposerBinary(dataDir) != ""
}

func PharPath(dataDir string) string {
	return filepath.Join(dataDir, "server", "composer", "composer.phar")
}
