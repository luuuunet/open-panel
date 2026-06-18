package aihub

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/appstore"
	"github.com/luuuunet/owpanel/internal/services/modelcatalog"
)

type HuggingFaceOptions = appstore.HuggingFaceOptions

type HuggingFaceStatus struct {
	Installed       bool   `json:"installed"`
	Status          string `json:"status"`
	Runtime         string `json:"runtime"`
	ModelID         string `json:"model_id"`
	TGIPort         int    `json:"tgi_port"`
	WebUIPort       int    `json:"webui_port"`
	TGIRunning      bool   `json:"tgi_running"`
	WebUIRunning    bool   `json:"webui_running"`
	OllamaRunning     bool   `json:"ollama_running"`
	PublicIP          string `json:"public_ip"`
	APIBaseURLLocal   string `json:"api_base_url_local"`
	APIBaseURLPublic  string `json:"api_base_url_public"`
	ChatURLLocal      string `json:"chat_url_local"`
	ChatURLPublic     string `json:"chat_url_public"`
	APIBaseURL        string `json:"api_base_url"`
	ChatURL           string `json:"chat_url"`
	APIKey            string `json:"api_key"`
	APISampleCurl     string `json:"api_sample_curl"`
	APISampleCurlLocal string `json:"api_sample_curl_local"`
	OpenAICompat    bool   `json:"openai_compat"`
	PanelConfigured   bool   `json:"panel_configured"`
	GPUAvailable      bool   `json:"gpu_available"`
	InstallStatus     string `json:"install_status"`
	HFTokenConfigured bool   `json:"hf_token_configured"`
	HubURL            string `json:"hub_url"`
}

type GPUInfo struct {
	Available bool     `json:"available"`
	Driver    string   `json:"driver"`
	Devices   []string `json:"devices"`
	Message   string   `json:"message"`
}

const hfTGIPort = 8095
const hfWebUIPort = 8097

func (s *Service) DefaultModels() []string {
	return DefaultModelIDs()
}

func (s *Service) ModelCatalog() []ModelCatalogEntry {
	return ModelCatalog()
}

func (s *Service) CatalogByModality(modality string) []ModelCatalogEntry {
	return CatalogByModality(modality)
}

func (s *Service) HubTasks() []modelcatalog.HubTask {
	return HubTasks()
}

func (s *Service) GPUInfo() GPUInfo {
	info := GPUInfo{Available: false, Message: "未检测到 NVIDIA GPU 或 nvidia-smi 不可用"}
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return info
	}
	out, err := exec.Command("nvidia-smi", "--query-gpu=name,driver_version", "--format=csv,noheader").Output()
	if err != nil {
		info.Message = strings.TrimSpace(err.Error())
		return info
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			info.Devices = append(info.Devices, line)
		}
	}
	if len(info.Devices) > 0 {
		parts := strings.SplitN(info.Devices[0], ",", 2)
		if len(parts) == 2 {
			info.Driver = strings.TrimSpace(parts[1])
		}
		info.Available = true
		info.Message = fmt.Sprintf("检测到 %d 块 GPU", len(info.Devices))
	}
	return info
}

