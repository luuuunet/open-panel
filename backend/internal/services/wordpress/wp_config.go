package wordpress

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var wpFTPDefineRE = regexp.MustCompile(`(?m)^\s*define\s*\(\s*['"](?:FS_METHOD|FTP_HOST|FTP_USER|FTP_PASS|FTP_SSL|FTP_BASE|FTP_CONTENT_DIR|FTP_PLUGIN_DIR|FTP_THEME_DIR)['"]\s*,[^\n]*\n`)
var wpDefaultSaltBlockRE = regexp.MustCompile(`(?s)/\*\*#@\+.*?/\*\*#@-?\*/\s*`)
var wpSaltDefineRE = regexp.MustCompile(`(?m)^\s*define\s*\(\s*['"](?:AUTH_KEY|SECURE_AUTH_KEY|LOGGED_IN_KEY|NONCE_KEY|AUTH_SALT|SECURE_AUTH_SALT|LOGGED_IN_SALT|NONCE_SALT)['"]\s*,[^\n]*\n`)

const wpProxyHTTPSBlock = `// OWPanel — trust reverse proxy HTTPS (Cloudflare / CDN)
if (isset($_SERVER['HTTP_X_FORWARDED_PROTO']) && $_SERVER['HTTP_X_FORWARDED_PROTO'] === 'https') {
	$_SERVER['HTTPS'] = 'on';
}
if (isset($_SERVER['HTTP_CF_VISITOR'])) {
	$op_cf = json_decode($_SERVER['HTTP_CF_VISITOR'], true);
	if (is_array($op_cf) && ($op_cf['scheme'] ?? '') === 'https') {
		$_SERVER['HTTPS'] = 'on';
	}
}

`

func buildWPProxyHTTPSBlock() string {
	return wpProxyHTTPSBlock
}

func stripDefaultWPSalts(content string) string {
	content = wpDefaultSaltBlockRE.ReplaceAllString(content, "")
	return wpSaltDefineRE.ReplaceAllString(content, "")
}

func ensureWPProxyHTTPS(content string) string {
	if strings.Contains(content, "OWPanel — trust reverse proxy HTTPS") {
		return content
	}
	if idx := strings.Index(content, "<?php"); idx >= 0 {
		end := idx + len("<?php")
		if nl := strings.Index(content[end:], "\n"); nl >= 0 {
			end += nl + 1
		}
		return content[:end] + "\n" + wpProxyHTTPSBlock + content[end:]
	}
	return wpProxyHTTPSBlock + content
}

func wpFTPHost() string {
	return "127.0.0.1"
}

func escapePHPString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}

func buildWPFilesystemBlock() string {
	return `// OWPanel — direct filesystem for plugin/theme installs
define('FS_METHOD', 'direct');
`
}

func buildWPFTPBlock(ftpUser, ftpPass string) string {
	return fmt.Sprintf(`// OWPanel — auto FTP for plugin/theme installs
define('FS_METHOD', 'ftpext');
define('FTP_HOST', '%s');
define('FTP_USER', '%s');
define('FTP_PASS', '%s');
`, wpFTPHost(), escapePHPString(ftpUser), escapePHPString(ftpPass))
}

func (s *Service) writeWPFilesystemConfig(root string, logger *DeployLogger) error {
	target := filepath.Join(root, "wp-config.php")
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("wp-config.php 不存在")
	}
	b, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	content := stripWPFilesystemBlock(string(b))
	block := buildWPFilesystemBlock()
	marker := "/* That's all, stop editing!"
	if idx := strings.Index(content, marker); idx >= 0 {
		content = content[:idx] + block + content[idx:]
	} else {
		content = strings.TrimRight(content, "\n") + "\n" + block
	}
	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		return err
	}
	if logger != nil {
		logger.Info("✓ 已配置 WordPress 直接写文件（无需 FTP）")
	}
	return nil
}

func (s *Service) writeWPFTPConfig(root, ftpUser, ftpPass string, logger *DeployLogger) error {
	_ = s.ensureWPSiteOwnership(root, logger)
	return s.writeWPFilesystemConfig(root, logger)
}

