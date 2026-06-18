package cilium

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/appstore"
)

func kubeEnv() []string {
	env := os.Environ()
	if runtime.GOOS == "linux" {
		env = append(env, "KUBECONFIG="+appstore.K3sKubeConfig)
	}
	return env
}

func kubectl(args ...string) (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("Cilium 需要 Linux 服务器与 k3s")
	}
	if !appstore.K3sRunning() {
		return "", fmt.Errorf("k3s 未运行")
	}
	cmd := exec.Command("kubectl", args...)
	cmd.Env = kubeEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return stdout.String(), nil
}

func helmUpgradeHostFirewall(hostFirewall, hubble, hubbleUI bool) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("Cilium 仅支持 Linux")
	}
	return appstore.InstallCiliumHelm(hostFirewall, hubble, hubbleUI)
}
