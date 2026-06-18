package edgeworker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

type ApplyResult struct {
	ConfPath    string `json:"conf_path"`
	Preview     string `json:"preview"`
	NginxReload bool   `json:"nginx_reloaded"`
	Message     string `json:"message"`
	Runtime     string `json:"runtime"`
}

func (s *Service) Preview() (string, error) {
	workers, err := s.ListEnabled()
	if err != nil {
		return "", err
	}
	rt := s.DetectRuntime()
	return s.generateAll(workers, rt), nil
}

func (s *Service) Apply() (*ApplyResult, error) {
	rt := s.DetectRuntime()
	if rt.Runtime != "openresty" && rt.Runtime != "nginx" {
		return nil, fmt.Errorf("%s", rt.Message)
	}
	if rt.Runtime == "nginx" && !rt.NjsAvailable {
		workers, _ := s.ListEnabled()
		for _, w := range workers {
			if w.ScriptType == "lua" {
				return nil, fmt.Errorf("Lua workers require OpenResty. Install OpenResty from Software Store or convert workers to template/njs type")
			}
		}
	}

	workers, err := s.ListEnabled()
	if err != nil {
		return nil, err
	}

	prefix := ""
	if s.apiPrefix != nil {
		prefix = s.apiPrefix()
	}
	if err := s.WriteRuntimeLua(s.panelPort, prefix); err != nil {
		return nil, fmt.Errorf("write edge runtime: %w", err)
	}

	preview := s.generateAll(workers, rt)

	confPath := s.ConfPath()
	if err := os.WriteFile(confPath, []byte(s.generateHTTP(workers, rt)), 0644); err != nil {
		return nil, err
	}

	siteMap, globalWorkers := s.buildSiteWorkersMap(workers)
	if err := os.WriteFile(s.GlobalServerConfPath(), []byte(s.generateServerSnippet(globalWorkers, rt, "global")), 0644); err != nil {
		return nil, err
	}

	var allSites []models.Website
	_ = s.db.Find(&allSites).Error
	for _, site := range allSites {
		siteWorkers := siteMap[site.ID]
		path := s.SiteServerConfPath(site.ID)
		if len(siteWorkers) == 0 {
			_ = os.Remove(path)
			continue
		}
		if err := os.WriteFile(path, []byte(s.generateServerSnippet(siteWorkers, rt, fmt.Sprintf("site-%d", site.ID))), 0644); err != nil {
			return nil, err
		}
	}

	included, includeHint := s.ensureEdgeInclude()

	reloaded := false
	if s.regen != nil {
		_ = s.regen()
	}
	if err := exec.Command("nginx", "-t").Run(); err == nil {
		if s.reload != nil {
			if err := s.reload(); err == nil {
				reloaded = true
			}
		} else if err := exec.Command("nginx", "-s", "reload").Run(); err == nil {
			reloaded = true
		}
	}

	include := nginxPath(confPath)
	msg := fmt.Sprintf("Edge Workers 配置已写入 %s，请在 nginx http {} 中添加: include %s;", include, include)
	if included {
		msg = "Edge Workers 配置已应用，已自动写入 Nginx http include"
	} else if includeHint != "" {
		msg = "Edge Workers 配置已写入；" + includeHint
	}
	if reloaded {
		msg = "Edge Workers 已部署，Nginx 已重载，站点虚拟主机已更新"
	}
	if rt.RecommendOpen && rt.Runtime == "nginx" {
		msg += "（建议安装 OpenResty 以获得完整 Lua 边缘计算能力）"
	}

	return &ApplyResult{
		ConfPath:    confPath,
		Preview:     preview,
		NginxReload: reloaded,
		Message:     msg,
		Runtime:     rt.Runtime,
	}, nil
}

