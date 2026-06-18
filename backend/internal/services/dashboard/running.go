package dashboard

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

var (
	ollamaCacheMu sync.RWMutex
	ollamaCache   []AIModelInfo
	ollamaCacheAt time.Time
)

type RunningAppInfo struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Status   string `json:"status"`
	Port     int    `json:"port"`
	Version  string `json:"version"`
}

type AIModelInfo struct {
	Provider  string `json:"provider"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	SizeVRAM  int64  `json:"size_vram"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type RunningSnapshot struct {
	TopProcesses []ProcessBrief   `json:"top_processes"`
	RunningApps  []RunningAppInfo `json:"running_apps"`
	AIModels     []AIModelInfo    `json:"ai_models"`
}

type MonitorExtras struct {
	TopProcesses  []ProcessBrief
	RunningApps   []RunningAppInfo
	InstalledApps []InstalledAppMetrics
}

type ProcessBrief struct {
	PID     int32   `json:"pid"`
	Name    string  `json:"name"`
	CPU     float64 `json:"cpu"`
	Memory  float32 `json:"memory"`
	Command string  `json:"command"`
}

func (s *Service) RunningAppsFromDB() []RunningAppInfo {
	if s.db == nil {
		return nil
	}
	var apps []models.App
	if err := s.db.Where("installed = ? AND status = ?", true, "running").Order("category, name").Find(&apps).Error; err != nil {
		return nil
	}
	out := make([]RunningAppInfo, 0, len(apps))
	for _, a := range apps {
		out = append(out, RunningAppInfo{
			Key: a.Key, Name: a.Name, Category: a.Category,
			Status: a.Status, Port: a.Port, Version: a.Version,
		})
	}
	return out
}

func FetchOllamaModels() []AIModelInfo {
	ollamaCacheMu.RLock()
	if time.Since(ollamaCacheAt) < 60*time.Second && ollamaCache != nil {
		out := ollamaCache
		ollamaCacheMu.RUnlock()
		return out
	}
	ollamaCacheMu.RUnlock()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:11434/api/ps")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	var parsed struct {
		Models []struct {
			Name      string `json:"name"`
			Size      int64  `json:"size"`
			SizeVRAM  int64  `json:"size_vram"`
			ExpiresAt string `json:"expires_at"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}
	out := make([]AIModelInfo, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		out = append(out, AIModelInfo{
			Provider: "ollama", Name: m.Name, Size: m.Size,
			SizeVRAM: m.SizeVRAM, ExpiresAt: m.ExpiresAt,
		})
	}
	ollamaCacheMu.Lock()
	ollamaCache = out
	ollamaCacheAt = time.Now()
	ollamaCacheMu.Unlock()
	return out
}

func FormatModelSize(bytes int64) string {
	if bytes <= 0 {
		return "—"
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
