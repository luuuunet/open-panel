package logs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseIniErrorLog(t *testing.T) {
	dir := t.TempDir()
	ini := filepath.Join(dir, "php.ini")
	custom := filepath.Join(dir, "custom-php.log")
	if err := os.WriteFile(ini, []byte(`
;error_log = syslog
error_log = "`+filepath.ToSlash(custom)+`"
`), 0644); err != nil {
		t.Fatal(err)
	}
	got := parseIniErrorLog(ini)
	if filepath.Clean(got) != filepath.Clean(custom) {
		t.Fatalf("got %q want %q", got, custom)
	}
}

func TestPHPFPMServiceLogPaths(t *testing.T) {
	paths := phpFPMServiceLogPaths("8.3")
	if len(paths) == 0 {
		t.Fatal("expected paths")
	}
	if filepath.ToSlash(paths[0]) != "/var/log/php8.3-fpm.log" {
		t.Fatalf("got %q", paths[0])
	}
}

func TestLogCandidatesForPHP(t *testing.T) {
	candidates := logCandidatesForPHP("php83", filepath.Join("/opt/owpanel/data/server/php83"), "/opt/owpanel/data")
	bySuffix := map[string]string{}
	for _, c := range candidates {
		bySuffix[c.suffix] = c.path
	}
	for _, suffix := range []string{"php_cgi", "php_fpm", "php_error"} {
		if bySuffix[suffix] == "" {
			t.Fatalf("missing %s in %+v", suffix, bySuffix)
		}
	}
	if filepath.ToSlash(bySuffix["php_fpm"]) != "/var/log/php8.3-fpm.log" {
		t.Fatalf("php_fpm path = %q", bySuffix["php_fpm"])
	}
	if filepath.ToSlash(bySuffix["php_error"]) != "/var/log/php8.3-fpm.log" &&
		!strings.Contains(filepath.ToSlash(bySuffix["php_error"]), "php8.3-fpm") {
		t.Fatalf("php_error path = %q", bySuffix["php_error"])
	}
}

func TestFirstExistingPrefersExistingFile(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "a.log")
	missing := filepath.Join(dir, "b.log")
	if err := os.WriteFile(existing, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	got := firstExisting(missing, existing)
	if got != existing {
		t.Fatalf("got %q want %q", got, existing)
	}
}
