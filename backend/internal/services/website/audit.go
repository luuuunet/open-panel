package website

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

type AuditFinding struct {
	Category string `json:"category"`
	Severity string `json:"severity"` // critical, warning, info
	Code     string `json:"code"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	FixHint  string `json:"fix_hint"`
}

type AuditCategoryScore struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Score int    `json:"score"`
	Grade string `json:"grade"`
}

type AuditReport struct {
	SiteID      uint                 `json:"site_id"`
	Domain      string               `json:"domain"`
	URL         string               `json:"url"`
	Status      string               `json:"status"`
	ScannedAt   time.Time            `json:"scanned_at"`
	Score       int                  `json:"score"`
	Grade       string               `json:"grade"`
	HTTPStatus  int                  `json:"http_status"`
	LatencyMs   int                  `json:"latency_ms"`
	PageSize    int                  `json:"page_size_bytes"`
	HTTPS       bool                 `json:"https"`
	SSLDaysLeft int                  `json:"ssl_days_left,omitempty"`
	Categories  []AuditCategoryScore `json:"categories"`
	Findings    []AuditFinding       `json:"findings"`
	Critical    int                  `json:"critical"`
	Warning     int                  `json:"warning"`
	BrokenLinks int                  `json:"broken_links"`
}

func (s *Service) AuditSite(siteID uint) (*AuditReport, error) {
	site, err := s.Get(siteID)
	if err != nil {
		return nil, err
	}
	report := &AuditReport{
		SiteID:    siteID,
		Domain:    site.Domain,
		Status:    site.Status,
		ScannedAt: time.Now(),
	}
	targetURL := buildSiteURL(site)
	report.URL = targetURL
	report.HTTPS = strings.HasPrefix(strings.ToLower(targetURL), "https://")

	findings := make([]AuditFinding, 0, 24)
	httpScore, httpFindings, body, headers, latency, statusCode := probeHTTP(targetURL)
	findings = append(findings, httpFindings...)
	report.HTTPStatus = statusCode
	report.LatencyMs = latency
	report.PageSize = len(body)

	sslScore, sslFindings, daysLeft := checkSSL(targetURL, headers)
	findings = append(findings, sslFindings...)
	report.SSLDaysLeft = daysLeft

	secScore, secFindings := checkSecurityHeaders(headers, report.HTTPS)
	findings = append(findings, secFindings...)

	seoScore, seoFindings := checkSEO(body)
	findings = append(findings, seoFindings...)

	perfScore, perfFindings := checkPerformance(latency, len(body), body)
	findings = append(findings, perfFindings...)

	linkScore, linkFindings, broken := checkBrokenLinks(targetURL, body)
	findings = append(findings, linkFindings...)
	report.BrokenLinks = broken

	panelScore := 100
	if diag, err := s.CollectDiagnostics(siteID); err == nil && diag != nil {
		panelFindings, ps := panelDiagnosticsFindings(diag)
		findings = append(findings, panelFindings...)
		panelScore = ps
	}

	report.Categories = []AuditCategoryScore{
		{Key: "http", Label: "HTTP", Score: httpScore, Grade: scoreGrade(httpScore)},
		{Key: "ssl", Label: "SSL", Score: sslScore, Grade: scoreGrade(sslScore)},
		{Key: "security", Label: "Security", Score: secScore, Grade: scoreGrade(secScore)},
		{Key: "seo", Label: "SEO", Score: seoScore, Grade: scoreGrade(seoScore)},
		{Key: "performance", Label: "Performance", Score: perfScore, Grade: scoreGrade(perfScore)},
		{Key: "links", Label: "Links", Score: linkScore, Grade: scoreGrade(linkScore)},
		{Key: "panel", Label: "Panel", Score: panelScore, Grade: scoreGrade(panelScore)},
	}

	report.Findings = prioritizeFindings(findings)
	for _, f := range report.Findings {
		switch f.Severity {
		case "critical":
			report.Critical++
		case "warning":
			report.Warning++
		}
	}

	overall := int(float64(httpScore)*0.18 + float64(sslScore)*0.18 + float64(secScore)*0.18 +
		float64(seoScore)*0.14 + float64(perfScore)*0.14 + float64(linkScore)*0.10 + float64(panelScore)*0.08)
	if overall > 100 {
		overall = 100
	}
	if overall < 0 {
		overall = 0
	}
	report.Score = overall
	report.Grade = scoreGrade(overall)
	return report, nil
}

func buildSiteURL(site *models.Website) string {
	scheme := "http"
	port := site.Port
	if site.SSL || site.ForceHTTPS {
		scheme = "https"
	}
	if port == 0 {
		if scheme == "https" {
			port = 443
		} else {
			port = 80
		}
	}
	if (scheme == "http" && port == 80) || (scheme == "https" && port == 443) {
		return fmt.Sprintf("%s://%s", scheme, site.Domain)
	}
	return fmt.Sprintf("%s://%s:%d", scheme, site.Domain, port)
}

func probeHTTP(target string) (score int, findings []AuditFinding, body []byte, headers http.Header, latencyMs, status int) {
	score = 100
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 8 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	start := time.Now()
	resp, err := client.Get(target)
	latencyMs = int(time.Since(start).Milliseconds())
	if err != nil {
		findings = append(findings, AuditFinding{
			Category: "http", Severity: "critical", Code: "http_unreachable",
			Title: "Site unreachable", Detail: err.Error(),
			FixHint: "Check DNS, nginx vhost, firewall, and that the site is running.",
		})
		return 0, findings, nil, nil, latencyMs, 0
	}
	defer resp.Body.Close()
	status = resp.StatusCode
	headers = resp.Header
	body, _ = io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))

	switch {
	case status >= 500:
		score -= 60
		findings = append(findings, AuditFinding{
			Category: "http", Severity: "critical", Code: "http_5xx",
			Title: "Server error", Detail: fmt.Sprintf("HTTP %d", status),
			FixHint: "Check nginx error log and application logs.",
		})
	case status >= 400:
		score -= 40
		findings = append(findings, AuditFinding{
			Category: "http", Severity: "warning", Code: "http_4xx",
			Title: "Client error response", Detail: fmt.Sprintf("HTTP %d", status),
			FixHint: "Verify index files, rewrite rules, and site root path.",
		})
	case status >= 300:
		score -= 10
		findings = append(findings, AuditFinding{
			Category: "http", Severity: "info", Code: "http_redirect",
			Title: "Redirect response", Detail: fmt.Sprintf("HTTP %d", status),
			FixHint: "Ensure final URL returns 200 for monitoring.",
		})
	}
	if score < 0 {
		score = 0
	}
	return score, findings, body, headers, latencyMs, status
}

func checkSSL(target string, headers http.Header) (score int, findings []AuditFinding, daysLeft int) {
	score = 100
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "https" {
		findings = append(findings, AuditFinding{
			Category: "ssl", Severity: "warning", Code: "no_https",
			Title: "Not using HTTPS", Detail: "Site fetched over plain HTTP",
			FixHint: "Enable SSL certificate and force HTTPS in site settings.",
		})
		return 40, findings, 0
	}
	host := u.Hostname()
	dialer := net.Dialer{Timeout: 12 * time.Second}
	conn, err := tls.DialWithDialer(&dialer, "tcp", u.Host, &tls.Config{ServerName: host})
	if err != nil {
		score -= 50
		findings = append(findings, AuditFinding{
			Category: "ssl", Severity: "critical", Code: "ssl_handshake_fail",
			Title: "TLS handshake failed", Detail: err.Error(),
			FixHint: "Renew or reinstall SSL certificate.",
		})
		return score, findings, 0
	}
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return 30, append(findings, AuditFinding{
			Category: "ssl", Severity: "critical", Code: "ssl_no_cert",
			Title: "No certificate", Detail: "TLS connected but no peer certificate",
			FixHint: "Install a valid SSL certificate.",
		}), 0
	}
	expiry := certs[0].NotAfter
	daysLeft = int(time.Until(expiry).Hours() / 24)
	switch {
	case daysLeft < 0:
		score = 0
		findings = append(findings, AuditFinding{
			Category: "ssl", Severity: "critical", Code: "ssl_expired",
			Title: "Certificate expired", Detail: expiry.Format("2006-01-02"),
			FixHint: "Renew SSL immediately in SSL menu or enable auto-renew.",
		})
	case daysLeft <= 14:
		score -= 30
		findings = append(findings, AuditFinding{
			Category: "ssl", Severity: "warning", Code: "ssl_expiring_soon",
			Title: "Certificate expiring soon", Detail: fmt.Sprintf("%d days left", daysLeft),
			FixHint: "Renew certificate before expiry.",
		})
	case daysLeft <= 30:
		score -= 10
		findings = append(findings, AuditFinding{
			Category: "ssl", Severity: "info", Code: "ssl_expiring",
			Title: "Certificate expiry approaching", Detail: fmt.Sprintf("%d days left", daysLeft),
			FixHint: "Schedule SSL renewal.",
		})
	}
	if headers.Get("Strict-Transport-Security") == "" {
		score -= 15
		findings = append(findings, AuditFinding{
			Category: "ssl", Severity: "warning", Code: "missing_hsts",
			Title: "Missing HSTS header", Detail: "Strict-Transport-Security not set",
			FixHint: "Add HSTS in nginx/OpenResty security headers.",
		})
	}
	if score < 0 {
		score = 0
	}
	return score, findings, daysLeft
}

func checkSecurityHeaders(headers http.Header, https bool) (score int, findings []AuditFinding) {
	score = 100
	checks := []struct {
		header   string
		code     string
		title    string
		severity string
		penalty  int
		httpsOnly bool
	}{
		{"Content-Security-Policy", "missing_csp", "Missing CSP", "warning", 12, false},
		{"X-Frame-Options", "missing_xfo", "Missing X-Frame-Options", "warning", 10, false},
		{"X-Content-Type-Options", "missing_xcto", "Missing X-Content-Type-Options", "warning", 10, false},
		{"Referrer-Policy", "missing_referrer", "Missing Referrer-Policy", "info", 5, false},
		{"Permissions-Policy", "missing_permissions", "Missing Permissions-Policy", "info", 5, false},
	}
	for _, c := range checks {
		if c.httpsOnly && !https {
			continue
		}
		if headers.Get(c.header) == "" {
			score -= c.penalty
			findings = append(findings, AuditFinding{
				Category: "security", Severity: c.severity, Code: c.code,
				Title: c.title, Detail: c.header + " header not present",
				FixHint: "Configure security headers in nginx or WAF.",
			})
		}
	}
	if headers.Get("Server") != "" && !strings.Contains(strings.ToLower(headers.Get("Server")), "cloudflare") {
		findings = append(findings, AuditFinding{
			Category: "security", Severity: "info", Code: "server_header",
			Title: "Server header exposed", Detail: headers.Get("Server"),
			FixHint: "Hide server version in web server config.",
		})
		score -= 3
	}
	if headers.Get("X-Powered-By") != "" {
		score -= 8
		findings = append(findings, AuditFinding{
			Category: "security", Severity: "warning", Code: "powered_by",
			Title: "X-Powered-By exposed", Detail: headers.Get("X-Powered-By"),
			FixHint: "Disable X-Powered-By in PHP/nginx.",
		})
	}
	if score < 0 {
		score = 0
	}
	return score, findings
}

var (
	reTitle       = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reMetaDesc    = regexp.MustCompile(`(?is)<meta[^>]+name=["']description["'][^>]+content=["'](.*?)["']`)
	reMetaDescAlt = regexp.MustCompile(`(?is)<meta[^>]+content=["'](.*?)["'][^>]+name=["']description["']`)
	reH1          = regexp.MustCompile(`(?is)<h1[^>]*>`)
	reCanonical   = regexp.MustCompile(`(?is)<link[^>]+rel=["']canonical["']`)
	reViewport    = regexp.MustCompile(`(?is)<meta[^>]+name=["']viewport["']`)
	reLang        = regexp.MustCompile(`(?is)<html[^>]+lang=["']([^"']+)["']`)
	reOG          = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:title["']`)
	reHref        = regexp.MustCompile(`(?is)href=["']([^"'#]+)["']`)
)

func checkSEO(body []byte) (score int, findings []AuditFinding) {
	score = 100
	html := string(body)
	if len(html) < 50 {
		findings = append(findings, AuditFinding{
			Category: "seo", Severity: "warning", Code: "empty_body",
			Title: "Empty or tiny response body", Detail: "Cannot analyze SEO metadata",
			FixHint: "Ensure homepage returns HTML content.",
		})
		return 30, findings
	}
	title := extractGroup(reTitle, html)
	switch {
	case title == "":
		score -= 25
		findings = append(findings, AuditFinding{
			Category: "seo", Severity: "critical", Code: "missing_title",
			Title: "Missing page title", Detail: "No <title> tag found",
			FixHint: "Add a unique title (50–60 characters).",
		})
	case len(title) < 10:
		score -= 10
		findings = append(findings, AuditFinding{
			Category: "seo", Severity: "warning", Code: "short_title",
			Title: "Title too short", Detail: fmt.Sprintf("%d chars", len(title)),
			FixHint: "Use a descriptive title around 50–60 characters.",
		})
	case len(title) > 70:
		score -= 5
		findings = append(findings, AuditFinding{
			Category: "seo", Severity: "info", Code: "long_title",
			Title: "Title may be truncated", Detail: fmt.Sprintf("%d chars", len(title)),
			FixHint: "Shorten title to ~60 characters.",
		})
	}
	desc := extractGroup(reMetaDesc, html)
	if desc == "" {
		desc = extractGroup(reMetaDescAlt, html)
	}
	if desc == "" {
		score -= 15
		findings = append(findings, AuditFinding{
			Category: "seo", Severity: "warning", Code: "missing_description",
			Title: "Missing meta description", Detail: "No meta description tag",
			FixHint: "Add meta description (150–160 characters).",
		})
	}
	if len(reH1.FindAllString(html, -1)) == 0 {
		score -= 12
		findings = append(findings, AuditFinding{
			Category: "seo", Severity: "warning", Code: "missing_h1",
			Title: "Missing H1 heading", Detail: "No <h1> found on page",
			FixHint: "Add one clear H1 per page.",
		})
	}
	if !reCanonical.MatchString(html) {
		score -= 8
		findings = append(findings, AuditFinding{
			Category: "seo", Severity: "info", Code: "missing_canonical",
			Title: "No canonical URL", Detail: "link rel=canonical not found",
			FixHint: "Set canonical URL to avoid duplicate content.",
		})
	}
	if !reViewport.MatchString(html) {
		score -= 10
		findings = append(findings, AuditFinding{
			Category: "seo", Severity: "warning", Code: "missing_viewport",
			Title: "Missing viewport meta", Detail: "Mobile-friendly signal missing",
			FixHint: "Add <meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">.",
		})
	}
	if !reLang.MatchString(html) {
		score -= 5
		findings = append(findings, AuditFinding{
			Category: "seo", Severity: "info", Code: "missing_lang",
			Title: "Missing html lang", Detail: "No lang attribute on <html>",
			FixHint: "Set lang attribute for accessibility and SEO.",
		})
	}
	if !reOG.MatchString(html) {
		score -= 5
		findings = append(findings, AuditFinding{
			Category: "seo", Severity: "info", Code: "missing_og",
			Title: "Missing Open Graph tags", Detail: "og:title not found",
			FixHint: "Add Open Graph meta for social sharing.",
		})
	}
	if score < 0 {
		score = 0
	}
	return score, findings
}

func checkPerformance(latencyMs, pageSize int, body []byte) (score int, findings []AuditFinding) {
	score = 100
	switch {
	case latencyMs > 3000:
		score -= 40
		findings = append(findings, AuditFinding{
			Category: "performance", Severity: "critical", Code: "slow_ttfb",
			Title: "Very slow response", Detail: fmt.Sprintf("%d ms TTFB", latencyMs),
			FixHint: "Enable CDN cache, optimize PHP/DB, check server load.",
		})
	case latencyMs > 1500:
		score -= 20
		findings = append(findings, AuditFinding{
			Category: "performance", Severity: "warning", Code: "moderate_ttfb",
			Title: "Slow response time", Detail: fmt.Sprintf("%d ms", latencyMs),
			FixHint: "Target TTFB under 600ms with caching and optimization.",
		})
	case latencyMs > 800:
		score -= 8
		findings = append(findings, AuditFinding{
			Category: "performance", Severity: "info", Code: "ok_ttfb",
			Title: "Response time acceptable", Detail: fmt.Sprintf("%d ms", latencyMs),
			FixHint: "Consider CDN and page cache for sub-600ms TTFB.",
		})
	}
	if pageSize > 3*1024*1024 {
		score -= 25
		findings = append(findings, AuditFinding{
			Category: "performance", Severity: "warning", Code: "large_page",
			Title: "Large page size", Detail: fmt.Sprintf("%.1f MB", float64(pageSize)/1024/1024),
			FixHint: "Compress images, minify assets, enable gzip/brotli.",
		})
	} else if pageSize > 1024*1024 {
		score -= 10
		findings = append(findings, AuditFinding{
			Category: "performance", Severity: "info", Code: "heavy_page",
			Title: "Page size above 1 MB", Detail: fmt.Sprintf("%.1f MB", float64(pageSize)/1024/1024),
			FixHint: "Optimize images and lazy-load below-the-fold content.",
		})
	}
	html := string(body)
	if strings.Count(strings.ToLower(html), "<script") > 25 {
		score -= 8
		findings = append(findings, AuditFinding{
			Category: "performance", Severity: "info", Code: "many_scripts",
			Title: "Many script tags", Detail: "Consider bundling and deferring JS",
			FixHint: "Reduce render-blocking scripts.",
		})
	}
	if score < 0 {
		score = 0
	}
	return score, findings
}

func checkBrokenLinks(baseURL string, body []byte) (score int, findings []AuditFinding, broken int) {
	score = 100
	if len(body) == 0 {
		return 100, nil, 0
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return 100, nil, 0
	}
	seen := map[string]struct{}{}
	candidates := make([]string, 0, 16)
	for _, m := range reHref.FindAllStringSubmatch(string(body), -1) {
		if len(m) < 2 {
			continue
		}
		href := strings.TrimSpace(m[1])
		if href == "" || strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") {
			continue
		}
		abs, err := base.Parse(href)
		if err != nil || abs.Scheme != "http" && abs.Scheme != "https" {
			continue
		}
		if abs.Host != base.Host {
			continue
		}
		u := abs.String()
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		candidates = append(candidates, u)
		if len(candidates) >= 12 {
			break
		}
	}
	if len(candidates) == 0 {
		return score, findings, 0
	}
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(r *http.Request, v []*http.Request) error {
		if len(v) >= 5 {
			return fmt.Errorf("redirect")
		}
		return nil
	}}
	for _, link := range candidates {
		resp, err := client.Head(link)
		if err != nil {
			resp, err = client.Get(link)
		}
		if err != nil {
			broken++
			findings = append(findings, AuditFinding{
				Category: "links", Severity: "warning", Code: "link_unreachable",
				Title: "Broken link", Detail: link + ": " + err.Error(),
				FixHint: "Fix or remove dead internal links.",
			})
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			broken++
			findings = append(findings, AuditFinding{
				Category: "links", Severity: "warning", Code: "link_error",
				Title: "Link returns error", Detail: fmt.Sprintf("%s → HTTP %d", link, resp.StatusCode),
				FixHint: "Update or redirect broken URLs.",
			})
		}
	}
	if broken > 0 {
		score -= broken * 8
	}
	if score < 0 {
		score = 0
	}
	return score, findings, broken
}