func (s *Service) HuggingFaceStatus() HuggingFaceStatus {
	tgiPort := hfTGIPort
	webuiPort := hfWebUIPort
	apiKey := "hf-local"
	st := HuggingFaceStatus{
		TGIPort:           tgiPort,
		WebUIPort:         webuiPort,
		Runtime:           "tgi",
		APIKey:            apiKey,
		OpenAICompat:      true,
		GPUAvailable:      s.GPUInfo().Available,
		HubURL:            hfHubOrigin,
		HFTokenConfigured: s.HFTokenConfigured(),
	}
	if app, err := s.appstore.Get(appstore.HFAppKey()); err == nil {
		st.Installed = app.Installed
		st.Status = app.Status
		if app.Status == "installing" {
			st.InstallStatus = "installing"
		}
	}
	all, _ := s.settings.GetAll()
	if all != nil {
		st.ModelID = all["hf_ai_model"]
		if rt := strings.TrimSpace(all["hf_ai_runtime"]); rt != "" {
			st.Runtime = rt
		}
		if st.Runtime == "ollama" {
			tgiPort = 11434
			webuiPort = hfWebUIPort
			apiKey = "ollama"
			st.APIKey = apiKey
		}
		if st.ModelID == "" {
			catalog := ModelCatalog()
			if len(catalog) > 0 && catalog[0].HFModelID != "" {
				st.ModelID = catalog[0].HFModelID
			}
		}
		st.PanelConfigured = all["ai_enabled"] == "true" &&
			(strings.Contains(all["ai_base_url"], fmt.Sprintf(":%d", hfTGIPort)) ||
				strings.Contains(all["ai_base_url"], ":11434"))
	}
	st.OllamaRunning = ollamaReachable()
	st.TGIRunning = dockerInspectRunning("owpanel-hf-tgi")
	st.WebUIRunning = dockerInspectRunning("owpanel-hf-webui")
	if st.Runtime == "ollama" && st.OllamaRunning {
		st.Status = "running"
	} else if st.TGIRunning {
		st.Status = "running"
	} else if st.Installed {
		st.Status = "stopped"
	}
	s.applyAIEndpointURLs(&st, tgiPort, webuiPort)
	st.TGIPort = tgiPort
	st.WebUIPort = webuiPort
	st.APISampleCurl = buildAPISampleCurl(st.APIBaseURLPublic, st.APIKey, st.ModelID)
	st.APISampleCurlLocal = buildAPISampleCurl(st.APIBaseURLLocal, st.APIKey, st.ModelID)
	if st.APISampleCurl == "" {
		st.APISampleCurl = st.APISampleCurlLocal
	}
	// Backward-compatible primary fields prefer public endpoint for external clients.
	st.APIBaseURL = st.APIBaseURLPublic
	if st.APIBaseURL == "" {
		st.APIBaseURL = st.APIBaseURLLocal
	}
	st.ChatURL = st.ChatURLPublic
	if st.ChatURL == "" {
		st.ChatURL = st.ChatURLLocal
	}
	logs := s.appstore.GetInstallLogs(appstore.HFAppKey())
	if logs.Status != "idle" {
		st.InstallStatus = logs.Status
	}
	return st
}

func ollamaReachable() bool {
	if _, err := exec.LookPath("ollama"); err != nil {
		return dockerInspectRunning("owpanel-ollama")
	}
	out, err := exec.Command("ollama", "list").Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

func (s *Service) resolvePublicIP() string {
	all, _ := s.settings.GetAll()
	if all != nil {
		if ip := strings.TrimSpace(all["server_public_ip"]); ip != "" {
			return ip
		}
	}
	if _, err := exec.LookPath("curl"); err != nil {
		return ""
	}
	out, err := exec.Command("curl", "-fsSL", "--max-time", "3", "ifconfig.me").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (s *Service) applyAIEndpointURLs(st *HuggingFaceStatus, apiPort, chatPort int) {
	st.APIBaseURLLocal = fmt.Sprintf("http://127.0.0.1:%d/v1", apiPort)
	st.ChatURLLocal = fmt.Sprintf("http://127.0.0.1:%d", chatPort)
	if ip := s.resolvePublicIP(); ip != "" {
		st.PublicIP = ip
		st.APIBaseURLPublic = fmt.Sprintf("http://%s:%d/v1", ip, apiPort)
		st.ChatURLPublic = fmt.Sprintf("http://%s:%d", ip, chatPort)
	}
}

func buildAPISampleCurl(baseURL, apiKey, modelID string) string {
	if baseURL == "" {
		return ""
	}
	model := modelID
	if model == "" {
		model = "your-model"
	}
	if idx := strings.LastIndex(model, "/"); idx >= 0 {
		model = model[idx+1:]
	}
	return fmt.Sprintf(`curl %s/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer %s" \
  -d '{"model":"%s","messages":[{"role":"user","content":"Hello"}]}'`, baseURL, apiKey, model)
}

func dockerInspectRunning(name string) bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", name).Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

func (s *Service) SetupHuggingFace(opts HuggingFaceOptions) error {
	token := strings.TrimSpace(opts.HFToken)
	if token != "" {
		if err := s.SaveHFToken(token); err != nil {
			return err
		}
	} else {
		opts.HFToken = s.resolveHFToken("")
	}
	if strings.TrimSpace(opts.ModelID) == "" && opts.CatalogID == "" {
		return fmt.Errorf("请选择或填写模型 ID")
	}
	return s.appstore.InstallHuggingFaceAI(opts)
}

func (s *Service) UninstallHuggingFace() error {
	return s.appstore.Uninstall(appstore.HFAppKey())
}

func (s *Service) ListAIAgents() ([]models.App, error) {
	list, err := s.appstore.ListInstalled()
	if err != nil {
		return nil, err
	}
	var agents []models.App
	for _, app := range list {
		if app.Category == "人工智能" && app.Installed {
			agents = append(agents, app)
		}
	}
	return agents, nil
}

func (s *Service) GetInstallLogs() appstore.InstallLogSnapshot {
	return s.appstore.GetInstallLogs(appstore.HFAppKey())
}
