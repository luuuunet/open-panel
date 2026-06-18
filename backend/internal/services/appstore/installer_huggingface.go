package appstore

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

const (
	hfAppKey          = "huggingface-ai"
	hfTGIContainer    = "owpanel-hf-tgi"
	hfWebUIContainer  = "owpanel-hf-webui"
	hfOllamaContainer = "owpanel-ollama"
	hfTGIImage        = "ghcr.io/huggingface/text-generation-inference:2.4.1"
	hfWebUIImage      = "ghcr.io/open-webui/open-webui:main"
	hfOllamaImage     = "ollama/ollama:latest"
	hfTGIPort         = 8095
	hfWebUIPort       = 8097
	hfOllamaPort      = 11434
)

var defaultHFModels = []string{
	"Qwen/Qwen2.5-0.5B-Instruct",
	"Qwen/Qwen2.5-1.5B-Instruct",
	"Qwen/Qwen2.5-7B-Instruct",
	"meta-llama/Llama-3.2-3B-Instruct",
	"meta-llama/Llama-3.1-8B-Instruct",
	"microsoft/Phi-3-mini-4k-instruct",
	"mistralai/Mistral-7B-Instruct-v0.3",
	"Qwen/Qwen2.5-Coder-1.5B-Instruct",
	"HuggingFaceTB/SmolLM2-360M-Instruct",
}

type HuggingFaceOptions struct {
	CatalogID          string `json:"catalog_id"`
	ModelID            string `json:"model_id"`
	HFToken            string `json:"hf_token"`
	Runtime            string `json:"runtime"` // tgi | ollama
	EnableChatUI       bool   `json:"enable_chat_ui"`
	UseGPU             bool   `json:"use_gpu"`
	AutoConfigurePanel bool   `json:"auto_configure_panel"`
}

func DefaultHFModels() []string {
	return append([]string(nil), defaultHFModels...)
}

func HFAppKey() string { return hfAppKey }

func (s *Service) InstallHuggingFaceAI(opts HuggingFaceOptions) error {
	app, err := s.Get(hfAppKey)
	if err != nil {
		return err
	}
	if app.Status == "installing" {
		return fmt.Errorf("Hugging Face AI 正在安装中")
	}
	opts = normalizeHFOpts(opts)

	s.db.Model(app).Updates(map[string]interface{}{
		"status":        "installing",
		"install_error": "",
	})

	go func() {
		if globalInstallLogs != nil {
			globalInstallLogs.Begin(hfAppKey, "latest", app.Name)
		}
		done := installLogScope(hfAppKey)
		defer done()

		installErr := s.installHuggingFaceCore(opts)
		if globalInstallLogs != nil {
			globalInstallLogs.Finish(hfAppKey, installErr)
		}
		s.finishHuggingFaceInstall(installErr, opts)
	}()
	return nil
}

func normalizeHFOpts(opts HuggingFaceOptions) HuggingFaceOptions {
	opts.Runtime = strings.ToLower(strings.TrimSpace(opts.Runtime))
	if opts.Runtime == "" {
		opts.Runtime = "tgi"
	}
	if opts.CatalogID != "" {
		if entry := resolveCatalogEntry(opts.CatalogID); entry != nil {
			if opts.Runtime == "ollama" && entry.OllamaModel != "" {
				opts.ModelID = entry.OllamaModel
			} else if entry.HFModelID != "" {
				opts.ModelID = entry.HFModelID
				opts.Runtime = "tgi"
			} else if entry.OllamaModel != "" {
				opts.ModelID = entry.OllamaModel
				opts.Runtime = "ollama"
			}
		}
	} else if strings.Contains(opts.ModelID, "/") && opts.Runtime == "" {
		opts.Runtime = "tgi"
	}
	if opts.Runtime == "ollama" && opts.ModelID == "" {
		opts.ModelID = "qwen2.5:7b"
	}
	if opts.ModelID == "" {
		opts.ModelID = defaultHFModels[0]
	}
	if opts.Runtime == "ollama" {
		opts.UseGPU = hasNVIDIA()
	}
	if !opts.EnableChatUI {
		opts.EnableChatUI = true
	}
	if !opts.AutoConfigurePanel {
		opts.AutoConfigurePanel = true
	}
	if opts.UseGPU && !hasNVIDIA() {
		opts.UseGPU = false
	}
	return opts
}

