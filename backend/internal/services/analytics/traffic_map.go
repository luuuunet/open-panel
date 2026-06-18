package analytics

import (
	"strconv"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/waf"
)

type CountryTraffic struct {
	Code    string  `json:"code"`
	Name    string  `json:"name"`
	Zh      string  `json:"zh"`
	MapName string  `json:"map_name"`
	Count   int64   `json:"count"`
	Bytes   uint64  `json:"bytes"`
	Percent float64 `json:"percent"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
}

type CityTraffic struct {
	Name    string  `json:"name"`
	Country string  `json:"country"`
	Count   int64   `json:"count"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
}

type TimelinePoint struct {
	Time  string `json:"time"`
	Count int64  `json:"count"`
}

type TrafficMapResponse struct {
	TotalRequests int64            `json:"total_requests"`
	TotalBytes    uint64           `json:"total_bytes"`
	UniqueIPs     int64            `json:"unique_ips"`
	Countries     []CountryTraffic `json:"countries"`
	Cities        []CityTraffic    `json:"cities"`
	Timeline      []TimelinePoint  `json:"timeline"`
	Source        string           `json:"source"`
	LogPaths      []string         `json:"log_paths"`
	GeoDBReady    bool             `json:"geo_db_ready"`
	ServerLat     float64          `json:"server_lat"`
	ServerLng     float64          `json:"server_lng"`
}

func (s *Service) GetTrafficMap(hours int) (*TrafficMapResponse, error) {
	if hours <= 0 {
		hours = 24
	}
	if hours > 720 {
		hours = 720
	}

	cacheTTL := s.trafficMapCacheTTL()
	s.mapCacheMu.Lock()
	if entry, ok := s.mapCache[hours]; ok && time.Since(entry.at) < cacheTTL && entry.resp != nil {
		resp := *entry.resp
		s.mapCacheMu.Unlock()
		return &resp, nil
	}
	s.mapCacheMu.Unlock()

	resp, err := s.buildTrafficMap(hours)
	if err != nil {
		return nil, err
	}
	s.mapCacheMu.Lock()
	if len(s.mapCache) >= maxMapCacheEntries {
		var oldestKey int
		var oldestAt time.Time
		first := true
		for k, v := range s.mapCache {
			if first || v.at.Before(oldestAt) {
				oldestKey, oldestAt, first = k, v.at, false
			}
		}
		delete(s.mapCache, oldestKey)
	}
	s.mapCache[hours] = trafficMapCacheEntry{at: time.Now(), resp: resp}
	s.mapCacheMu.Unlock()
	return resp, nil
}

func (s *Service) buildTrafficMap(hours int) (*TrafficMapResponse, error) {
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	baseWhere := "created_at >= ? AND log_source != ?"
	baseArgs := []interface{}{since, "demo"}

	type summaryRow struct {
		Total int64
		Bytes uint64
	}
	var summary summaryRow
	s.db.Model(&models.TrafficHit{}).
		Select("count(*) as total, coalesce(sum(bytes),0) as bytes").
		Where(baseWhere, baseArgs...).
		Scan(&summary)
	total := summary.Total
	if total == 0 {
		return &TrafficMapResponse{
			Countries:  []CountryTraffic{},
			Cities:     []CityTraffic{},
			Timeline:   []TimelinePoint{},
			Source:     "empty",
			LogPaths:   s.logPaths(),
			GeoDBReady: s.geoDBReady(),
		}, nil
	}

	var uniqueIPs int64
	s.db.Model(&models.TrafficHit{}).Where(baseWhere, baseArgs...).Distinct("ip").Count(&uniqueIPs)

	type aggRow struct {
		CountryCode string
		CountryName string
		Count       int64
		Bytes       uint64
		AvgLat      float64
		AvgLng      float64
	}
	var rows []aggRow
	s.db.Model(&models.TrafficHit{}).
		Select("country_code, country_name, count(*) as count, sum(bytes) as bytes, avg(latitude) as avg_lat, avg(longitude) as avg_lng").
		Where("created_at >= ? AND log_source != ? AND country_code != '' AND country_code != 'XX'", since, "demo").
		Group("country_code, country_name").
		Order("count desc").
		Limit(120).
		Scan(&rows)

	countries := make([]CountryTraffic, 0, len(rows))
	for _, r := range rows {
		pct := float64(0)
		if total > 0 {
			pct = float64(r.Count) * 100 / float64(total)
		}
		lat, lng := r.AvgLat, r.AvgLng
		if lat == 0 && lng == 0 {
			lat, lng = countryCentroid(r.CountryCode)
		}
		countries = append(countries, CountryTraffic{
			Code:    r.CountryCode,
			Name:    r.CountryName,
			Zh:      countryZh(r.CountryCode),
			MapName: echartsMapName(r.CountryCode, r.CountryName),
			Count:   r.Count,
			Bytes:   r.Bytes,
			Percent: round1(pct),
			Lat:     lat,
			Lng:     lng,
		})
	}

	type cityRow struct {
		City        string
		CountryCode string
		Count       int64
		AvgLat      float64
		AvgLng      float64
	}
	var cityRows []cityRow
	s.db.Model(&models.TrafficHit{}).
		Select("city, country_code, count(*) as count, avg(latitude) as avg_lat, avg(longitude) as avg_lng").
		Where("created_at >= ? AND log_source != ? AND city != '' AND latitude != 0", since, "demo").
		Group("city, country_code").
		Order("count desc").
		Limit(80).
		Scan(&cityRows)

	cities := make([]CityTraffic, 0, len(cityRows))
	for _, r := range cityRows {
		cities = append(cities, CityTraffic{
			Name:    r.City,
			Country: r.CountryCode,
			Count:   r.Count,
			Lat:     r.AvgLat,
			Lng:     r.AvgLng,
		})
	}

	return &TrafficMapResponse{
		TotalRequests: total,
		TotalBytes:    summary.Bytes,
		UniqueIPs:     uniqueIPs,
		Countries:     countries,
		Cities:        cities,
		Timeline:      s.buildTimeline(since),
		Source:        "logs",
		LogPaths:      s.logPaths(),
		GeoDBReady:    s.geoDBReady(),
	}, nil
}

