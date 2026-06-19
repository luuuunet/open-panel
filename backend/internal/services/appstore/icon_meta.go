package appstore

import (
	"fmt"
	"os"
	"strings"
)

type IconMeta struct {
	BG    string
	Label string
}

var iconMetaOverrides = map[string]IconMeta{
	"nginx": {BG: "#009639", Label: "N"},
	"openresty": {BG: "#009639", Label: "OR"},
	"apache": {BG: "#D22128", Label: "A"},
	"openlitespeed": {BG: "#0066CC", Label: "LS"},
	"mysql": {BG: "#00758F", Label: "My"},
	"mariadb": {BG: "#C0765A", Label: "Ma"},
	"postgresql": {BG: "#336791", Label: "Pg"},
	"redis": {BG: "#DC382D", Label: "R"},
	"mongodb": {BG: "#47A248", Label: "Mg"},
	"php83": {BG: "#777BB4", Label: "8.3"},
	"php82": {BG: "#777BB4", Label: "8.2"},
	"php81": {BG: "#777BB4", Label: "8.1"},
	"php74": {BG: "#777BB4", Label: "7.4"},
	"nodejs20": {BG: "#339933", Label: "20"},
	"nodejs18": {BG: "#339933", Label: "18"},
	"python": {BG: "#3776AB", Label: "Py"},
	"dotnet10": {BG: "#512BD4", Label: "10"},
	"dotnet8": {BG: "#512BD4", Label: "8"},
	"java21": {BG: "#5382A1", Label: "21"},
	"java17": {BG: "#5382A1", Label: "17"},
	"java11": {BG: "#5382A1", Label: "11"},
	"java8": {BG: "#5382A1", Label: "8"},
	"pureftpd": {BG: "#F5921E", Label: "FTP"},
	"mail-server": {BG: "#004499", Label: "Mail"},
	"phpmyadmin": {BG: "#F5921E", Label: "PMA"},
	"memcached": {BG: "#5A5A5A", Label: "MC"},
	"docker": {BG: "#2496ED", Label: "D"},
	"fail2ban": {BG: "#E74C3C", Label: "F2"},
	"supervisor": {BG: "#4A90D9", Label: "SV"},
	"pm2": {BG: "#2B037D", Label: "PM"},
	"composer": {BG: "#885630", Label: "Cp"},
	"certbot": {BG: "#2E8540", Label: "SSL"},
	"tomcat9": {BG: "#F8DC75", Label: "T9"},
	"tomcat10": {BG: "#F8DC75", Label: "T10"},
	"tomcat": {BG: "#F8DC75", Label: "Tc"},
	"ollama": {BG: "#1a1a2e", Label: "Ol"},
	"open-webui": {BG: "#2563eb", Label: "UI"},
	"localai": {BG: "#10b981", Label: "LA"},
	"dify": {BG: "#6366f1", Label: "Df"},
	"jupyter": {BG: "#F37726", Label: "Ju"},
	"jupyter-notebook": {BG: "#F37726", Label: "Ju"},
	"vllm": {BG: "#7c3aed", Label: "vL"},
	"comfyui": {BG: "#0ea5e9", Label: "CU"},
	"sd-webui": {BG: "#a855f7", Label: "SD"},
	"anythingllm": {BG: "#334155", Label: "AL"},
	"fastgpt": {BG: "#059669", Label: "FG"},
	"whisper": {BG: "#412991", Label: "Wh"},
	"huggingface-ai": {BG: "#FFD21E", Label: "HF"},
	"chatchat": {BG: "#0284c7", Label: "CC"},
	"wordpress": {BG: "#21759B", Label: "WP"},
	"gitea": {BG: "#609926", Label: "Gt"},
	"gitlab": {BG: "#FC6D26", Label: "GL"},
	"grafana": {BG: "#F46800", Label: "Gr"},
	"openpanel-analytics": {BG: "#6366F1", Label: "OP"},
	"prometheus": {BG: "#E6522C", Label: "Pr"},
	"elasticsearch": {BG: "#005571", Label: "ES"},
	"minio": {BG: "#C72C48", Label: "Mi"},
	"portainer": {BG: "#13BEF9", Label: "Pt"},
	"jenkins": {BG: "#D33833", Label: "Jk"},
	"nextcloud": {BG: "#0082C9", Label: "NC"},
	"jellyfin": {BG: "#AA5CC3", Label: "Jf"},
	"home-assistant": {BG: "#18BCF2", Label: "HA"},
	"keycloak": {BG: "#4D4D4D", Label: "KC"},
	"kafka": {BG: "#231F20", Label: "Kf"},
	"k3s": {BG: "#FFC107", Label: "K3"},
	"cilium": {BG: "#6376DD", Label: "Ci"},
	"rabbitmq": {BG: "#FF6600", Label: "Rb"},
	"traefik": {BG: "#24A1C1", Label: "Tr"},
	"caddy": {BG: "#1F88C0", Label: "Cd"},
	"n8n": {BG: "#EA4B71", Label: "n8"},
	"vaultwarden": {BG: "#175DDC", Label: "VW"},
	"mattermost": {BG: "#0058CC", Label: "MM"},
	"clickhouse": {BG: "#FFCC01", Label: "CH"},
	"influxdb": {BG: "#22ADF6", Label: "If"},
	"ghost": {BG: "#15171A", Label: "Gh"},
	"discourse": {BG: "#000000", Label: "Dc"},
	"maxkb": {BG: "#3370FF", Label: "MK"},
}