func defaultHFOpts() HuggingFaceOptions {
	return normalizeHFOpts(HuggingFaceOptions{})
}

func (s *Service) installHuggingFaceCore(opts HuggingFaceOptions) error {
	logInstallLine("开始全自动部署 AI 模型 …")
	logInstallLine(fmt.Sprintf("引擎: %s | 模型: %s", opts.Runtime, opts.ModelID))

	if err := ensureDockerEngine(s.dataDir); err != nil {
		return err
	}

	if err := ensureHFDataDirs(s.dataDir); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}

	if opts.Runtime == "ollama" {
		return s.installHFOllamaRuntime(opts)
	}
	return s.installHFTGIRuntime(opts)
}

func (s *Service) installHFTGIRuntime(opts HuggingFaceOptions) error {
	logInstallLine("拉取 Hugging Face TGI 镜像 …")
	if err := runCommand("docker", "pull", hfTGIImage); err != nil {
		return fmt.Errorf("docker pull TGI: %w", err)
	}

	dockerRemoveHF(hfTGIContainer)
	modelsDir, cacheDir := hfDataDirs(s.dataDir)
	args := []string{
		"run", "-d",
		"--name", hfTGIContainer,
		"--restart", "unless-stopped",
		"-p", fmt.Sprintf("%d:80", hfTGIPort),
		"--shm-size", "1g",
		"-v", modelsDir + ":/data",
		"-v", cacheDir + ":/root/.cache/huggingface",
	}
	if opts.UseGPU && hasNVIDIA() {
		args = append(args, "--gpus", "all")
		logInstallLine("已启用 GPU 加速")
	} else {
		logInstallLine("以 CPU 模式部署（建议使用小模型）")
	}
	if token := strings.TrimSpace(opts.HFToken); token != "" {
		args = append(args, "-e", "HF_TOKEN="+token)
	}
	args = append(args, hfTGIImage, "--model-id", opts.ModelID)
	if !opts.UseGPU || !hasNVIDIA() {
		args = append(args, "--dtype", "float32")
	}

	logInstallLine("启动 TGI 推理服务 …")
	if err := runCommand("docker", args...); err != nil {
		return fmt.Errorf("启动 TGI 失败: %w", err)
	}
	logInstallLine(fmt.Sprintf("TGI 已启动: http://127.0.0.1:%d/v1", hfTGIPort))

	apiBase := fmt.Sprintf("http://%s:%d/v1", hostDockerInternal(), hfTGIPort)
	if opts.EnableChatUI {
		s.startHFWebUI(apiBase, "hf-local")
	}

	if opts.AutoConfigurePanel {
		logInstallLine("自动配置面板 AI 助手 …")
		if err := s.configurePanelHF(opts); err != nil {
			logInstallLine("面板 AI 配置失败: " + err.Error())
		} else {
			logInstallLine("面板 AI 已指向本地 TGI 推理服务")
		}
	}

	logInstallLine("TGI 部署完成")
	return nil
}

func (s *Service) installHFOllamaRuntime(opts HuggingFaceOptions) error {
	useDockerOllama, err := s.ensureOllamaEngine(opts)
	if err != nil {
		return err
	}

	logInstallLine(fmt.Sprintf("拉取 Ollama 模型 %s …", opts.ModelID))
	if useDockerOllama {
		if err := runCommand("docker", "exec", hfOllamaContainer, "ollama", "pull", opts.ModelID); err != nil {
			return fmt.Errorf("ollama pull: %w", err)
		}
	} else if err := runCommand("ollama", "pull", opts.ModelID); err != nil {
		return fmt.Errorf("ollama pull: %w", err)
	}
	logInstallLine("模型拉取完成")

	apiBase := fmt.Sprintf("http://%s:%d/v1", hostDockerInternal(), hfOllamaPort)
	if opts.EnableChatUI {
		s.startHFWebUI(apiBase, "ollama")
	}

	if opts.AutoConfigurePanel {
		logInstallLine("自动配置面板 AI 助手 …")
		if err := s.configurePanelHF(opts); err != nil {
			logInstallLine("面板 AI 配置失败: " + err.Error())
		} else {
			logInstallLine("面板 AI 已指向本地 Ollama 服务")
		}
	}

	logInstallLine("Ollama 部署完成")
	return nil
}

