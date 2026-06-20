package productanalytics

import (
	"fmt"
	"html"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/appstore"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"gorm.io/gorm"
)

const (
	DefaultDashboardURL = "http://localhost:3300"
	DefaultAPIURL       = "http://localhost:3333/api"
)

type Service struct {
	db       *gorm.DB
	dataDir  string
	settings *settings.Service
}

type Status struct {
	Installed    bool   `json:"installed"`
	Running      bool   `json:"running"`
	DashboardURL string `json:"dashboard_url"`
	APIURL       string `json:"api_url"`
}

type TrackingSnippet struct {
	Snippet string `json:"snippet"`
}

type WebsiteConfig struct {
	ProductAnalyticsEnabled  bool   `json:"product_analytics_enabled"`
	ProductAnalyticsClientID string `json:"product_analytics_client_id"`
	ProductAnalyticsAPIURL   string `json:"product_analytics_api_url"`
}

func NewService(db *gorm.DB, dataDir string, settingsSvc *settings.Service) *Service {
	return &Service{db: db, dataDir: dataDir, settings: settingsSvc}
}

func (s *Service) Status() Status {
	installed := appstore.OpenpanelInstalled(s.dataDir)
	running := appstore.OpenpanelComposeStatus(s.dataDir) == "running"
	dashboardURL := DefaultDashboardURL
	apiURL := DefaultAPIURL
	if s.settings != nil {
		if all, err := s.settings.GetAll(); err == nil {
			host := strings.TrimSpace(all["server_public_ip"])
			if host != "" {
				host = strings.TrimPrefix(host, "http://")
				host = strings.TrimPrefix(host, "https://")
				host = strings.Split(host, "/")[0]
				host = strings.Split(host, ":")[0]
				if host != "" {
					dashboardURL = fmt.Sprintf("http://%s:3300", host)
					apiURL = fmt.Sprintf("http://%s:3333/api", host)
				}
			}
		}
	}
	return Status{
		Installed:    installed,
		Running:      running,
		DashboardURL: dashboardURL,
		APIURL:       apiURL,
	}
}

func (s *Service) TrackingSnippet(clientID, apiURL string) TrackingSnippet {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = "YOUR_CLIENT_ID"
	}
	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" {
		apiURL = DefaultAPIURL
	}
	apiURL = strings.TrimRight(apiURL, "/")
	if !strings.HasSuffix(apiURL, "/api") {
		apiURL += "/api"
	}
	scriptBase := strings.TrimSuffix(apiURL, "/api")

	snippet := fmt.Sprintf(`<script>
  window.op=window.op||function(){var n=[];return new Proxy(function(){arguments.length&&n.push([].slice.call(arguments))},{get:function(t,r){return"q"===r?n:function(){n.push([r].concat([].slice.call(arguments)))}},has:function(t,r){return"q"===r}})()}();
  window.op('init', {
    apiUrl: '%s',
    clientId: '%s',
    trackScreenViews: true,
    trackOutgoingLinks: true,
    trackAttributes: true,
  });
</script>
<script src="%s/op1.js" defer async></script>`,
		html.EscapeString(apiURL),
		html.EscapeString(clientID),
		html.EscapeString(scriptBase),
	)
	return TrackingSnippet{Snippet: snippet}
}

func (s *Service) UpdateWebsiteConfig(siteID uint, req WebsiteConfig) (*models.Website, error) {
	var site models.Website
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, err
	}
	updates := map[string]interface{}{
		"product_analytics_enabled":   req.ProductAnalyticsEnabled,
		"product_analytics_client_id": strings.TrimSpace(req.ProductAnalyticsClientID),
		"product_analytics_api_url":   strings.TrimSpace(req.ProductAnalyticsAPIURL),
	}
	if err := s.db.Model(&site).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := s.db.First(&site, siteID).Error; err != nil {
		return nil, err
	}
	return &site, nil
}
