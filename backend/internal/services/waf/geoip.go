package waf

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

type GeoIPStatus struct {
	Enabled        bool     `json:"enabled"`
	Mode           string   `json:"mode"`
	Countries      []string `json:"countries"`
	DBExists       bool     `json:"db_exists"`
	DBPath         string   `json:"db_path"`
	DBSize         int64    `json:"db_size"`
	NginxGeoIP2    bool     `json:"nginx_geoip2_hint"`
	SetupHint      string   `json:"setup_hint"`
}

func (s *Service) GeoDBPath(cfg *models.SecurityConfig) string {
	if cfg != nil && strings.TrimSpace(cfg.GeoDbPath) != "" {
		return cfg.GeoDbPath
	}
	return filepath.Join(s.confDir, "GeoLite2-Country.mmdb")
}

func (s *Service) GeoIPStatus() (*GeoIPStatus, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	dbPath := s.GeoDBPath(cfg)
	var dbSize int64
	dbExists := false
	if st, err := os.Stat(dbPath); err == nil {
		dbExists = true
		dbSize = st.Size()
	}

	countries := parseCountryCodes(cfg.BlockedCountries)
	mode := cfg.GeoMode
	if mode == "" {
		mode = "block"
	}

	hint := "请将 MaxMind GeoLite2-Country.mmdb 放到: " + dbPath +
		"（需 Nginx 编译 ngx_http_geoip2_module 模块）。免费下载: https://dev.maxmind.com/geoip/geolite2-free-geolocation-data"

	return &GeoIPStatus{
		Enabled:     cfg.GeoBlockEnabled,
		Mode:        mode,
		Countries:   countries,
		DBExists:    dbExists,
		DBPath:      dbPath,
		DBSize:      dbSize,
		NginxGeoIP2: true,
		SetupHint:   hint,
	}, nil
}

func parseCountryCodes(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		cc := strings.TrimSpace(strings.ToUpper(part))
		if cc != "" {
			out = append(out, cc)
		}
	}
	return out
}

func joinCountryCodes(codes []string) string {
	return strings.Join(codes, ",")
}