func CatalogKeys() []string {
	items := mergedCatalog()
	keys := make([]string, len(items))
	for i, item := range items {
		keys[i] = item.Key
	}
	return keys
}

func IconMetaForKey(key string) IconMeta {
	if m, ok := iconMetaOverrides[key]; ok {
		return m
	}
	return IconMeta{BG: hashIconColor(key), Label: iconLabelForKey(key)}
}

func iconLabelForKey(key string) string {
	if strings.HasPrefix(key, "php") && len(key) > 3 {
		return strings.TrimPrefix(key, "php")
	}
	if strings.HasPrefix(key, "nodejs") {
		v := strings.TrimPrefix(key, "nodejs")
		if v != "" {
			return v
		}
		return "JS"
	}
	if strings.HasPrefix(key, "java") {
		v := strings.TrimPrefix(key, "java")
		if v != "" {
			return v
		}
		return "Jv"
	}
	if strings.HasPrefix(key, "dotnet") {
		return "DN"
	}
	if strings.HasPrefix(key, "tomcat") {
		if strings.Contains(key, "10") {
			return "T10"
		}
		return "T9"
	}
	parts := strings.FieldsFunc(key, func(r rune) bool { return r == '-' || r == '_' })
	if len(parts) >= 2 {
		a, b := parts[0], parts[1]
		if len(a) > 0 && len(b) > 0 {
			return strings.ToUpper(string(a[0]) + string(b[0]))
		}
	}
	if len(key) >= 2 {
		return strings.ToUpper(key[:2])
	}
	return strings.ToUpper(key)
}

func hashIconColor(key string) string {
	var h uint32
	for i := 0; i < len(key); i++ {
		h = h*31 + uint32(key[i])
	}
	hue := h % 360
	return fmt.Sprintf("hsl(%d, 58%%, 46%%)", hue)
}

func WriteBadgeSVG(path, key string) error {
	meta := IconMetaForKey(key)
	label := strings.ReplaceAll(meta.Label, "&", "&amp;")
	label = strings.ReplaceAll(label, "<", "&lt;")
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" fill="none">
  <rect width="64" height="64" rx="12" fill="%s"/>
  <text x="32" y="38" text-anchor="middle" fill="#fff" font-family="system-ui,sans-serif" font-size="18" font-weight="700">%s</text>
</svg>
`, meta.BG, label)
	return os.WriteFile(path, []byte(svg), 0644)
}

func EnsureSoftwareIcons(dir string) (created int, err error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, err
	}
	for _, key := range CatalogKeys() {
		path := fmt.Sprintf("%s/%s.svg", strings.TrimRight(dir, "/\\"), key)
		if _, statErr := os.Stat(path); statErr == nil {
			continue
		}
		if writeErr := WriteBadgeSVG(path, key); writeErr != nil {
			return created, writeErr
		}
		created++
	}
	return created, nil
}
