package php

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

type ExtensionInfo struct {
	Name    string `json:"name"`
	File    string `json:"file,omitempty"`
	Enabled bool   `json:"enabled"`
	Loaded  bool   `json:"loaded"`
	Builtin bool   `json:"builtin"`
}

type PHPDetail struct {
	IniPath          string          `json:"ini_path"`
	DisableFunctions string          `json:"disable_functions"`
	Extensions       []ExtensionInfo `json:"extensions"`
	CanInstall       bool            `json:"can_install"`
}

var commonExtensions = []string{
	"mysqli", "pdo_mysql", "pdo_sqlite", "curl", "gd", "mbstring", "openssl",
	"fileinfo", "zip", "redis", "memcached", "imagick", "intl", "bcmath",
	"soap", "sockets", "xml", "xsl", "opcache",
}

var builtinModules = map[string]bool{
	"Core": true, "date": true, "hash": true, "json": true, "pcre": true,
	"Reflection": true, "SPL": true, "standard": true, "tokenizer": true,
}

func (m *Manager) IniPath(key string) (string, error) {
	return m.ensureIni(key)
}

func (m *Manager) ensureIni(key string) (string, error) {
	ver := VersionFromKey(key)
	if ver == "" {
		return "", fmt.Errorf("invalid php key: %s", key)
	}
	for _, p := range m.systemIniPaths(ver) {
		if fileExists(p) {
			return p, nil
		}
	}
	runtimeIni := filepath.Join(m.runtimeDir(key), "php.ini")
	if fileExists(runtimeIni) {
		return runtimeIni, nil
	}
	if bin, err := m.findBinary(ver); err == nil {
		candidate := filepath.Join(filepath.Dir(bin), "php.ini")
		if fileExists(candidate) {
			return candidate, nil
		}
	}
	if err := m.ensureRuntimeDir(key, ver); err != nil {
		return "", err
	}
	if fileExists(runtimeIni) {
		return runtimeIni, nil
	}
	if err := m.writeMinimalIni(runtimeIni, m.runtimeDir(key)); err != nil {
		return "", err
	}
	return runtimeIni, nil
}

