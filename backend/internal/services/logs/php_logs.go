package logs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var phpKeyVersions = map[string]string{
	"php83": "8.3", "php82": "8.2", "php81": "8.1", "php74": "7.4",
}

func phpVersionFromKey(key string) string {
	return phpKeyVersions[key]
}

func phpFPMServiceLogPaths(version string) []string {
	if version == "" {
		return nil
	}
	return []string{
		filepath.Join("/var/log", fmt.Sprintf("php%s-fpm.log", version)),
		filepath.Join("/var/log/php-fpm", fmt.Sprintf("%s-fpm.log", version)),
	}
}

func phpFPMErrorLogPaths(version, installPath string) []string {
	if version == "" {
		return nil
	}
	paths := []string{
		filepath.Join("/var/log", fmt.Sprintf("php%s-fpm.log", version)),
		filepath.Join("/var/log", fmt.Sprintf("php%s-fpm", version), "error.log"),
	}
	if installPath != "" {
		paths = append(paths, filepath.Join(installPath, "var", "log", "php_errors.log"))
	}
	return paths
}

func phpRuntimeLogPaths(key, dataDir string) []string {
	phpDir := filepath.Join(dataDir, "php", key)
	return []string{
		filepath.Join(phpDir, "php_errors.log"),
		filepath.Join(phpDir, "php-cgi.log"),
	}
}

func phpIniPaths(key, dataDir string) []string {
	ver := phpVersionFromKey(key)
	phpDir := filepath.Join(dataDir, "php", key)
	var paths []string
	paths = append(paths, filepath.Join(phpDir, "php.ini"))
	if ver != "" {
		paths = append(paths,
			filepath.Join("/etc/php", ver, "fpm", "php.ini"),
			filepath.Join("/etc/php", ver, "cli", "php.ini"),
			filepath.Join(dataDir, "server", "php"+strings.ReplaceAll(ver, ".", ""), "etc", "php.ini"),
		)
	}
	return paths
}

func phpIniErrorLogFromInis(iniPaths ...string) string {
	for _, iniPath := range iniPaths {
		if p := parseIniErrorLog(iniPath); p != "" {
			return p
		}
	}
	return ""
}

func parseIniErrorLog(iniPath string) string {
	data, err := os.ReadFile(iniPath)
	if err != nil {
		return ""
	}
	dir := filepath.Dir(iniPath)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(line), "error_log") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		val := strings.Trim(strings.TrimSpace(parts[1]), `"`)
		if val == "" || val == "syslog" {
			continue
		}
		if !filepath.IsAbs(val) {
			val = filepath.Join(dir, val)
		}
		return val
	}
	return ""
}

func logCandidatesForPHP(key, installPath, dataDir string) []logCandidate {
	var out []logCandidate
	add := func(suffix, label, path string) {
		if path == "" {
			return
		}
		out = append(out, logCandidate{suffix: suffix, label: label, path: path})
	}

	ver := phpVersionFromKey(key)
	phpDir := filepath.Join(dataDir, "php", key)
	iniErrLog := phpIniErrorLogFromInis(phpIniPaths(key, dataDir)...)

	add("php_cgi", "php-cgi 输出", firstExisting(
		filepath.Join(phpDir, "php-cgi.log"),
	))

	add("php_fpm", "PHP-FPM 日志", firstExisting(phpFPMServiceLogPaths(ver)...))

	errorCandidates := []string{}
	if iniErrLog != "" {
		errorCandidates = append(errorCandidates, iniErrLog)
	}
	errorCandidates = append(errorCandidates, phpFPMErrorLogPaths(ver, installPath)...)
	errorCandidates = append(errorCandidates, filepath.Join(phpDir, "php_errors.log"))
	resolvedError := firstExisting(errorCandidates...)
	add("php_error", "PHP 错误日志", resolvedError)

	if iniErrLog != "" && iniErrLog != resolvedError {
		add("php_ini_error", "php.ini error_log", iniErrLog)
	}

	return out
}
