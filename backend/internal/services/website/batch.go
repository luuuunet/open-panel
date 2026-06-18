package website

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Pipe-delimited batch line: 域名|文档根目录|FTP|数据库|PHP版本
// Example: example.com,www.example.com:8081|/opt/owpanel/data/wwwroot/example.com|1|1|83
type pipeBatchLine struct {
	lineNum    int
	raw        string
	domainsRaw string
	rootPath   string
	ftp        bool
	database   bool
	phpVersion string
}

func parsePipeBatchPHP(code string) string {
	code = strings.TrimSpace(code)
	if code == "" || code == "0" {
		return "static"
	}
	if strings.Contains(code, ".") {
		return code
	}
	if len(code) >= 2 && code[0] >= '0' && code[0] <= '9' {
		return code[:1] + "." + code[1:]
	}
	return code
}

func parsePipeBatchText(text string) ([]pipeBatchLine, error) {
	var lines []pipeBatchLine
	for i, raw := range strings.Split(text, "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		parts := strings.Split(raw, "|")
		if len(parts) < 5 {
			return nil, fmt.Errorf("第 %d 行格式错误，应为：域名|根目录|FTP|数据库|PHP版本", i+1)
		}
		lines = append(lines, pipeBatchLine{
			lineNum:    i + 1,
			raw:        raw,
			domainsRaw: strings.TrimSpace(parts[0]),
			rootPath:   strings.TrimSpace(parts[1]),
			ftp:        strings.TrimSpace(parts[2]) == "1",
			database:   strings.TrimSpace(parts[3]) == "1",
			phpVersion: parsePipeBatchPHP(parts[4]),
		})
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("请输入批量建站数据")
	}
	return lines, nil
}

func isPipeBatchFormat(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return strings.Contains(line, "|")
	}
	return false
}

func domainsTextFromCommaList(raw string) string {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '，' })
	var lines []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			lines = append(lines, p)
		}
	}
	return strings.Join(lines, "\n")
}

func (line pipeBatchLine) toCreateRequest(defaults CreateRequest, defaultRoot string) (*CreateRequest, error) {
	if line.domainsRaw == "" {
		return nil, fmt.Errorf("第 %d 行：域名不能为空", line.lineNum)
	}
	req := defaults
	req.DomainsText = domainsTextFromCommaList(line.domainsRaw)
	if req.DomainsText == "" {
		return nil, fmt.Errorf("第 %d 行：域名格式无效", line.lineNum)
	}
	if line.rootPath != "" {
		req.RootPath = line.rootPath
	} else {
		primary := parseDomainLine(strings.Split(line.domainsRaw, ",")[0]).Host
		if primary != "" && defaultRoot != "" {
			req.RootPath = filepath.Join(defaultRoot, primary)
		}
	}
	if line.ftp {
		req.Ftp = "create"
	} else {
		req.Ftp = "none"
	}
	if line.database {
		req.Database = "mysql"
	} else {
		req.Database = "none"
	}
	req.PhpVersion = line.phpVersion
	return &req, nil
}