func (s *Service) generateAll(workers []models.EdgeWorker, rt RuntimeInfo) string {
	var b strings.Builder
	b.WriteString("# OWPanel Edge Workers — preview\n")
	b.WriteString(fmt.Sprintf("# runtime: %s (lua=%v njs=%v)\n\n", rt.Runtime, rt.LuaAvailable, rt.NjsAvailable))
	b.WriteString("=== http {} block (edgeworkers.conf) ===\n")
	b.WriteString(s.generateHTTP(workers, rt))
	siteMap, globalWorkers := s.buildSiteWorkersMap(workers)
	b.WriteString("\n=== global server include (wildcard * domains) ===\n")
	b.WriteString(s.generateServerSnippet(globalWorkers, rt, "global"))
	for _, id := range sortedSiteIDs(siteMap) {
		b.WriteString(fmt.Sprintf("\n=== site %d server include ===\n", id))
		b.WriteString(s.generateServerSnippet(siteMap[id], rt, fmt.Sprintf("site-%d", id)))
	}
	return b.String()
}

func (s *Service) generateHTTP(workers []models.EdgeWorker, rt RuntimeInfo) string {
	var b strings.Builder
	b.WriteString("# OWPanel Edge Workers — auto generated (Cloudflare Workers-style)\n")
	b.WriteString(fmt.Sprintf("# runtime: %s\n\n", rt.Runtime))
	if len(workers) == 0 {
		b.WriteString("# No enabled workers\n")
		return b.String()
	}
	if rt.LuaAvailable {
		b.WriteString("lua_shared_dict op_edge_workers 10m;\n")
		nsIDs, _ := s.CollectKVNamespaceIDs()
		for _, id := range nsIDs {
			b.WriteString(fmt.Sprintf("lua_shared_dict %s 2m;\n", sharedDictName(id)))
		}
		luaDir := nginxPath(s.LuaDir())
		b.WriteString(fmt.Sprintf("\nlua_package_path \"%s/?.lua;;\";\n", luaDir))
		b.WriteString("\n")
	}
	if rt.NjsAvailable {
		for _, w := range workers {
			if w.ScriptType == "njs" {
				scriptPath := s.njsScriptPath(w.ID)
				b.WriteString(fmt.Sprintf("js_import op_edge_%d from %s;\n", w.ID, nginxPath(scriptPath)))
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (s *Service) generateServerSnippet(workers []models.EdgeWorker, rt RuntimeInfo, label string) string {
	if len(workers) == 0 {
		return fmt.Sprintf("# OWPanel Edge Workers — %s (empty)\n", label)
	}
	sort.SliceStable(workers, func(i, j int) bool {
		if workers[i].Priority != workers[j].Priority {
			return workers[i].Priority < workers[j].Priority
		}
		return workers[i].ID < workers[j].ID
	})

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# OWPanel Edge Workers — %s\n", label))
	var headerFilters []models.EdgeWorker
	for _, w := range workers {
		triggers := parseTriggers(w.Triggers)
		if w.ScriptType == "template" {
			b.WriteString(fmt.Sprintf("\n# Worker: %s (template, priority %d)\n", w.Name, w.Priority))
			b.WriteString(indentSnippet(w.Script, 4))
			b.WriteString("\n")
			continue
		}
		if hasTrigger(triggers, "request") {
			b.WriteString(s.generateRequestWorker(w, rt))
		}
		if hasTrigger(triggers, "response") {
			headerFilters = append(headerFilters, w)
		}
	}
	if len(headerFilters) > 0 && rt.LuaAvailable {
		b.WriteString("\nheader_filter_by_lua_block {\n")
		for _, w := range headerFilters {
			b.WriteString(fmt.Sprintf("    -- Worker: %s\n", w.Name))
			if prelude, err := s.GenerateBindingPrelude(w.ID); err == nil && prelude != "" {
				b.WriteString(indentSnippet(prelude, 4))
			}
			b.WriteString(indentSnippet(w.Script, 4))
		}
		b.WriteString("}\n")
	}
	return b.String()
}

func (s *Service) generateRequestWorker(w models.EdgeWorker, rt RuntimeInfo) string {
	loc := formatRoute(w.RoutePattern)
	domains := ParseDomains(&w)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n# Worker: %s (priority %d, domains: %s)\n", w.Name, w.Priority, w.Domains))
	switch w.ScriptType {
	case "lua":
		if !rt.LuaAvailable {
			b.WriteString("# skipped: Lua requires OpenResty\n")
			return b.String()
		}
		b.WriteString(fmt.Sprintf("%s {\n", loc))
		b.WriteString("    access_by_lua_block {\n")
		prelude, _ := s.GenerateBindingPrelude(w.ID)
		edgeRequired := prelude != ""
		if prelude != "" {
			b.WriteString(indentSnippet(prelude, 8))
		}
		if guard := hostGuardLua(domains, edgeRequired); guard != "" {
			b.WriteString(indentSnippet(guard, 8))
		}
		b.WriteString(indentSnippet(w.Script, 8))
		b.WriteString("\n    }\n}\n")
		_ = os.WriteFile(s.luaScriptPath(w.ID), []byte(w.Script), 0644)
	case "njs":
		if !rt.NjsAvailable {
			b.WriteString("# skipped: njs module not available\n")
			return b.String()
		}
		scriptPath := s.njsScriptPath(w.ID)
		body := wrapNJS(w.Script, w.ID)
		_ = os.WriteFile(scriptPath, []byte(body), 0644)
		b.WriteString(fmt.Sprintf("%s {\n    js_content op_edge_%d.requestHandler;\n}\n", loc, w.ID))
	}
	return b.String()
}

func wrapNJS(script string, id uint) string {
	if strings.Contains(script, "export default") {
		return script
	}
	return fmt.Sprintf(`function requestHandler(r) {
%s
}
export default { requestHandler };
`, indentSnippet(script, 4))
}

func (s *Service) luaScriptPath(id uint) string {
	return filepath.Join(s.confDir, "scripts", fmt.Sprintf("worker-%d.lua", id))
}

func (s *Service) njsScriptPath(id uint) string {
	return filepath.Join(s.confDir, "scripts", fmt.Sprintf("worker-%d.js", id))
}

func (s *Service) ServerBlockDirectives(site *models.Website) string {
	var parts []string
	global := s.GlobalServerConfPath()
	if st, err := os.Stat(global); err == nil && st.Size() > 0 {
		parts = append(parts, fmt.Sprintf("include %s;", nginxPath(global)))
	}
	if site != nil && site.ID > 0 {
		sitePath := s.SiteServerConfPath(site.ID)
		if st, err := os.Stat(sitePath); err == nil && st.Size() > 0 {
			parts = append(parts, fmt.Sprintf("include %s;", nginxPath(sitePath)))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "\n    " + strings.Join(parts, "\n    ")
}

func sortedSiteIDs(siteMap map[uint][]models.EdgeWorker) []uint {
	var ids []uint
	for id := range siteMap {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func parseTriggers(raw string) []string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return []string{"request"}
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "access" {
			p = "request"
		}
		if p == "header_filter" {
			p = "response"
		}
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func hasTrigger(triggers []string, name string) bool {
	for _, t := range triggers {
		if t == name {
			return true
		}
	}
	return false
}

func formatRoute(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "/" {
		return "location /"
	}
	if strings.HasPrefix(pattern, "~") || strings.HasPrefix(pattern, "^~") || strings.HasPrefix(pattern, "=") {
		return "location " + pattern
	}
	if !strings.HasPrefix(pattern, "/") {
		pattern = "/" + pattern
	}
	return "location ^~ " + pattern
}

func indentSnippet(script string, spaces int) string {
	pad := strings.Repeat(" ", spaces)
	lines := strings.Split(strings.TrimRight(script, "\n"), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			lines[i] = pad + line
		}
	}
	return strings.Join(lines, "\n")
}

func nginxPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return filepath.ToSlash(p)
}
