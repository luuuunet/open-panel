package devops

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

type SlowLogEntry struct {
	Source   string  `json:"source"`
	Domain   string  `json:"domain,omitempty"`
	Message  string  `json:"message"`
	Duration float64 `json:"duration_ms,omitempty"`
	At       string  `json:"at,omitempty"`
}

type SlowLogSummary struct {
	Total   int            `json:"total"`
	Entries []SlowLogEntry `json:"entries"`
	BySource map[string]int `json:"by_source"`
}

type TrafficAnomaly struct {
	Domain      string  `json:"domain"`
	CurrentHits int64   `json:"current_hits"`
	PreviousHits int64  `json:"previous_hits"`
	ChangePct   float64 `json:"change_pct"`
	Severity    string  `json:"severity"`
	Hint        string  `json:"hint"`
}

var (
	slowReqRe     = regexp.MustCompile(`request_time[=:]\s*([\d.]+)`)
	upstreamSlowRe = regexp.MustCompile(`upstream timed out|upstream response time`)
)

func (s *Service) SlowLogSummary(limit int) (*SlowLogSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	var entries []SlowLogEntry

	entries = append(entries, s.scanNginxSlowLogs(limit)...)
	entries = append(entries, s.scanMySQLSlowLog(limit)...)
	entries = append(entries, s.scanPHPFMPSlowLog(limit)...)

	if len(entries) > limit {
		entries = entries[:limit]
	}

	bySource := map[string]int{}
	for _, e := range entries {
		bySource[e.Source]++
	}
	return &SlowLogSummary{Total: len(entries), Entries: entries, BySource: bySource}, nil
}

func (s *Service) scanNginxSlowLogs(limit int) []SlowLogEntry {
	logDir := filepath.Join(s.dataDir, "logs")
	entries := []SlowLogEntry{}
	_ = filepath.Walk(logDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, "_access.log") && !strings.HasSuffix(path, "_error.log") {
			return nil
		}
		domain := strings.TrimSuffix(filepath.Base(path), "_access.log")
		domain = strings.TrimSuffix(domain, "_error.log")
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
			if len(lines) > 500 {
				lines = lines[1:]
			}
		}
		for _, line := range lines {
			if m := slowReqRe.FindStringSubmatch(line); len(m) > 1 {
				sec, _ := strconv.ParseFloat(m[1], 64)
				if sec >= 1.0 {
					entries = append(entries, SlowLogEntry{
						Source: "nginx", Domain: domain,
						Message: truncate(line, 200), Duration: sec * 1000,
					})
				}
			} else if upstreamSlowRe.MatchString(line) {
				entries = append(entries, SlowLogEntry{
					Source: "nginx", Domain: domain, Message: truncate(line, 200),
				})
			}
			if len(entries) >= limit {
				return filepath.SkipAll
			}
		}
		return nil
	})
	return entries
}

func (s *Service) scanMySQLSlowLog(limit int) []SlowLogEntry {
	paths := []string{
		filepath.Join(s.dataDir, "mysql", "slow.log"),
		"/var/log/mysql/mysql-slow.log",
		"/var/log/mysqld.log",
	}
	var entries []SlowLogEntry
	for _, p := range paths {
		data, err := tailFile(p, 80)
		if err != nil || data == "" {
			continue
		}
		for _, line := range strings.Split(data, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.Contains(strings.ToLower(line), "query_time") || strings.Contains(line, "Slow query") {
				entries = append(entries, SlowLogEntry{Source: "mysql", Message: truncate(line, 240)})
				if len(entries) >= limit {
					return entries
				}
			}
		}
	}
	return entries
}

func (s *Service) scanPHPFMPSlowLog(limit int) []SlowLogEntry {
	paths := []string{
		filepath.Join(s.dataDir, "logs", "php-fpm-slow.log"),
		"/var/log/php-fpm/slow.log",
		"/var/log/php8.3-fpm.log",
	}
	var entries []SlowLogEntry
	for _, p := range paths {
		data, err := tailFile(p, 60)
		if err != nil || data == "" {
			continue
		}
		for _, line := range strings.Split(data, "\n") {
			if strings.Contains(line, "script_filename") || strings.Contains(line, "pool www") {
				entries = append(entries, SlowLogEntry{Source: "php-fpm", Message: truncate(line, 240)})
				if len(entries) >= limit {
					return entries
				}
			}
		}
	}
	return entries
}

func tailFile(path string, maxLines int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > maxLines {
			lines = lines[1:]
		}
	}
	return strings.Join(lines, "\n"), scanner.Err()
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (s *Service) TrafficAnomalies() ([]TrafficAnomaly, error) {
	now := time.Now()
	curStart := now.Add(-24 * time.Hour)
	prevStart := now.Add(-48 * time.Hour)

	type agg struct{ cur, prev int64 }
	domains := map[string]*agg{}

	var hits []models.TrafficHit
	_ = s.db.Where("created_at >= ?", prevStart).Find(&hits).Error
	for _, h := range hits {
		d := domainFromLogSource(h.LogSource)
		if d == "" {
			continue
		}
		if domains[d] == nil {
			domains[d] = &agg{}
		}
		if !h.CreatedAt.Before(curStart) {
			domains[d].cur++
		} else {
			domains[d].prev++
		}
	}

	var out []TrafficAnomaly
	for domain, a := range domains {
		if a.cur == 0 && a.prev == 0 {
			continue
		}
		change := 0.0
		if a.prev > 0 {
			change = float64(a.cur-a.prev) / float64(a.prev) * 100
		} else if a.cur > 0 {
			change = 100
		}
		sev := "info"
		hint := "流量正常"
		if change >= 200 {
			sev = "high"
			hint = "带宽/请求量异常升高，请检查该域名访问日志与进程"
		} else if change <= -70 && a.prev > 100 {
			sev = "medium"
			hint = "流量显著下降，可能服务不可用或 DNS 变更"
		} else if change >= 80 {
			sev = "medium"
			hint = "流量明显上升，建议查看慢日志与 WAF 拦截"
		}
		if sev != "info" || a.cur > 50 {
			out = append(out, TrafficAnomaly{
				Domain: domain, CurrentHits: a.cur, PreviousHits: a.prev,
				ChangePct: round1(change), Severity: sev, Hint: hint,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return abs(out[i].ChangePct) > abs(out[j].ChangePct)
	})
	if len(out) > 30 {
		out = out[:30]
	}
	return out, nil
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func domainFromLogSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" || source == "demo" {
		return ""
	}
	base := filepath.Base(source)
	base = strings.TrimSuffix(base, "_access.log")
	base = strings.TrimSuffix(base, ".log")
	return strings.ToLower(base)
}
