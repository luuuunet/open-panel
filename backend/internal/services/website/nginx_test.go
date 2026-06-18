package website

import (
	"strings"
	"testing"

	"github.com/luuuunet/owpanel/internal/models"
)

func TestRenderServerBlockAccessLogPaths(t *testing.T) {
	s := &Service{dataDir: "/opt/owpanel/data"}
	site := &models.Website{
		Domain:     "example.com",
		RootPath:   "/opt/owpanel/data/wwwroot/example.com/public",
		Port:       80,
		PHP:        true,
		PhpVersion: "8.4",
	}
	features := &nginxFeatureBlocks{}
	block, err := s.renderServerBlock(site, site.RootPath, 80, []string{site.Domain}, sslOpts{}, features)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(block, "access_log ;") {
		t.Fatalf("malformed access_log in vhost:\n%s", block)
	}
	if strings.Contains(block, "%!(EXTRA") {
		t.Fatalf("fmt placeholder mismatch in vhost:\n%s", block)
	}
	wantAccess := "/opt/owpanel/data/logs/example.com_access.log"
	wantError := "/opt/owpanel/data/logs/example.com_error.log"
	if !strings.Contains(block, "access_log "+wantAccess+";") {
		t.Fatalf("missing access log path, got:\n%s", block)
	}
	if !strings.Contains(block, "error_log "+wantError+";") {
		t.Fatalf("missing error log path, got:\n%s", block)
	}
}
