package api

import (
	"fmt"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) safePathBase() string {
	all, err := s.settings.GetAll()
	if err != nil {
		return "/"
	}
	sp := strings.Trim(strings.TrimSpace(all["panel_safe_path"]), "/")
	if sp == "" {
		return "/"
	}
	return "/" + sp + "/"
}

func (s *Server) serveIndexHTML(c *gin.Context) {
	path := s.cfg.WebDir + "/index.html"
	content, err := os.ReadFile(path)
	if err != nil {
		c.File(path)
		return
	}
	base := s.safePathBase()
	baseTag := `<base href="/">`
	if base != "/" {
		baseTag = fmt.Sprintf(`<base href=%q>`, base)
	}
	inject := fmt.Sprintf(`%s<script>window.__OWPANEL_BASE__=%q;</script>`, baseTag, base)
	html := strings.Replace(string(content), "<head>", "<head>"+inject, 1)
	if base != "/" {
		// Absolute asset URLs so nested routes (/software/, /dashboard/) always load JS/CSS correctly.
		html = strings.ReplaceAll(html, `src="./assets/`, `src="`+base+`assets/`)
		html = strings.ReplaceAll(html, `href="./assets/`, `href="`+base+`assets/`)
		html = strings.ReplaceAll(html, `href="./favicon.svg"`, `href="`+base+`favicon.svg"`)
		html = strings.ReplaceAll(html, `href="./logo.png"`, `href="`+base+`logo.png"`)
		html = strings.ReplaceAll(html, `href="./logo.svg"`, `href="`+base+`logo.svg"`)
		// Legacy absolute paths from older builds
		html = strings.ReplaceAll(html, `src="/assets/`, `src="`+strings.TrimSuffix(base, "/")+`/assets/`)
		html = strings.ReplaceAll(html, `href="/assets/`, `href="`+strings.TrimSuffix(base, "/")+`/assets/`)
		html = strings.ReplaceAll(html, `href="/favicon.svg"`, `href="`+strings.TrimSuffix(base, "/")+`/favicon.svg"`)
	}
	c.Data(200, "text/html; charset=utf-8", []byte(html))
}

func looksLikeStaticAsset(path string) bool {
	if strings.Contains(path, "/assets/") || strings.Contains(path, "/software-icons/") || strings.Contains(path, "/geo/") {
		return true
	}
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".js") ||
		strings.HasSuffix(lower, ".css") ||
		strings.HasSuffix(lower, ".svg") ||
		strings.HasSuffix(lower, ".map") ||
		strings.HasSuffix(lower, ".woff2") ||
		strings.HasSuffix(lower, ".woff") ||
		strings.HasSuffix(lower, ".png") ||
		strings.HasSuffix(lower, ".jpg") ||
		strings.HasSuffix(lower, ".json")
}
