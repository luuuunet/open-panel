package toolbox

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

type SnippetItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Command  string `json:"command"`
	Category string `json:"category"`
	Remark   string `json:"remark,omitempty"`
	Builtin  bool   `json:"builtin"`
}

type RunResult struct {
	Command  string `json:"command"`
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
	Duration int64  `json:"duration_ms"`
}

func builtinSnippets(lang string) []SnippetItem {
	if lang == "en" {
		return []SnippetItem{
			{ID: "builtin:disk", Name: "Disk usage", Command: "df -hT", Category: "inspect", Remark: "Filesystem usage", Builtin: true},
			{ID: "builtin:mem", Name: "Memory", Command: "free -h", Category: "inspect", Remark: "RAM and swap", Builtin: true},
			{ID: "builtin:top-cpu", Name: "Top CPU processes", Command: "ps aux --sort=-%cpu | head -n 15", Category: "inspect", Remark: "Highest CPU consumers", Builtin: true},
			{ID: "builtin:conn", Name: "Active connections", Command: "ss -s", Category: "network", Remark: "Socket summary", Builtin: true},
			{ID: "builtin:listen", Name: "Listening ports", Command: "ss -tulnp", Category: "network", Remark: "All listeners", Builtin: true},
			{ID: "builtin:docker-ps", Name: "Docker containers", Command: "docker ps -a --format 'table {{.Names}}\\t{{.Status}}\\t{{.Ports}}'", Category: "docker", Remark: "Container list", Builtin: true},
			{ID: "builtin:nginx-test", Name: "Nginx config test", Command: "nginx -t 2>&1 || openresty -t 2>&1", Category: "web", Remark: "Validate web server config", Builtin: true},
			{ID: "builtin:clear-logs", Name: "Truncate large logs", Command: "find /var/log -type f -name '*.log' -size +100M -exec truncate -s 0 {} \\; 2>/dev/null; echo done", Category: "maintain", Remark: "Zero files over 100MB", Builtin: true},
		}
	}
	return []SnippetItem{
		{ID: "builtin:disk", Name: "磁盘占用", Command: "df -hT", Category: "inspect", Remark: "各分区使用情况", Builtin: true},
		{ID: "builtin:mem", Name: "内存状态", Command: "free -h", Category: "inspect", Remark: "内存与 Swap", Builtin: true},
		{ID: "builtin:top-cpu", Name: "CPU 占用 TOP", Command: "ps aux --sort=-%cpu | head -n 15", Category: "inspect", Remark: "CPU 最高的进程", Builtin: true},
		{ID: "builtin:conn", Name: "连接统计", Command: "ss -s", Category: "network", Remark: "Socket 汇总", Builtin: true},
		{ID: "builtin:listen", Name: "监听端口", Command: "ss -tulnp", Category: "network", Remark: "全部监听端口", Builtin: true},
		{ID: "builtin:docker-ps", Name: "Docker 容器", Command: "docker ps -a --format 'table {{.Names}}\\t{{.Status}}\\t{{.Ports}}'", Category: "docker", Remark: "容器列表", Builtin: true},
		{ID: "builtin:nginx-test", Name: "Nginx 配置检测", Command: "nginx -t 2>&1 || openresty -t 2>&1", Category: "web", Remark: "检测 Web 服务器配置", Builtin: true},
		{ID: "builtin:clear-logs", Name: "截断超大日志", Command: "find /var/log -type f -name '*.log' -size +100M -exec truncate -s 0 {} \\; 2>/dev/null; echo done", Category: "maintain", Remark: "清空超过 100MB 的日志", Builtin: true},
	}
}

func (s *Service) ListSnippets(lang string) ([]SnippetItem, error) {
	out := builtinSnippets(lang)
	if s.db == nil {
		return out, nil
	}
	var rows []models.CommandSnippet
	if err := s.db.Order("id desc").Find(&rows).Error; err != nil {
		return out, err
	}
	for _, r := range rows {
		out = append(out, SnippetItem{
			ID: fmt.Sprintf("user:%d", r.ID), Name: r.Name, Command: r.Command,
			Category: r.Category, Remark: r.Remark, Builtin: false,
		})
	}
	return out, nil
}

func (s *Service) CreateSnippet(snippet *models.CommandSnippet) error {
	if s.db == nil {
		return fmt.Errorf("database unavailable")
	}
	if strings.TrimSpace(snippet.Name) == "" || strings.TrimSpace(snippet.Command) == "" {
		return fmt.Errorf("名称和命令不能为空")
	}
	return s.db.Create(snippet).Error
}

func (s *Service) UpdateSnippet(id uint, updates map[string]interface{}) error {
	if s.db == nil {
		return fmt.Errorf("database unavailable")
	}
	res := s.db.Model(&models.CommandSnippet{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Service) DeleteSnippet(id uint) error {
	if s.db == nil {
		return fmt.Errorf("database unavailable")
	}
	return s.db.Delete(&models.CommandSnippet{}, id).Error
}

func (s *Service) ResolveSnippetCommand(id string) (string, error) {
	if strings.HasPrefix(id, "builtin:") {
		key := strings.TrimPrefix(id, "builtin:")
		for _, item := range builtinSnippets("zh") {
			if item.ID == "builtin:"+key {
				return item.Command, nil
			}
		}
		for _, item := range builtinSnippets("en") {
			if item.ID == "builtin:"+key {
				return item.Command, nil
			}
		}
		return "", fmt.Errorf("内置片段不存在")
	}
	if strings.HasPrefix(id, "user:") && s.db != nil {
		var sid uint
		if _, err := fmt.Sscanf(strings.TrimPrefix(id, "user:"), "%d", &sid); err != nil {
			return "", err
		}
		var row models.CommandSnippet
		if err := s.db.First(&row, sid).Error; err != nil {
			return "", err
		}
		return row.Command, nil
	}
	return "", fmt.Errorf("无效的片段 ID")
}

func (s *Service) RunCommand(command string, timeoutSec int) (*RunResult, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, fmt.Errorf("命令不能为空")
	}
	if timeoutSec <= 0 {
		timeoutSec = 60
	}
	if timeoutSec > 120 {
		timeoutSec = 120
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	start := time.Now()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	out, err := cmd.CombinedOutput()
	result := &RunResult{
		Command:  command,
		Output:   string(out),
		Duration: time.Since(start).Milliseconds(),
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
			if result.Output == "" {
				result.Output = err.Error()
			}
		}
	}
	return result, nil
}