func (s *Service) ensureOllamaEngine(opts HuggingFaceOptions) (useDocker bool, err error) {
	if ollamaNativeReachable() {
		logInstallLine("使用本机 Ollama 服务")
		return false, nil
	}
	if runtime.GOOS == "linux" {
		_ = runCommand("systemctl", "start", "ollama")
		if ollamaNativeReachable() {
			logInstallLine("已启动本机 Ollama 服务")
			return false, nil
		}
	}

	logInstallLine("启动 Ollama 容器 …")
	if err := runCommand("docker", "pull", hfOllamaImage); err != nil {
		return false, fmt.Errorf("docker pull ollama: %w", err)
	}
	dockerRemoveHF(hfOllamaContainer)
	ollamaModelsDir := filepath.Join(s.dataDir, "ai", "ollama", "models")
	if err := os.MkdirAll(ollamaModelsDir, 0755); err != nil {
		return false, fmt.Errorf("创建 Ollama 数据目录失败: %w", err)
	}
	args := []string{
		"run", "-d",
		"--name", hfOllamaContainer,
		"--restart", "unless-stopped",
		"-p", fmt.Sprintf("%d:11434", hfOllamaPort),
		"-v", ollamaModelsDir + ":/root/.ollama",
		"-e", "OLLAMA_HOST=0.0.0.0:11434",
	}
	if opts.UseGPU && hasNVIDIA() {
		args = append(args, "--gpus", "all")
		logInstallLine("Ollama 已启用 GPU 加速")
	}
	args = append(args, hfOllamaImage)
	if err := runCommand("docker", args...); err != nil {
		return false, fmt.Errorf("启动 Ollama 容器失败: %w", err)
	}
	logInstallLine(fmt.Sprintf("Ollama 容器已启动: http://127.0.0.1:%d", hfOllamaPort))
	return true, nil
}

func ollamaNativeReachable() bool {
	if _, err := exec.LookPath("ollama"); err != nil {
		return false
	}
	_, err := exec.Command("ollama", "list").Output()
	return err == nil
}

func ollamaEngineRunning() bool {
	if ollamaNativeReachable() {
		return true
	}
	return dockerRunningHF(hfOllamaContainer)
}

func (s *Service) startHFWebUI(apiBase, apiKey string) {
	logInstallLine("拉取 Web 对话界面镜像 …")
	if err := runCommand("docker", "pull", hfWebUIImage); err != nil {
		logInstallLine("WebUI 镜像拉取失败，跳过: " + err.Error())
		return
	}
	dockerRemoveHF(hfWebUIContainer)
	webArgs := []string{
		"run", "-d",
		"--name", hfWebUIContainer,
		"--restart", "unless-stopped",
		"-p", fmt.Sprintf("%d:8080", hfWebUIPort),
		"-e", "OPENAI_API_BASE_URL=" + apiBase,
		"-e", "OPENAI_API_KEY=" + apiKey,
		"-e", "WEBUI_AUTH=false",
		hfWebUIImage,
	}
	if err := runCommand("docker", webArgs...); err != nil {
		logInstallLine("WebUI 启动失败: " + err.Error())
		return
	}
	logInstallLine(fmt.Sprintf("对话界面: http://127.0.0.1:%d", hfWebUIPort))
}

func (s *Service) finishHuggingFaceInstall(installErr error, opts HuggingFaceOptions) {
	app, err := s.Get(hfAppKey)
	if err != nil {
		return
	}
	updates := map[string]interface{}{}
	if installErr != nil {
		updates["status"] = "failed"
		updates["installed"] = false
		updates["install_error"] = installErr.Error()
	} else {
		updates["status"] = "running"
		updates["installed"] = true
		updates["install_error"] = ""
		if opts.Runtime == "ollama" {
			updates["port"] = hfOllamaPort
		} else {
			updates["port"] = hfTGIPort
		}
	}
	s.db.Model(app).Updates(updates)
	s.InvalidateLiveStatus(hfAppKey)
}

