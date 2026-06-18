package security

import (
	"bufio"
	"os"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/settings"
)

type ScoreFactor struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Score  int    `json:"score"`
	Max    int    `json:"max"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type ScoreReport struct {
	Score   int           `json:"score"`
	Grade   string        `json:"grade"`
	Summary string        `json:"summary"`
	Factors []ScoreFactor `json:"factors"`
}

func (s *Service) ComputeScore() ScoreReport {
	factors := []ScoreFactor{}
	total, maxTotal := 0, 0

	add := func(key, name, status, detail string, score, max int) {
		factors = append(factors, ScoreFactor{Key: key, Name: name, Score: score, Max: max, Status: status, Detail: detail})
		total += score
		maxTotal += max
	}

	items := s.Scan()
	passCount := 0
	for _, it := range items {
		if it.Status == "pass" {
			passCount++
		}
	}
	wafScore := passCount * 40 / max(len(items), 1)
	add("waf_scan", "WAF 安全检测", scoreStatus(wafScore, 40), "", wafScore, 40)

	panelPath := ""
	ipWhitelist := false
	strongPwd := false
	headersOn := true
	if s.settings != nil {
		all, _ := s.settings.GetAll()
		panelPath = strings.Trim(all["panel_safe_path"], "/")
		ipWhitelist = all["panel_ip_whitelist_enabled"] == "true"
		strongPwd = all["password_require_strong"] != "false"
		headersOn = all["panel_security_headers"] != "false"
	}
	if panelPath != "" {
		add("panel_entry", "面板安全入口", "ok", panelPath, 15, 15)
	} else {
		add("panel_entry", "面板安全入口", "warn", "未配置", 0, 15)
	}
	if ipWhitelist {
		add("panel_ip", "面板 IP 白名单", "ok", "已启用", 15, 15)
	} else {
		add("panel_ip", "面板 IP 白名单", "warn", "未启用", 5, 15)
	}
	if strongPwd {
		add("password", "强密码策略", "ok", "已启用", 10, 10)
	} else {
		add("password", "强密码策略", "warn", "仅最小长度", 4, 10)
	}
	if headersOn {
		add("headers", "面板安全 Headers", "ok", "已启用", 10, 10)
	} else {
		add("headers", "面板安全 Headers", "warn", "已关闭", 0, 10)
	}

	sshScore, sshDetail, sshStatus := scoreSSH()
	add("ssh", "SSH 安全", sshStatus, sshDetail, sshScore, 10)

	score := 0
	if maxTotal > 0 {
		score = total * 100 / maxTotal
	}
	grade := gradeFromScore(score)
	summary := scoreSummary(score, passCount, len(items))
	return ScoreReport{Score: score, Grade: grade, Summary: summary, Factors: factors}
}

func scoreStatus(score, max int) string {
	if score >= max*8/10 {
		return "ok"
	}
	if score >= max/2 {
		return "warn"
	}
	return "danger"
}

func gradeFromScore(score int) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 80:
		return "B"
	case score >= 70:
		return "C"
	case score >= 60:
		return "D"
	default:
		return "F"
	}
}

func scoreSummary(score, pass, total int) string {
	if score >= 85 {
		return "面板整体安全状况良好"
	}
	if score >= 65 {
		return "存在可改进项，建议完成 WAF 检测修复并加强面板访问控制"
	}
	return "安全风险较高，请优先修复 WAF 检测项并启用 IP 白名单与强密码"
}

func scoreSSH() (int, string, string) {
	port, rootLogin, passAuth := readSSHConfig()
	detail := "Port " + port
	score := 4
	status := "warn"
	if port != "22" && port != "" {
		score += 3
		detail += " · 非默认端口"
	}
	if !rootLogin {
		score += 2
		detail += " · 禁止 root 登录"
	}
	if !passAuth {
		score += 1
		detail += " · 禁用密码登录"
	}
	if score >= 8 {
		status = "ok"
	}
	if score > 10 {
		score = 10
	}
	return score, detail, status
}

func readSSHConfig() (port string, permitRoot, passwordAuth bool) {
	port = "22"
	permitRoot = true
	passwordAuth = true
	f, err := os.Open("/etc/ssh/sshd_config")
	if err != nil {
		return port, permitRoot, passwordAuth
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		key := strings.ToLower(parts[0])
		val := strings.ToLower(parts[1])
		switch key {
		case "port":
			port = parts[1]
		case "permitrootlogin":
			permitRoot = val == "yes" || val == "without-password"
		case "passwordauthentication":
			passwordAuth = val == "yes"
		}
	}
	return port, permitRoot, passwordAuth
}

func panelEntryStatus(settingsSvc *settings.Service) string {
	if settingsSvc == nil {
		return "warn"
	}
	all, err := settingsSvc.GetAll()
	if err != nil {
		return "warn"
	}
	if strings.Trim(all["panel_safe_path"], "/") != "" {
		return "pass"
	}
	return "warn"
}

func sshPortStatus() string {
	port, _, _ := readSSHConfig()
	if port != "" && port != "22" {
		return "pass"
	}
	return "warn"
}
