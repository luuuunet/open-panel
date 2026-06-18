package analytics

import (
	"net/url"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

type DomainTraffic struct {
	Host      string `json:"host"`
	WebsiteID uint   `json:"website_id,omitempty"`
	Count     int64  `json:"count"`
	Bytes     uint64 `json:"bytes"`
}

type PathStat struct {
	Path  string `json:"path"`
	Count int64  `json:"count"`
	Bytes uint64 `json:"bytes"`
}

type RefererStat struct {
	Host  string `json:"host"`
	Count int64  `json:"count"`
}

type IPStat struct {
	IP    string `json:"ip"`
	Count int64  `json:"count"`
	Bytes uint64 `json:"bytes"`
}

type DomainDetailResponse struct {
	Host       string        `json:"host"`
	TotalPV    int64         `json:"total_pv"`
	TotalBytes uint64        `json:"total_bytes"`
	TopPaths   []PathStat    `json:"top_paths"`
	TopReferers []RefererStat `json:"top_referers"`
	TopIPs     []IPStat      `json:"top_ips"`
}

type TrafficWebsiteOption struct {
	ID     uint   `json:"id"`
	Domain string `json:"domain"`
}

func (s *Service) parseHours(hours int) (time.Time, int) {
	if hours <= 0 {
		hours = 24
	}
	if hours > 720 {
		hours = 720
	}
	return time.Now().Add(-time.Duration(hours) * time.Hour), hours
}

func (s *Service) countryFilter(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func (s *Service) GetCountryDomains(countryCode string, hours int) ([]DomainTraffic, error) {
	since, _ := s.parseHours(hours)
	code := s.countryFilter(countryCode)
	if code == "" {
		return []DomainTraffic{}, nil
	}

	type aggRow struct {
		Host  string
		Count int64
		Bytes uint64
	}
	var rows []aggRow
	s.db.Model(&models.TrafficHit{}).
		Select("host, count(*) as count, sum(bytes) as bytes").
		Where("created_at >= ? AND log_source != ? AND country_code = ? AND host != ''", since, "demo", code).
		Group("host").
		Order("count desc").
		Limit(100).
		Scan(&rows)

	hostToSite := s.hostWebsiteMap()
	out := make([]DomainTraffic, 0, len(rows))
	for _, r := range rows {
		dt := DomainTraffic{Host: r.Host, Count: r.Count, Bytes: r.Bytes}
		if id, ok := hostToSite[r.Host]; ok {
			dt.WebsiteID = id
		}
		out = append(out, dt)
	}
	return out, nil
}

func (s *Service) GetCountryDomainDetails(countryCode, host string, hours int) (*DomainDetailResponse, error) {
	since, _ := s.parseHours(hours)
	code := s.countryFilter(countryCode)
	host = strings.TrimSpace(host)
	if code == "" || host == "" {
		return &DomainDetailResponse{Host: host}, nil
	}

	base := s.db.Model(&models.TrafficHit{}).
		Where("created_at >= ? AND log_source != ? AND country_code = ? AND host = ?", since, "demo", code, host)

	var totalPV int64
	var totalBytes uint64
	base.Count(&totalPV)
	base.Select("coalesce(sum(bytes),0)").Scan(&totalBytes)

	type pathRow struct {
		Path  string
		Count int64
		Bytes uint64
	}
	var pathRows []pathRow
	s.db.Model(&models.TrafficHit{}).
		Select("path, count(*) as count, sum(bytes) as bytes").
		Where("created_at >= ? AND log_source != ? AND country_code = ? AND host = ?", since, "demo", code, host).
		Group("path").
		Order("count desc").
		Limit(30).
		Scan(&pathRows)

	topPaths := make([]PathStat, 0, len(pathRows))
	for _, r := range pathRows {
		topPaths = append(topPaths, PathStat{Path: r.Path, Count: r.Count, Bytes: r.Bytes})
	}

	var refererRows []struct {
		Referer string
		Count   int64
	}
	s.db.Model(&models.TrafficHit{}).
		Select("referer, count(*) as count").
		Where("created_at >= ? AND log_source != ? AND country_code = ? AND host = ? AND referer != '' AND referer != '-'", since, "demo", code, host).
		Group("referer").
		Order("count desc").
		Limit(200).
		Scan(&refererRows)

	refAgg := map[string]int64{}
	for _, r := range refererRows {
		h := refererHost(r.Referer)
		if h == "" {
			continue
		}
		refAgg[h] += r.Count
	}
	topReferers := make([]RefererStat, 0, len(refAgg))
	for h, c := range refAgg {
		topReferers = append(topReferers, RefererStat{Host: h, Count: c})
	}
	sortReferers(topReferers)
	if len(topReferers) > 20 {
		topReferers = topReferers[:20]
	}

	type ipRow struct {
		IP    string
		Count int64
		Bytes uint64
	}
	var ipRows []ipRow
	s.db.Model(&models.TrafficHit{}).
		Select("ip, count(*) as count, sum(bytes) as bytes").
		Where("created_at >= ? AND log_source != ? AND country_code = ? AND host = ?", since, "demo", code, host).
		Group("ip").
		Order("count desc").
		Limit(20).
		Scan(&ipRows)

	topIPs := make([]IPStat, 0, len(ipRows))
	for _, r := range ipRows {
		topIPs = append(topIPs, IPStat{IP: r.IP, Count: r.Count, Bytes: r.Bytes})
	}

	return &DomainDetailResponse{
		Host:        host,
		TotalPV:     totalPV,
		TotalBytes:  totalBytes,
		TopPaths:    topPaths,
		TopReferers: topReferers,
		TopIPs:      topIPs,
	}, nil
}

func (s *Service) ListTrafficWebsites() ([]TrafficWebsiteOption, error) {
	var sites []models.Website
	if err := s.db.Select("id, domain").Order("domain asc").Find(&sites).Error; err != nil {
		return nil, err
	}
	out := make([]TrafficWebsiteOption, 0, len(sites))
	for _, site := range sites {
		out = append(out, TrafficWebsiteOption{ID: site.ID, Domain: site.Domain})
	}
	return out, nil
}

func (s *Service) hostWebsiteMap() map[string]uint {
	var sites []models.Website
	s.db.Select("id, domain").Find(&sites)
	m := make(map[string]uint, len(sites))
	for _, site := range sites {
		m[site.Domain] = site.ID
	}
	return m
}

func refererHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "-" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		if strings.Contains(raw, "://") {
			return ""
		}
		parts := strings.SplitN(raw, "/", 2)
		return strings.TrimSpace(parts[0])
	}
	return u.Host
}

func sortReferers(items []RefererStat) {
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].Count > items[i].Count {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}
