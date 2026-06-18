package aihub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/services/modelcatalog"
)

const hfHubOrigin = "https://huggingface.co"

// HubModelSummary is a normalized Hugging Face Hub model for the panel UI.
type HubModelSummary struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Author       string   `json:"author"`
	Downloads    int      `json:"downloads"`
	Gated        bool     `json:"gated"`
	PipelineTag  string   `json:"pipeline_tag"`
	Modality     string   `json:"modality"`
	Tags         []string `json:"tags"`
	LibraryName  string   `json:"library_name"`
	LastModified string   `json:"last_modified"`
	Deployable   bool     `json:"deployable"`
	RuntimeHint  string   `json:"runtime_hint"`
	DeployVia    string   `json:"deploy_via"`
	AppStoreKey  string   `json:"app_store_key"`
	DeployNote   string   `json:"deploy_note"`
	HubURL       string   `json:"hub_url"`
}

type hfAPIModel struct {
	ModelID      string          `json:"modelId"`
	ID           string          `json:"id"`
	Author       string          `json:"author"`
	Downloads    int             `json:"downloads"`
	Gated        json.RawMessage `json:"gated"`
	PipelineTag  string          `json:"pipeline_tag"`
	Tags         []string        `json:"tags"`
	LibraryName  string          `json:"library_name"`
	LastModified string          `json:"lastModified"`
}

type hfWhoami struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Type  string `json:"type"`
}

func (s *Service) HFTokenConfigured() bool {
	all, err := s.settings.GetAll()
	if err != nil {
		return false
	}
	return strings.TrimSpace(all["hf_token"]) != ""
}

func (s *Service) SaveHFToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("HF Token 不能为空")
	}
	return s.settings.Update(map[string]string{"hf_token": token})
}

func (s *Service) resolveHFToken(override string) string {
	if t := strings.TrimSpace(override); t != "" {
		return t
	}
	all, err := s.settings.GetAll()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(all["hf_token"])
}

func (s *Service) TestHFToken(token string) (map[string]string, error) {
	token = s.resolveHFToken(token)
	if token == "" {
		return nil, fmt.Errorf("请先填写或保存 HF Token")
	}
	var who hfWhoami
	if err := hfHubGET("/api/whoami-v2", token, &who); err != nil {
		return nil, err
	}
	if who.Name == "" && who.Type == "" {
		return nil, fmt.Errorf("Token 无效或已过期")
	}
	return map[string]string{
		"name":  who.Name,
		"email": who.Email,
		"type":  who.Type,
	}, nil
}

func (s *Service) SearchHubModels(query, task string, limit int, token string) ([]HubModelSummary, error) {
	if limit <= 0 || limit > 50 {
		limit = 40
	}
	token = s.resolveHFToken(token)
	q := url.Values{}
	q.Set("sort", "downloads")
	q.Set("direction", "-1")
	q.Set("limit", fmt.Sprintf("%d", limit))
	if strings.TrimSpace(query) != "" {
		q.Set("search", strings.TrimSpace(query))
	}
	task = strings.TrimSpace(task)
	if task != "" && task != "all" {
		q.Set("filter", task)
	}
	path := "/api/models?" + q.Encode()
	var raw []hfAPIModel
	if err := hfHubGET(path, token, &raw); err != nil {
		return nil, err
	}
	out := make([]HubModelSummary, 0, len(raw))
	seen := map[string]bool{}
	for _, m := range raw {
		summary := mapHubModel(m)
		if seen[summary.ID] {
			continue
		}
		seen[summary.ID] = true
		out = append(out, summary)
	}
	// Merge curated presets when search is empty or short.
	if strings.TrimSpace(query) == "" {
		out = mergeCuratedHubResults(out, task)
	}
	return out, nil
}

func mergeCuratedHubResults(hub []HubModelSummary, task string) []HubModelSummary {
	modality := hubTaskModality(task)
	seen := map[string]bool{}
	for _, m := range hub {
		seen[m.ID] = true
	}
	prefix := make([]HubModelSummary, 0)
	for _, c := range modelcatalog.Catalog() {
		if c.HFModelID == "" {
			continue
		}
		if modality != "" && c.Modality != modality {
			continue
		}
		if task != "" && task != "all" && c.PipelineTag != task {
			continue
		}
		if seen[c.HFModelID] {
			continue
		}
		seen[c.HFModelID] = true
		deployable, runtime, via, appKey, note := hubDeployHint(c.PipelineTag, c.HFModelID)
		if c.DeployVia == modelcatalog.DeployViaTGI || c.DeployVia == modelcatalog.DeployViaOllama {
			deployable = c.HubDeployable
			runtime = c.DeployVia
		}
		if c.DeployVia != "" {
			via = c.DeployVia
		}
		if c.AppStoreKey != "" {
			appKey = c.AppStoreKey
		}
		prefix = append(prefix, HubModelSummary{
			ID: c.HFModelID, Name: c.Name, Author: strings.Split(c.HFModelID, "/")[0],
			Gated: c.Gated, PipelineTag: c.PipelineTag, Modality: c.Modality,
			Deployable: deployable, RuntimeHint: runtime, DeployVia: via, AppStoreKey: appKey,
			DeployNote: note, HubURL: hfHubOrigin + "/" + c.HFModelID, Tags: c.Tags,
		})
	}
	return append(prefix, hub...)
}

func hubTaskModality(task string) string {
	for _, t := range modelcatalog.HubTasks() {
		if t.ID == task {
			return t.Modality
		}
	}
	return ""
}

