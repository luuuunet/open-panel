package appstore

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// AI 软件容器名与镜像配置
type aiDockerSpec struct {
	Container string
	Image     string
	Port      string // host:container
	Env       []string
}

var aiDockerApps = map[string]aiDockerSpec{
	"open-webui": {
		Container: "owpanel-open-webui",
		Image:     "ghcr.io/open-webui/open-webui:main",
		Port:      "8080:8080",
		Env:       []string{"OLLAMA_BASE_URL=http://host.docker.internal:11434"},
	},
	"localai": {
		Container: "owpanel-localai",
		Image:     "localai/localai:latest",
		Port:      "8090:8080",
	},
	"dify": {
		Container: "owpanel-dify",
		Image:     "langgenius/dify-web:latest",
		Port:      "8091:3000",
	},
	"anythingllm": {
		Container: "owpanel-anythingllm",
		Image:     "mintplexlabs/anythingllm:latest",
		Port:      "3001:3001",
	},
	"fastgpt": {
		Container: "owpanel-fastgpt",
		Image:     "ghcr.io/labring/fastgpt:latest",
		Port:      "3002:3000",
	},
	"comfyui": {
		Container: "owpanel-comfyui",
		Image:     "yanwk/comfyui-boot:cu124-slim",
		Port:      "8188:8188",
	},
	"sd-webui": {
		Container: "owpanel-sd-webui",
		Image:     "continuumio/miniconda3:latest",
		Port:      "7860:7860",
	},
}

var aiInstallers = map[string]func(string, string, string) error{
	"ollama":      installOllama,
	"jupyter":     installJupyter,
	"vllm":        installVLLM,
	"whisper":     installWhisper,
	"chatchat":    installChatChat,
	"open-webui":  aiDocker("open-webui"),
	"localai":     aiDocker("localai"),
	"dify":        aiDocker("dify"),
	"anythingllm": aiDocker("anythingllm"),
	"fastgpt":     aiDocker("fastgpt"),
	"comfyui":     aiDocker("comfyui"),
	"sd-webui":    aiDocker("sd-webui"),
}

func aiDocker(key string) func(string, string, string) error {
	return func(_, installPath, dataDir string) error {
		return installAIDocker(key, installPath, dataDir)
	}
}

func tryAIInstall(key, version, installPath, dataDir string) (bool, error) {
	if ok, err := tryHuggingFaceInstall(key, version, installPath, dataDir); ok {
		return true, err
	}
	fn, ok := aiInstallers[key]
	if !ok {
		return false, nil
	}
	return true, fn(version, installPath, dataDir)
}

func tryAIUninstall(key, dataDir string) (bool, error) {
	if ok, err := tryHuggingFaceUninstall(key, dataDir); ok {
		return true, err
	}
	if spec, ok := aiDockerApps[key]; ok {
		_ = dockerRemove(spec.Container)
		return true, removeSimulatedInstall(key, dataDir)
	}
	switch key {
	case "ollama":
		if runtime.GOOS == "linux" {
			_ = runCommand("systemctl", "stop", "ollama")
			_ = runCommand("systemctl", "disable", "ollama")
		}
		return true, removeSimulatedInstall(key, dataDir)
	case "jupyter", "vllm", "whisper", "chatchat":
		return true, removeSimulatedInstall(key, dataDir)
	}
	return false, nil
}

func tryAIServiceAction(key, action string) (bool, error) {
	if ok, err := tryHuggingFaceServiceAction(key, action); ok {
		return true, err
	}
	if spec, ok := aiDockerApps[key]; ok {
		return true, dockerServiceAction(spec.Container, action)
	}
	switch key {
	case "ollama":
		if runtime.GOOS != "linux" {
			return true, nil
		}
		switch action {
		case "start":
			return true, runCommand("systemctl", "start", "ollama")
		case "stop":
			return true, runCommand("systemctl", "stop", "ollama")
		case "restart", "reload":
			return true, runCommand("systemctl", "restart", "ollama")
		}
	}
	return false, nil
}