func stripWPFilesystemBlock(content string) string {
	return stripWPFTPBlock(content)
}

func stripWPFTPBlock(content string) string {
	lines := strings.Split(content, "\n")
	var out []string
	skip := false
	for _, line := range lines {
		if strings.Contains(line, "OWPanel — auto FTP") || strings.Contains(line, "OWPanel — direct filesystem") {
			skip = true
			continue
		}
		if skip {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				skip = false
				continue
			}
			if isWPFilesystemDefine(trimmed) {
				continue
			}
			skip = false
		}
		out = append(out, line)
	}
	content = strings.Join(out, "\n")
	return wpFTPDefineRE.ReplaceAllString(content, "")
}

func isWPFilesystemDefine(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	if !strings.HasPrefix(lower, "define(") {
		return false
	}
	return strings.Contains(lower, "fs_method") ||
		strings.Contains(lower, "ftp_host") ||
		strings.Contains(lower, "ftp_user") ||
		strings.Contains(lower, "ftp_pass") ||
		strings.Contains(lower, "ftp_ssl") ||
		strings.Contains(lower, "ftp_base") ||
		strings.Contains(lower, "ftp_content_dir") ||
		strings.Contains(lower, "ftp_plugin_dir") ||
		strings.Contains(lower, "ftp_theme_dir")
}

func (s *Service) patchWPSiteURL(root, domain string, https bool) error {
	target := filepath.Join(root, "wp-config.php")
	b, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	content := ensureWPConfigHealth(string(b))
	scheme := "http"
	if https {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s", scheme, domain)
	content = replaceWPDefine(content, "WP_HOME", url)
	content = replaceWPDefine(content, "WP_SITEURL", url)
	return os.WriteFile(target, []byte(content), 0644)
}

// FixWPConfig repairs duplicate salts and adds proxy HTTPS detection.
func (s *Service) FixWPConfig(root string) error {
	target := filepath.Join(root, "wp-config.php")
	b, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	content := ensureWPConfigHealth(string(b))
	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		return err
	}
	_ = s.ensureWPSiteOwnership(root, nil)
	return s.writeWPFilesystemConfig(root, nil)
}

func ensureWPConfigHealth(content string) string {
	keys := []string{"AUTH_KEY", "SECURE_AUTH_KEY", "LOGGED_IN_KEY", "NONCE_KEY", "AUTH_SALT", "SECURE_AUTH_SALT", "LOGGED_IN_SALT", "NONCE_SALT"}
	kept := map[string]string{}
	for _, k := range keys {
		kept[k] = extractWPDefine(content, k)
	}
	content = stripDefaultWPSalts(content)
	content = ensureWPProxyHTTPS(content)

	var saltLines []string
	for _, k := range keys {
		v := kept[k]
		if v == "" || v == "put your unique phrase here" {
			v = wpRandomString(64)
		}
		saltLines = append(saltLines, fmt.Sprintf("define('%s', '%s');", k, v))
	}
	block := strings.Join(saltLines, "\n") + "\n"
	marker := "/* That's all, stop editing!"
	if idx := strings.Index(content, marker); idx >= 0 {
		content = content[:idx] + block + content[idx:]
	}
	return content
}

func extractWPDefine(content, key string) string {
	re := regexp.MustCompile(`define\s*\(\s*['"]` + regexp.QuoteMeta(key) + `['"]\s*,\s*['"]([^'"]*)['"]\s*\)`)
	matches := re.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1][1]
}

func replaceWPDefine(content, key, value string) string {
	needle := fmt.Sprintf("define('%s'", key)
	if idx := strings.Index(content, needle); idx >= 0 {
		end := strings.Index(content[idx:], ");")
		if end >= 0 {
			return content[:idx] + fmt.Sprintf("define('%s', '%s');", key, value) + content[idx+end+2:]
		}
	}
	insert := fmt.Sprintf("define('%s', '%s');\n", key, value)
	marker := "/* That's all, stop editing!"
	if idx := strings.Index(content, marker); idx >= 0 {
		return content[:idx] + insert + content[idx:]
	}
	return content + "\n" + insert
}