func (m *Manager) ReadIni(key string) (string, error) {
	path, err := m.ensureIni(key)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (m *Manager) WriteIni(key, content string) error {
	path, err := m.IniPath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func (m *Manager) GetDirective(key, name string) string {
	content, err := m.ReadIni(key)
	if err != nil {
		return ""
	}
	return parseDirective(content, name)
}

func (m *Manager) SetDirective(key, name, value string) error {
	content, err := m.ReadIni(key)
	if err != nil {
		return err
	}
	updated := setDirective(content, name, value)
	return m.WriteIni(key, updated)
}

func (m *Manager) ApplyDirectives(key string, directives map[string]interface{}) error {
	ver := VersionFromKey(key)
	if ver != "" && runtime.GOOS == "linux" {
		for _, p := range m.systemIniPaths(ver) {
			if fileExists(p) {
				return m.writeAccelConf(ver, directives)
			}
		}
	}
	content, err := m.ReadIni(key)
	if err != nil {
		return err
	}
	for k, v := range directives {
		content = setDirective(content, k, fmt.Sprint(v))
	}
	return m.WriteIni(key, content)
}

func (m *Manager) writeAccelConf(version string, directives map[string]interface{}) error {
	path := m.accelConfPath(version)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("; OWPanel PHP acceleration\n")
	for k, v := range directives {
		b.WriteString(k)
		b.WriteString(" = ")
		b.WriteString(fmt.Sprint(v))
		b.WriteString("\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func (m *Manager) GetDisableFunctions(key string) string {
	return m.GetDirective(key, "disable_functions")
}

func (m *Manager) SetDisableFunctions(key, value string) error {
	return m.SetDirective(key, "disable_functions", value)
}

func (m *Manager) Detail(key string) (PHPDetail, error) {
	iniPath, err := m.IniPath(key)
	if err != nil {
		return PHPDetail{}, err
	}
	content, _ := os.ReadFile(iniPath)
	exts, _ := m.ListExtensions(key)
	return PHPDetail{
		IniPath:          iniPath,
		DisableFunctions: parseDirective(string(content), "disable_functions"),
		Extensions:       exts,
		CanInstall:       runtime.GOOS == "linux",
	}, nil
}

func (m *Manager) ListExtensions(key string) ([]ExtensionInfo, error) {
	ver := VersionFromKey(key)
	bin, err := m.findBinary(ver)
	if err != nil {
		return nil, err
	}
	extDir := filepath.Join(filepath.Dir(bin), "ext")
	loaded := m.loadedModules(bin)

	content, _ := m.ReadIni(key)
	enabledInIni := parseExtensionStates(content)

	var out []ExtensionInfo
	seen := map[string]bool{}

	if runtime.GOOS == "windows" {
		entries, _ := os.ReadDir(extDir)
		for _, e := range entries {
			name := e.Name()
			if !strings.HasPrefix(strings.ToLower(name), "php_") || !strings.HasSuffix(strings.ToLower(name), ".dll") {
				continue
			}
			base := strings.TrimSuffix(strings.TrimPrefix(name, "php_"), ".dll")
			base = strings.TrimSuffix(base, ".DLL")
			if seen[base] {
				continue
			}
			seen[base] = true
			enabled := enabledInIni[base] || enabledInIni[name]
			if v, ok := enabledInIni[base]; ok {
				enabled = v
			}
			out = append(out, ExtensionInfo{
				Name: base, File: name, Enabled: enabled,
				Loaded: loaded[base] || loaded[strings.ToLower(base)],
			})
		}
	}

	for _, name := range commonExtensions {
		if seen[name] {
			continue
		}
		seen[name] = true
		fileName := extensionFileName(name)
		filePath := filepath.Join(extDir, fileName)
		exists := fileExists(filePath)
		enabled := enabledInIni[name]
		if v, ok := enabledInIni[fileName]; ok {
			enabled = v
		}
		if !exists && !loaded[name] && !builtinModules[name] {
			if runtime.GOOS == "windows" {
				continue
			}
		}
		out = append(out, ExtensionInfo{
			Name: name, File: fileName, Enabled: enabled,
			Loaded: loaded[name], Builtin: builtinModules[name] || !exists && loaded[name],
		})
	}

	for mod := range loaded {
		low := strings.ToLower(mod)
		if seen[low] || builtinModules[mod] {
			continue
		}
		seen[low] = true
		out = append(out, ExtensionInfo{
			Name: low, Enabled: true, Loaded: true, Builtin: true,
		})
	}
	return out, nil
}

func (m *Manager) SetExtension(key, name string, enabled bool) error {
	ver := VersionFromKey(key)
	if ver != "" && runtime.GOOS == "linux" {
		if err := m.setLinuxExtension(ver, name, enabled); err == nil {
			return nil
		}
	}
	content, err := m.ReadIni(key)
	if err != nil {
		return err
	}
	fileName := extensionFileName(name)
	updated := setExtensionState(content, name, fileName, enabled)
	return m.WriteIni(key, updated)
}

func (m *Manager) setLinuxExtension(version, name string, enabled bool) error {
	name = strings.TrimSpace(strings.ToLower(name))
	confDir := filepath.Join("/etc/php", version, "fpm", "conf.d")
	avail := filepath.Join("/etc/php", version, "mods-available", name+".ini")
	linkName := filepath.Join(confDir, "20-"+name+".ini")
	if !fileExists(filepath.Join("/etc/php", version, "fpm", "php.ini")) {
		return fmt.Errorf("system php-fpm not installed")
	}
	if enabled {
		if fileExists(avail) {
			_ = os.Remove(linkName)
			if err := os.Symlink(avail, linkName); err != nil {
				return err
			}
			return nil
		}
		if err := os.MkdirAll(confDir, 0755); err != nil {
			return err
		}
		body := "extension=" + name + "\n"
		if name == "opcache" {
			body = "zend_extension=opcache.so\n"
		}
		return os.WriteFile(linkName, []byte(body), 0644)
	}
	_ = os.Remove(linkName)
	return nil
}

func (m *Manager) InstallExtension(key, name string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("扩展安装目前仅支持 Linux（apt），Windows 请在 ext 目录放置 DLL 后启用")
	}
	ver := VersionFromKey(key)
	pkgVer := ver
	pkg := fmt.Sprintf("php%s-%s", pkgVer, strings.ToLower(name))
	if out, err := exec.Command("apt-get", "install", "-y", pkg).CombinedOutput(); err != nil {
		alt := fmt.Sprintf("php-%s", strings.ToLower(name))
		if out2, err2 := exec.Command("apt-get", "install", "-y", alt).CombinedOutput(); err2 != nil {
			return fmt.Errorf("安装失败: %s / %s", strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)))
		}
	}
	return m.SetExtension(key, name, true)
}

func (m *Manager) loadedModules(bin string) map[string]bool {
	out := map[string]bool{}
	cmd := exec.Command(bin, "-m")
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = nil
	}
	data, err := cmd.Output()
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "[") {
			continue
		}
		out[line] = true
		out[strings.ToLower(line)] = true
	}
	return out
}