func tryAIStatus(key string) (bool, string) {
	if ok, status := tryHuggingFaceStatus(key); ok {
		return true, status
	}
	if spec, ok := aiDockerApps[key]; ok {
		if dockerRunning(spec.Container) {
			return true, "running"
		}
		return true, "stopped"
	}
	if key == "ollama" && runtime.GOOS == "linux" {
		out, err := exec.Command("systemctl", "is-active", "ollama").Output()
		if err == nil && strings.TrimSpace(string(out)) == "active" {
			return true, "running"
		}
		return true, "stopped"
	}
	return false, ""
}

func installOllama(_, _, dataDir string) error {
	switch runtime.GOOS {
	case "linux":
		if err := runCommand("sh", "-c", "curl -fsSL https://ollama.com/install.sh | sh"); err != nil {
			return fmt.Errorf("ollama install: %w", err)
		}
		_ = runCommand("systemctl", "enable", "ollama")
		return runCommand("systemctl", "start", "ollama")
	case "windows":
		if err := runCommand("winget", "install", "-e", "--id", "Ollama.Ollama", "--accept-package-agreements", "--accept-source-agreements"); err != nil {
			return simulateInstall("ollama", "latest", "", dataDir)
		}
		return nil
	default:
		return simulateInstall("ollama", "latest", "", dataDir)
	}
}

func installJupyter(_, installPath, dataDir string) error {
	if runtime.GOOS == "linux" {
		if err := runCommand("pip3", "install", "--break-system-packages", "jupyterlab"); err != nil {
			if err2 := runCommand("pip3", "install", "jupyterlab"); err2 != nil {
				if err3 := installLinuxPackages(packageSpec{Apt: []string{"jupyter-notebook"}}); err3 != nil {
					return simulateInstall("jupyter", "4", installPath, dataDir)
				}
			}
		}
		return nil
	}
	return simulateInstall("jupyter", "4", installPath, dataDir)
}

func installVLLM(_, installPath, dataDir string) error {
	if runtime.GOOS == "linux" {
		if err := runCommand("pip3", "install", "vllm"); err != nil {
			return simulateInstall("vllm", "0.6", installPath, dataDir)
		}
		return nil
	}
	return simulateInstall("vllm", "0.6", installPath, dataDir)
}

func installWhisper(_, installPath, dataDir string) error {
	if runtime.GOOS == "linux" {
		if err := runCommand("pip3", "install", "openai-whisper"); err != nil {
			return simulateInstall("whisper", "2024", installPath, dataDir)
		}
		return nil
	}
	return simulateInstall("whisper", "2024", installPath, dataDir)
}

func installChatChat(_, installPath, dataDir string) error {
	// Langchain-Chatchat 需 GPU 环境，先标记安装目录并提示 Docker 部署
	return simulateInstall("chatchat", "0.3", installPath, dataDir)
}

func installAIDocker(key, _, dataDir string) error {
	spec, ok := aiDockerApps[key]
	if !ok {
		return fmt.Errorf("unknown ai docker app: %s", key)
	}
	if err := ensureDockerEngine(dataDir); err != nil {
		return err
	}
	_ = dockerRemove(spec.Container)
	if err := runCommand("docker", "pull", spec.Image); err != nil {
		return fmt.Errorf("docker pull %s: %w", spec.Image, err)
	}
	args := []string{"run", "-d", "--name", spec.Container, "--restart", "unless-stopped", "-p", spec.Port}
	for _, e := range spec.Env {
		args = append(args, "-e", e)
	}
	args = append(args, spec.Image)
	if err := runCommand("docker", args...); err != nil {
		return simulateInstall(key, "latest", "", dataDir)
	}
	return nil
}

func dockerRemove(name string) error {
	_ = runCommand("docker", "stop", name)
	_ = runCommand("docker", "rm", "-f", name)
	return nil
}

func dockerServiceAction(name, action string) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return nil
	}
	switch action {
	case "start":
		return runCommand("docker", "start", name)
	case "stop":
		return runCommand("docker", "stop", name)
	case "restart", "reload":
		_ = runCommand("docker", "stop", name)
		return runCommand("docker", "start", name)
	}
	return nil
}

func dockerRunning(name string) bool {
	out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", name).Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}