func (s *Service) trafficMapCacheTTL() time.Duration {
	if s.perf != nil {
		if d := s.perf.TrafficMapCacheInterval(); d > 0 {
			return d
		}
	}
	return trafficMapCacheTTL
}

func (s *Service) geoDBReady() bool {
	s.geoDBMu.RLock()
	defer s.geoDBMu.RUnlock()
	return s.geoDB != nil
}

func (s *Service) buildTimeline(since time.Time) []TimelinePoint {
	type tlRow struct {
		Bucket int64
		Count  int64
	}
	var rows []tlRow
	s.db.Model(&models.TrafficHit{}).
		Select("(cast(strftime('%s', created_at) as integer) / 3600) * 3600 as bucket, count(*) as count").
		Where("created_at >= ? AND log_source != ?", since, "demo").
		Group("bucket").
		Order("bucket").
		Scan(&rows)

	out := make([]TimelinePoint, 0, len(rows))
	for _, r := range rows {
		t := time.Unix(r.Bucket, 0)
		out = append(out, TimelinePoint{Time: t.Format("15:04"), Count: r.Count})
	}
	return out
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

func countryZh(code string) string {
	for _, c := range waf.ListCountries() {
		if c.Code == code {
			return c.Zh
		}
	}
	return code
}

func mapCountryName(code, fallback string) string {
	for _, c := range waf.ListCountries() {
		if c.Code == code {
			return c.Name
		}
	}
	if fallback != "" {
		return fallback
	}
	return code
}

func (s *Service) seedDemoTraffic() {
	var n int64
	s.db.Model(&models.TrafficHit{}).Count(&n)
	if n > 0 {
		return
	}
	samples := []struct {
		code string
		city string
		n    int
	}{
		{"CN", "Shanghai", 420}, {"CN", "Beijing", 380}, {"US", "New York", 290},
		{"US", "Los Angeles", 210}, {"JP", "Tokyo", 180}, {"SG", "Singapore", 150},
		{"DE", "Frankfurt", 120}, {"GB", "London", 110}, {"KR", "Seoul", 95},
		{"FR", "Paris", 88}, {"IN", "Mumbai", 76}, {"AU", "Sydney", 65},
		{"BR", "São Paulo", 54}, {"RU", "Moscow", 48}, {"CA", "Toronto", 42},
		{"NL", "Amsterdam", 38}, {"HK", "Hong Kong", 35}, {"TW", "Taipei", 32},
	}
	now := time.Now()
	var hits []models.TrafficHit
	for si, sample := range samples {
		lat, lng := countryCentroid(sample.code)
		if clat, clng, ok := cityCoord(sample.city); ok {
			lat, lng = clat, clng
		}
		for i := 0; i < sample.n; i++ {
			hits = append(hits, models.TrafficHit{
				CreatedAt:   now.Add(-time.Duration((si*30+i)%1440) * time.Minute),
				IP:          "203.0.113." + strconv.Itoa((si+i)%250+1),
				CountryCode: sample.code,
				CountryName: mapCountryName(sample.code, ""),
				City:        sample.city,
				Latitude:    lat,
				Longitude:   lng,
				Bytes:       uint64(1024 + i*17),
				Status:      200,
				Method:      "GET",
				Path:        "/",
				LogSource:   "demo",
			})
		}
	}
	_ = s.db.CreateInBatches(hits, 300).Error
}
