package uptime

import (
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

type ImportResult struct {
	Created int `json:"created"`
	Skipped int `json:"skipped"`
	Total   int `json:"total"`
}

func websiteProbeURL(site models.Website) string {
	scheme := "http"
	if site.SSL || site.ForceHTTPS {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/", scheme, strings.TrimSpace(site.Domain))
}

// ImportFromWebsites creates uptime monitors for running websites that are not already monitored.
// When websiteIDs is non-empty, only those sites are considered (still must be running).
func (s *Service) ImportFromWebsites(intervalSec int, websiteIDs []uint) (*ImportResult, error) {
	if intervalSec < 15 {
		intervalSec = 300
	}
	var sites []models.Website
	q := s.db.Where("status = ?", "running")
	if len(websiteIDs) > 0 {
		q = q.Where("id IN ?", websiteIDs)
	}
	if err := q.Find(&sites).Error; err != nil {
		return nil, err
	}
	existing, err := s.List()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, m := range existing {
		seen[strings.ToLower(strings.TrimSpace(m.URL))] = true
	}
	res := &ImportResult{Total: len(sites)}
	for _, site := range sites {
		url := websiteProbeURL(site)
		if seen[strings.ToLower(url)] {
			res.Skipped++
			continue
		}
		m := &models.UptimeMonitor{
			Name:           site.Domain,
			URL:            url,
			Method:         "GET",
			IntervalSec:    intervalSec,
			TimeoutSec:     10,
			ExpectedStatus: 200,
			Enabled:        true,
		}
		if err := s.Create(m); err != nil {
			return res, err
		}
		seen[strings.ToLower(url)] = true
		res.Created++
	}
	return res, nil
}