func (s *Service) configurePanelHF(opts HuggingFaceOptions) error {
	modelShort := shortHFModelName(opts.ModelID)
	provider := "huggingface"
	baseURL := fmt.Sprintf("http://127.0.0.1:%d/v1", hfTGIPort)
	runtime := opts.Runtime
	if runtime == "" {
		runtime = "tgi"
	}
	if runtime == "ollama" {
		provider = "ollama"
		baseURL = fmt.Sprintf("http://127.0.0.1:%d/v1", hfOllamaPort)
		modelShort = opts.ModelID
	}
	pairs := map[string]string{
		"ai_enabled":     "true",
		"ai_provider":    provider,
		"ai_base_url":    baseURL,
		"ai_model":       modelShort,
		"hf_ai_model":    opts.ModelID,
		"hf_ai_runtime":  runtime,
	}
	for k, v := range pairs {
		var row models.PanelSetting
		err := s.db.Where("key = ?", k).First(&row).Error
		if err != nil {
			if err := s.db.Create(&models.PanelSetting{Key: k, Value: v}).Error; err != nil {
				return err
			}
		} else if err := s.db.Model(&row).Update("value", v).Error; err != nil {
			return err
		}
	}
	return nil
}

func tryHuggingFaceInstall(key, _, _, _ string) (bool, error) {
	if key != hfAppKey {
		return false, nil
	}
	if installService == nil {
		return true, fmt.Errorf("appstore service not initialized")
	}
	return true, installService.installHuggingFaceCore(defaultHFOpts())
}

func tryHuggingFaceUninstall(key, dataDir string) (bool, error) {
	if key != hfAppKey {
		return false, nil
	}
	dockerRemoveHF(hfTGIContainer)
	dockerRemoveHF(hfWebUIContainer)
	dockerRemoveHF(hfOllamaContainer)
	return true, removeSimulatedInstall(key, dataDir)
}

func tryHuggingFaceServiceAction(key, action string) (bool, error) {
	if key != hfAppKey {
		return false, nil
	}
	switch action {
	case "start":
		_ = runCommand("docker", "start", hfTGIContainer)
		_ = runCommand("docker", "start", hfOllamaContainer)
		_ = runCommand("docker", "start", hfWebUIContainer)
		if runtime.GOOS == "linux" {
			_ = runCommand("systemctl", "start", "ollama")
		}
	case "stop":
		_ = runCommand("docker", "stop", hfTGIContainer)
		_ = runCommand("docker", "stop", hfOllamaContainer)
		_ = runCommand("docker", "stop", hfWebUIContainer)
		if runtime.GOOS == "linux" {
			_ = runCommand("systemctl", "stop", "ollama")
		}
	case "restart", "reload":
		_ = runCommand("docker", "restart", hfTGIContainer)
		_ = runCommand("docker", "restart", hfOllamaContainer)
		_ = runCommand("docker", "restart", hfWebUIContainer)
		if runtime.GOOS == "linux" {
			_ = runCommand("systemctl", "restart", "ollama")
		}
	}
	return true, nil
}

func tryHuggingFaceStatus(key string) (bool, string) {
	if key != hfAppKey {
		return false, ""
	}
	if dockerRunningHF(hfTGIContainer) || ollamaEngineRunning() {
		return true, "running"
	}
	return true, "stopped"
}

func ensureHFDataDirs(dataDir string) error {
	base := filepath.Join(dataDir, "ai", "huggingface")
	if err := os.MkdirAll(filepath.Join(base, "models"), 0755); err != nil {
		return err
	}
	return os.MkdirAll(filepath.Join(base, "cache"), 0755)
}

func hfDataDirs(dataDir string) (string, string) {
	base := filepath.Join(dataDir, "ai", "huggingface")
	return filepath.Join(base, "models"), filepath.Join(base, "cache")
}

func dockerRemoveHF(name string) {
	_ = runCommand("docker", "stop", name)
	_ = runCommand("docker", "rm", "-f", name)
}

func dockerRunningHF(name string) bool {
	out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", name).Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

func hasNVIDIA() bool {
	_, err := exec.LookPath("nvidia-smi")
	return err == nil
}

func hostDockerInternal() string {
	if runtime.GOOS == "linux" {
		return "172.17.0.1"
	}
	return "host.docker.internal"
}

func shortHFModelName(modelID string) string {
	parts := strings.Split(modelID, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return modelID
}
