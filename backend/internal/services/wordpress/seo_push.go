package wordpress

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/luuuunet/owpanel/internal/models"
)

type SEOPushSettings struct {
	Enabled       bool   `json:"enabled"`
	PushOnDeploy  bool   `json:"push_on_deploy"`
	Google        bool   `json:"google"`
	Bing          bool   `json:"bing"`
	IndexNow      bool   `json:"indexnow"`
	Baidu         bool   `json:"baidu"`
	Yandex        bool   `json:"yandex"`
	IndexNowKey   string `json:"indexnow_key"`
	SitemapURL    string `json:"sitemap_url"`
	BaiduToken    string `json:"baidu_token"`
	LastPushAt    *time.Time `json:"last_seo_push_at"`
	LastPushStatus string `json:"last_seo_push_status"`
	LastPushLog   string `json:"last_seo_push_log"`
}

type SEOPushEngineResult struct {
	Engine  string `json:"engine"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type SEOPushResult struct {
	SiteURL    string                `json:"site_url"`
	SitemapURL string                `json:"sitemap_url"`
	Results    []SEOPushEngineResult `json:"results"`
}

type SEOPushUpdateRequest struct {
	Enabled      *bool  `json:"enabled"`
	PushOnDeploy *bool  `json:"push_on_deploy"`
	Google       *bool  `json:"google"`
	Bing         *bool  `json:"bing"`
	IndexNow     *bool  `json:"indexnow"`
	Baidu        *bool  `json:"baidu"`
	Yandex       *bool  `json:"yandex"`
	IndexNowKey  string `json:"indexnow_key"`
	SitemapURL   string `json:"sitemap_url"`
	BaiduToken   string `json:"baidu_token"`
}

func (s *Service) GetSEOPushSettings(id uint) (*SEOPushSettings, error) {
	site, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	return seoSettingsFromSite(site), nil
}

func seoSettingsFromSite(site *models.WordPressSite) *SEOPushSettings {
	return &SEOPushSettings{
		Enabled:        site.SEOPushEnabled,
		PushOnDeploy:   site.SEOPushOnDeploy,
		Google:         site.SEOPushGoogle,
		Bing:           site.SEOPushBing,
		IndexNow:       site.SEOPushIndexNow,
		Baidu:          site.SEOPushBaidu,
		Yandex:         site.SEOPushYandex,
		IndexNowKey:    site.IndexNowKey,
		SitemapURL:     site.SitemapURL,
		BaiduToken:     site.BaiduPushToken,
		LastPushAt:     site.LastSEOPushAt,
		LastPushStatus: site.LastSEOPushStatus,
		LastPushLog:    site.LastSEOPushLog,
	}
}

func (s *Service) UpdateSEOPushSettings(id uint, req *SEOPushUpdateRequest) (*SEOPushSettings, error) {
	site, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	updates := map[string]interface{}{}
	if req.Enabled != nil {
		updates["seo_push_enabled"] = *req.Enabled
	}
	if req.PushOnDeploy != nil {
		updates["seo_push_on_deploy"] = *req.PushOnDeploy
	}
	if req.Google != nil {
		updates["seo_push_google"] = *req.Google
	}
	if req.Bing != nil {
		updates["seo_push_bing"] = *req.Bing
	}
	if req.IndexNow != nil {
		updates["seo_push_indexnow"] = *req.IndexNow
	}
	if req.Baidu != nil {
		updates["seo_push_baidu"] = *req.Baidu
	}
	if req.Yandex != nil {
		updates["seo_push_yandex"] = *req.Yandex
	}
	updates["sitemap_url"] = strings.TrimSpace(req.SitemapURL)
	updates["baidu_push_token"] = strings.TrimSpace(req.BaiduToken)
	key := strings.TrimSpace(req.IndexNowKey)
	if key != "" {
		updates["indexnow_key"] = key
	}
	if len(updates) == 0 {
		return seoSettingsFromSite(site), nil
	}
	if err := s.db.Model(site).Updates(updates).Error; err != nil {
		return nil, err
	}
	site, err = s.Get(id)
	if err != nil {
		return nil, err
	}
	if key != "" && site.RootPath != "" {
		_ = writeIndexNowKeyFile(site.RootPath, key)
	}
	return seoSettingsFromSite(site), nil
}

func (s *Service) PushToSearchEngines(id uint) (*SEOPushResult, error) {
	site, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if site.Status != "active" && site.Status != "running" && site.Status != "success" {
		return nil, fmt.Errorf("site is not ready for SEO push")
	}

	key := strings.TrimSpace(site.IndexNowKey)
	if key == "" {
		key = strings.ReplaceAll(uuid.New().String(), "-", "")
		_ = s.db.Model(site).Update("indexnow_key", key).Error
		site.IndexNowKey = key
	}
	if site.RootPath != "" {
		_ = writeIndexNowKeyFile(site.RootPath, key)
	}

	baseURL := sitePublicURL(site)
	client := &http.Client{Timeout: 20 * time.Second}
	sitemap := resolveSitemapURL(client, baseURL, strings.TrimSpace(site.SitemapURL))
	homeURL := strings.TrimSuffix(baseURL, "/") + "/"

	var results []SEOPushEngineResult
	if site.SEOPushGoogle {
		results = append(results, pingSitemapEngine("google", "https://www.google.com/ping?sitemap=", sitemap))
	}
	if site.SEOPushBing {
		results = append(results, pingSitemapEngine("bing", "https://www.bing.com/ping?sitemap=", sitemap))
	}
	if site.SEOPushYandex {
		results = append(results, pingSitemapEngine("yandex", "https://webmaster.yandex.com/ping?sitemap=", sitemap))
	}
	if site.SEOPushIndexNow {
		results = append(results, s.submitIndexNow(site.Domain, key, []string{homeURL, sitemap}))
	}
	if site.SEOPushBaidu {
		results = append(results, pingBaiduSitemap(sitemap, strings.TrimSpace(site.BaiduPushToken)))
	}

	status := "success"
	for _, r := range results {
		if !r.OK {
			status = "partial"
			break
		}
	}
	if len(results) == 0 {
		status = "skipped"
	} else {
		allFail := true
		for _, r := range results {
			if r.OK {
				allFail = false
				break
			}
		}
		if allFail {
			status = "failed"
		}
	}

	logLines := []string{fmt.Sprintf("[%s] site=%s sitemap=%s", time.Now().Format(time.RFC3339), homeURL, sitemap)}
	for _, r := range results {
		mark := "OK"
		if !r.OK {
			mark = "FAIL"
		}
		logLines = append(logLines, fmt.Sprintf("  [%s] %s: %s", mark, r.Engine, r.Message))
	}
	logText := strings.Join(logLines, "\n")
	now := time.Now()
	_ = s.db.Model(site).Updates(map[string]interface{}{
		"last_seo_push_at":     now,
		"last_seo_push_status": status,
		"last_seo_push_log":    logText,
	}).Error

	return &SEOPushResult{SiteURL: homeURL, SitemapURL: sitemap, Results: results}, nil
}

func (s *Service) PushSEOIfEnabled(id uint) {
	site, err := s.Get(id)
	if err != nil || !site.SEOPushEnabled || !site.SEOPushOnDeploy {
		return
	}
	_, _ = s.PushToSearchEngines(id)
}

func sitePublicURL(site *models.WordPressSite) string {
	scheme := "http"
	if site.SSL || site.ForceHTTPS || site.AutoSSL {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, strings.TrimSpace(site.Domain))
}

func resolveSitemapURL(client *http.Client, baseURL, override string) string {
	if u := strings.TrimSpace(override); u != "" {
		return u
	}
	base := strings.TrimSuffix(baseURL, "/")
	for _, path := range []string{"/wp-sitemap.xml", "/sitemap_index.xml", "/sitemap.xml"} {
		u := base + path
		req, err := http.NewRequest(http.MethodHead, u, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			return u
		}
	}
	return base + "/wp-sitemap.xml"
}

func pingSitemapEngine(name, pingBase, sitemapURL string) SEOPushEngineResult {
	u := pingBase + url.QueryEscape(sitemapURL)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return SEOPushEngineResult{Engine: name, OK: false, Message: err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = resp.Status
	}
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	if name == "google" && !ok {
		// Google deprecated ping; treat as informational success for UX
		return SEOPushEngineResult{
			Engine:  name,
			OK:      true,
			Message: "sitemap ping sent (Google 已弃用 Ping，请到 Search Console 提交站点地图)",
		}
	}
	return SEOPushEngineResult{Engine: name, OK: ok, Message: msg}
}

func pingBaiduSitemap(sitemapURL, token string) SEOPushEngineResult {
	u := "http://data.zz.baidu.com/ping?sitemap=" + url.QueryEscape(sitemapURL)
	if token != "" {
		u += "&token=" + url.QueryEscape(token)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return SEOPushEngineResult{Engine: "baidu", OK: false, Message: err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	return SEOPushEngineResult{Engine: "baidu", OK: ok, Message: strings.TrimSpace(string(body))}
}

func (s *Service) submitIndexNow(host, key string, urls []string) SEOPushEngineResult {
	payload := map[string]interface{}{
		"host":        host,
		"key":         key,
		"keyLocation": fmt.Sprintf("https://%s/%s.txt", host, key),
		"urlList":     urls,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, "https://api.indexnow.org/indexnow", bytes.NewReader(body))
	if err != nil {
		return SEOPushEngineResult{Engine: "indexnow", OK: false, Message: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return SEOPushEngineResult{Engine: "indexnow", OK: false, Message: err.Error()}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	ok := resp.StatusCode == 200 || resp.StatusCode == 202
	msg := strings.TrimSpace(string(respBody))
	if msg == "" {
		msg = resp.Status
	}
	if ok {
		msg = "submitted to Bing/Yandex/Naver etc. via IndexNow"
	}
	return SEOPushEngineResult{Engine: "indexnow", OK: ok, Message: msg}
}

func writeIndexNowKeyFile(rootPath, key string) error {
	if rootPath == "" || key == "" {
		return nil
	}
	p := filepath.Join(rootPath, key+".txt")
	return os.WriteFile(p, []byte(key), 0644)
}