func extensionFileName(name string) string {
	name = strings.TrimSpace(name)
	if runtime.GOOS == "windows" {
		if strings.HasPrefix(strings.ToLower(name), "php_") {
			if !strings.HasSuffix(strings.ToLower(name), ".dll") {
				return name + ".dll"
			}
			return name
		}
		return "php_" + name + ".dll"
	}
	return name
}

var directiveRe = regexp.MustCompile(`^(\s*)([a-zA-Z0-9_.]+)\s*=\s*(.*)$`)

func parseDirective(content, name string) string {
	for _, line := range strings.Split(content, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, ";") || strings.HasPrefix(trim, "#") {
			continue
		}
		m := directiveRe.FindStringSubmatch(trim)
		if len(m) == 4 && strings.EqualFold(m[2], name) {
			return strings.TrimSpace(m[3])
		}
	}
	return ""
}

func setDirective(content, name, value string) string {
	lines := strings.Split(content, "\n")
	found := false
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, ";") || strings.HasPrefix(trim, "#") {
			continue
		}
		m := directiveRe.FindStringSubmatch(trim)
		if len(m) == 4 && strings.EqualFold(m[2], name) {
			lines[i] = m[1] + name + " = " + value
			found = true
			break
		}
	}
	if !found {
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += name + " = " + value + "\n"
		return content
	}
	return strings.Join(lines, "\n")
}

func parseExtensionStates(content string) map[string]bool {
	states := map[string]bool{}
	extRe := regexp.MustCompile(`^(\s*);?\s*(zend_)?extension\s*=\s*(.+)$`)
	for _, line := range strings.Split(content, "\n") {
		trim := strings.TrimSpace(line)
		m := extRe.FindStringSubmatch(trim)
		if len(m) != 4 {
			continue
		}
		enabled := !strings.HasPrefix(trim, ";")
		val := strings.TrimSpace(m[3])
		val = strings.Trim(val, `"'`)
		base := filepath.Base(val)
		base = strings.TrimSuffix(base, filepath.Ext(base))
		base = strings.TrimPrefix(strings.ToLower(base), "php_")
		states[base] = enabled
		states[val] = enabled
	}
	return states
}

func setExtensionState(content, name, fileName string, enabled bool) string {
	lines := strings.Split(content, "\n")
	name = strings.TrimSpace(name)
	fileName = strings.TrimSpace(fileName)
	targets := []string{name, fileName, extensionFileName(name)}
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(fileName), ".dll") {
		targets = append(targets, "php_"+name+".dll")
	}

	found := false
	extLineRe := regexp.MustCompile(`^(\s*);?\s*(zend_)?extension\s*=\s*(.+)$`)
	for i, line := range lines {
		m := extLineRe.FindStringSubmatch(strings.TrimSpace(line))
		if len(m) != 4 {
			continue
		}
		val := strings.Trim(strings.TrimSpace(m[3]), `"'`)
		for _, t := range targets {
			if strings.Contains(strings.ToLower(val), strings.ToLower(t)) ||
				strings.EqualFold(filepath.Base(val), t) {
				prefix := ""
				if !enabled {
					prefix = ";"
				}
				lines[i] = prefix + "extension=" + fileName
				found = true
				break
			}
		}
	}
	if !found {
		prefix := ""
		if !enabled {
			prefix = ";"
		}
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += prefix + "extension=" + fileName + "\n"
		return content
	}
	return strings.Join(lines, "\n")
}