func (s *Service) GetHubModel(repoID, token string) (*HubModelSummary, error) {
	repoID = strings.TrimSpace(repoID)
	if repoID == "" {
		return nil, fmt.Errorf("model id required")
	}
	token = s.resolveHFToken(token)
	path := "/api/models/" + encodeRepoPath(repoID)
	var raw hfAPIModel
	if err := hfHubGET(path, token, &raw); err != nil {
		return nil, err
	}
	summary := mapHubModel(raw)
	return &summary, nil
}

func hfHubGET(path, token string, dest any) error {
	req, err := http.NewRequest(http.MethodGet, hfHubOrigin+path, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("User-Agent", "owpanel/1.0")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("连接 huggingface.co 失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("HF Token 无效或无权限访问该模型")
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("模型不存在: %s", path)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return fmt.Errorf("Hugging Face API 错误 (%d): %s", resp.StatusCode, msg)
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("解析 Hugging Face 响应失败: %w", err)
	}
	return nil
}

func encodeRepoPath(repoID string) string {
	parts := strings.Split(repoID, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func mapHubModel(m hfAPIModel) HubModelSummary {
	id := m.ModelID
	if id == "" {
		id = m.ID
	}
	name := id
	if idx := strings.LastIndex(id, "/"); idx >= 0 {
		name = id[idx+1:]
	}
	deployable, runtime, deployVia, appKey, note := hubDeployHint(m.PipelineTag, id)
	return HubModelSummary{
		ID:           id,
		Name:         name,
		Author:       m.Author,
		Downloads:    m.Downloads,
		Gated:        parseGated(m.Gated),
		PipelineTag:  m.PipelineTag,
		Modality:     pipelineModality(m.PipelineTag),
		Tags:         m.Tags,
		LibraryName:  m.LibraryName,
		LastModified: m.LastModified,
		Deployable:   deployable,
		RuntimeHint:  runtime,
		DeployVia:    deployVia,
		AppStoreKey:  appKey,
		DeployNote:   note,
		HubURL:       hfHubOrigin + "/" + id,
	}
}

func parseGated(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return b
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.ToLower(strings.TrimSpace(s))
		return s == "true" || s == "auto" || s == "manual"
	}
	return false
}

func pipelineModality(tag string) string {
	for _, t := range modelcatalog.HubTasks() {
		if t.ID == tag {
			return t.Modality
		}
	}
	switch strings.ToLower(tag) {
	case "text-generation", "text2text-generation", "conversational":
		return modelcatalog.ModalityText
	case "text-to-image", "image-to-image":
		return modelcatalog.ModalityImage
	case "image-to-text", "visual-question-answering", "image-text-to-text":
		return modelcatalog.ModalityVision
	case "automatic-speech-recognition", "text-to-speech", "audio-to-audio", "audio-classification":
		return modelcatalog.ModalityAudio
	case "text-to-video", "image-to-video":
		return modelcatalog.ModalityVideo
	}
	return ""
}

func hubDeployHint(pipelineTag, modelID string) (deployable bool, runtime, deployVia, appStoreKey, note string) {
	tag := strings.ToLower(strings.TrimSpace(pipelineTag))
	id := strings.ToLower(modelID)
	switch tag {
	case "text-generation", "text2text-generation", "conversational":
		return true, "tgi", modelcatalog.DeployViaTGI, "", ""
	case "image-to-text", "visual-question-answering", "image-text-to-text":
		if isVisionLLM(id) {
			return true, "tgi", modelcatalog.DeployViaTGI, "", ""
		}
		if strings.Contains(id, "ocr") || strings.Contains(id, "trocr") {
			return false, "", modelcatalog.DeployViaManual, "", "OCR 模型需 transformers/vLLM 等专用推理服务，当前一键部署仅支持对话类 TGI"
		}
		return false, "", modelcatalog.DeployViaManual, "", "图像描述/分类模型（如 BLIP）需专用推理框架，暂不支持 TGI 一键部署"
	case "text-to-image", "image-to-image":
		if strings.Contains(id, "flux") {
			return false, "", modelcatalog.DeployViaComfyUI, "comfyui", "请安装 ComfyUI 后加载该模型"
		}
		return false, "", modelcatalog.DeployViaSDWebUI, "sd-webui", "请安装 SD WebUI 后加载该模型"
	case "automatic-speech-recognition":
		return false, "", modelcatalog.DeployViaWhisper, "whisper", "请从软件商店安装 Whisper"
	case "text-to-speech", "audio-to-audio", "audio-classification":
		return false, "", modelcatalog.DeployViaManual, "", "音频模型需专用 TTS/音乐生成服务，暂未接入一键部署"
	case "text-to-video", "image-to-video":
		return false, "", modelcatalog.DeployViaManual, "", "视频生成模型需高显存与专用推理框架，暂未接入一键部署"
	case "":
		if strings.Contains(modelID, "/") {
			return true, "tgi", modelcatalog.DeployViaTGI, "", ""
		}
	}
	return false, "", modelcatalog.DeployViaManual, "", "该模型类型暂未接入面板一键部署"
}

func isVisionLLM(modelID string) bool {
	return (strings.Contains(modelID, "qwen") && strings.Contains(modelID, "vl")) ||
		strings.Contains(modelID, "llava") ||
		strings.Contains(modelID, "internvl") ||
		strings.Contains(modelID, "cogvlm")
}