func panelDiagnosticsFindings(d *SiteDiagnosticBundle) ([]AuditFinding, int) {
	score := 100
	findings := make([]AuditFinding, 0, len(d.Issues)+4)
	severity := func(msg string) string {
		if strings.Contains(msg, "停止") || strings.Contains(msg, "未运行") || strings.Contains(msg, "不存在") {
			return "critical"
		}
		return "warning"
	}
	for _, issue := range d.Issues {
		sev := severity(issue)
		penalty := 15
		if sev == "critical" {
			penalty = 25
		}
		score -= penalty
		findings = append(findings, AuditFinding{
			Category: "panel", Severity: sev, Code: "panel_issue",
			Title: "Panel diagnostic", Detail: issue,
			FixHint: "Fix in Websites → site settings or use AI repair.",
		})
	}
	if !d.RootExists {
		score -= 20
	}
	if !d.WebServerRunning {
		score -= 30
	}
	if score < 0 {
		score = 0
	}
	return findings, score
}

func prioritizeFindings(in []AuditFinding) []AuditFinding {
	order := map[string]int{"critical": 0, "warning": 1, "info": 2}
	out := make([]AuditFinding, len(in))
	copy(out, in)
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if order[out[j].Severity] < order[out[i].Severity] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

func scoreGrade(score int) string {
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

func extractGroup(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(stripTags(m[1]))
}

func stripTags(s string) string {
	return regexp.MustCompile(`(?s)<[^>]+>`).ReplaceAllString(s, "")
}
