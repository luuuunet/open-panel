package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SetNginxConfResolver returns the active nginx/openresty main config path.
func (s *Service) SetNginxConfResolver(fn func() string) {
	s.nginxConfPath = fn
}

func (s *Service) ensureCacheInclude() (bool, string) {
	if s.nginxConfPath == nil {
		return false, ""
	}
	confPath := strings.TrimSpace(s.nginxConfPath())
	if confPath == "" {
		return false, ""
	}
	cachePath := filepath.ToSlash(s.ConfPath())
	includeLine := fmt.Sprintf("include %s;", cachePath)
	marker := "owpanel-cache.conf"

	data, err := os.ReadFile(confPath)
	if err != nil {
		return false, fmt.Sprintf("无法读取 Nginx 主配置 %s: %v", confPath, err)
	}
	content := string(data)
	if strings.Contains(content, marker) {
		return true, ""
	}

	updated, ok := injectHTTPInclude(content, includeLine)
	if !ok {
		snippetPath := filepath.Join(s.confDir, "owpanel-http-snippet.conf")
		snippet := fmt.Sprintf("# OWPanel CDN cache — include from http {}\n%s\n", includeLine)
		if err := os.WriteFile(snippetPath, []byte(snippet), 0644); err != nil {
			return false, err.Error()
		}
		snippetInclude := fmt.Sprintf("include %s;", filepath.ToSlash(snippetPath))
		return false, fmt.Sprintf("未能自动写入 %s 的 http {} 块，请手动添加: %s", confPath, snippetInclude)
	}

	if err := os.WriteFile(confPath, []byte(updated), 0644); err != nil {
		return false, err.Error()
	}
	return true, ""
}

func injectHTTPInclude(content, includeLine string) (string, bool) {
	lower := strings.ToLower(content)
	idx := strings.Index(lower, "http")
	if idx < 0 {
		return content, false
	}
	brace := strings.Index(content[idx:], "{")
	if brace < 0 {
		return content, false
	}
	insertAt := idx + brace + 1
	// Skip whitespace/newline after {
	for insertAt < len(content) && (content[insertAt] == ' ' || content[insertAt] == '\t') {
		insertAt++
	}
	if insertAt < len(content) && content[insertAt] == '\r' {
		insertAt++
	}
	if insertAt < len(content) && content[insertAt] == '\n' {
		insertAt++
	}
	block := fmt.Sprintf("    # OWPanel CDN cache\n    %s\n", includeLine)
	return content[:insertAt] + block + content[insertAt:], true
}
