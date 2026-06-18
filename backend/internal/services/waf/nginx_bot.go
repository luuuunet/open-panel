package waf

import (
	"fmt"
	"strings"
)

// BuildSiteBotBlock returns nginx if directives to block crawlers for a site vhost.
func BuildSiteBotBlock(blocked []CrawlerPreset) string {
	if len(blocked) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n    # OWPanel — per-site bot/crawler control")
	for _, c := range blocked {
		re := CrawlerPatternRegex(c.Patterns)
		if re == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("\n    if ($http_user_agent ~* \"%s\") { return 403; }", re))
	}
	return b.String()
}
