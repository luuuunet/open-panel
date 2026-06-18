package cache

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

var cacheLogLineRe = regexp.MustCompile(`^([^|]+)\|(\d+)\|([^|]*)\|([^|]*)\|([^|]*)`)

type AnalyticsSummary struct {
	TotalRequests  int64   `json:"total_requests"`
	TotalBandwidth int64   `json:"total_bandwidth"`
	CachedRequests int64   `json:"cached_requests"`
	OriginRequests int64   `json:"origin_requests"`
	EgressSaved    int64   `json:"egress_saved"`
	CacheHitRate   float64 `json:"cache_hit_rate"`
	CurrentStorage int64   `json:"current_storage"`
}

type AnalyticsTimePoint struct {
	Time           string `json:"time"`
	Requests       int64  `json:"requests"`
	Bandwidth      int64  `json:"bandwidth"`
	CachedRequests int64  `json:"cached_requests"`
	OriginRequests int64  `json:"origin_requests"`
	EgressSaved    int64  `json:"egress_saved"`
}

type StatusCount struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
	Bytes  int64  `json:"bytes"`
}

type NamedCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
	Bytes int64  `json:"bytes"`
}

type PathCount struct {
	Path  string `json:"path"`
	Count int64  `json:"count"`
	Bytes int64  `json:"bytes"`
}

type StoragePoint struct {
	Time  string `json:"time"`
	Bytes int64  `json:"bytes"`
}

type AnalyticsReport struct {
	RangeHours      int                  `json:"range_hours"`
	Domain          string               `json:"domain,omitempty"`
	Summary         AnalyticsSummary     `json:"summary"`
	TimeSeries      []AnalyticsTimePoint `json:"time_series"`
	StatusBreakdown []StatusCount        `json:"status_breakdown"`
	ContentTypes    []NamedCount         `json:"content_types"`
	TopPaths        []NamedCount         `json:"top_paths"`
	StorageHistory  []StoragePoint       `json:"storage_history"`
	SparkRequests   []int64              `json:"spark_requests"`
	SparkBandwidth  []int64              `json:"spark_bandwidth"`
}

type logEntry struct {
	at     time.Time
	bytes  int64
	path   string
	status string
}

func (s *Service) SiteCacheLogPath(domain string) string {
	safe := sanitizeDomain(domain)
	return filepath.Join(s.dataDir, "logs", safe+"_cache.log")
}

func (s *Service) CacheLogFormatBlock() string {
	return `
log_format opanel_cache '$time_iso8601|$body_bytes_sent|$request_uri|$upstream_cache_status|$sent_http_x_cache_status';
`
}

func (s *Service) GetAnalytics(hours int, domain string) (*AnalyticsReport, error) {
	if hours <= 0 {
		hours = 24
	}
	if hours > 168 {
		hours = 168
	}
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	entries := s.readCacheLogs(cutoff, domain)

	buckets := make(map[int64]*AnalyticsTimePoint)
	statusMap := make(map[string]*StatusCount)
	typeMap := make(map[string]*NamedCount)
	pathMap := make(map[string]*NamedCount)

	var summary AnalyticsSummary
	for _, e := range entries {
		bucketKey := e.at.Unix() / 3600
		tp, ok := buckets[bucketKey]
		if !ok {
			tp = &AnalyticsTimePoint{Time: time.Unix(bucketKey*3600, 0).Format("2006-01-02 15:04")}
			buckets[bucketKey] = tp
		}
		tp.Requests++
		tp.Bandwidth += e.bytes
		summary.TotalRequests++
		summary.TotalBandwidth += e.bytes

		norm := normalizeCacheStatus(e.status)
		sc, ok := statusMap[norm]
		if !ok {
			sc = &StatusCount{Status: norm}
			statusMap[norm] = sc
		}
		sc.Count++
		sc.Bytes += e.bytes

		if isCachedStatus(norm) {
			tp.CachedRequests++
			tp.EgressSaved += e.bytes
			summary.CachedRequests++
			summary.EgressSaved += e.bytes
		} else {
			tp.OriginRequests++
			summary.OriginRequests++
		}

		ext := contentTypeFromPath(e.path)
		tc, ok := typeMap[ext]
		if !ok {
			tc = &NamedCount{Name: ext}
			typeMap[ext] = tc
		}
		tc.Count++
		tc.Bytes += e.bytes

		p := truncatePath(e.path, 120)
		pc, ok := pathMap[p]
		if !ok {
			pc = &NamedCount{Name: p}
			pathMap[p] = pc
		}
		pc.Count++
		pc.Bytes += e.bytes
	}

	if summary.TotalRequests > 0 {
		summary.CacheHitRate = float64(summary.CachedRequests) / float64(summary.TotalRequests) * 100
	}
	summary.CurrentStorage = dirSize(s.ProxyCacheDir()) + dirSize(s.FastCGICacheDir())

	timeSeries := make([]AnalyticsTimePoint, 0, len(buckets))
	for _, tp := range buckets {
		timeSeries = append(timeSeries, *tp)
	}
	sort.Slice(timeSeries, func(i, j int) bool { return timeSeries[i].Time < timeSeries[j].Time })

	statusBreakdown := make([]StatusCount, 0, len(statusMap))
	for _, v := range statusMap {
		statusBreakdown = append(statusBreakdown, *v)
	}
	sort.Slice(statusBreakdown, func(i, j int) bool { return statusBreakdown[i].Count > statusBreakdown[j].Count })

	contentTypes := topNamed(typeMap, 12)
	topPaths := topNamed(pathMap, 15)

	filledSeries := fillTimeSeries(timeSeries, hours)

	storageHistory := buildStorageHistory(filledSeries, summary.CurrentStorage, hours)
	if hist := s.StorageHistoryFromDB(hours, domain); len(hist) > 0 {
		storageHistory = hist
	}
	sparkReq, sparkBw := buildSparklines(filledSeries, hours)

	return &AnalyticsReport{
		RangeHours:      hours,
		Domain:          domain,
		Summary:         summary,
		TimeSeries:      filledSeries,
		StatusBreakdown: statusBreakdown,
		ContentTypes:    contentTypes,
		TopPaths:        topPaths,
		StorageHistory:  storageHistory,
		SparkRequests:   sparkReq,
		SparkBandwidth:  sparkBw,
	}, nil
}

func (s *Service) readCacheLogs(cutoff time.Time, domain string) []logEntry {
	var out []logEntry
	logDir := filepath.Join(s.dataDir, "logs")
	if domain != "" {
		p := s.SiteCacheLogPath(domain)
		return s.parseCacheLogFile(p, cutoff)
	}
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), "_cache.log") {
			continue
		}
		out = append(out, s.parseCacheLogFile(filepath.Join(logDir, e.Name()), cutoff)...)
	}
	return out
}

func (s *Service) parseCacheLogFile(path string, cutoff time.Time) []logEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []logEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := cacheLogLineRe.FindStringSubmatch(line)
		if len(m) < 6 {
			continue
		}
		ts, err := time.Parse(time.RFC3339, m[1])
		if err != nil {
			ts, err = time.Parse("2006-01-02T15:04:05-07:00", m[1])
			if err != nil {
				continue
			}
		}
		if ts.Before(cutoff) {
			continue
		}
		bytes := parseInt64(m[2])
		uri := m[3]
		upstream := strings.TrimSpace(m[4])
		header := strings.TrimSpace(m[5])
		status := header
		if status == "" || status == "-" {
			status = upstream
		}
		out = append(out, logEntry{at: ts, bytes: bytes, path: uri, status: status})
	}
	return out
}

func parseInt64(s string) int64 {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

func normalizeCacheStatus(raw string) string {
	raw = strings.ToUpper(strings.TrimSpace(raw))
	switch raw {
	case "HIT", "MISS", "BYPASS", "EXPIRED", "STALE", "REVALIDATED", "UPDATING":
		return raw
	case "STATIC", "BROWSER":
		return raw
	case "", "-":
		return "NONE"
	default:
		if strings.Contains(raw, "HIT") {
			return "HIT"
		}
		return "DYNAMIC"
	}
}

func isCachedStatus(status string) bool {
	switch status {
	case "HIT", "STALE", "STATIC", "REVALIDATED":
		return true
	default:
		return false
	}
}

func contentTypeFromPath(path string) string {
	path = strings.Split(path, "?")[0]
	idx := strings.LastIndex(path, ".")
	if idx < 0 || idx == len(path)-1 {
		if path == "/" || path == "" {
			return "html"
		}
		return "other"
	}
	ext := strings.ToLower(path[idx+1:])
	switch ext {
	case "htm":
		return "html"
	case "jpeg":
		return "jpg"
	default:
		return ext
	}
}

func truncatePath(p string, n int) string {
	if len(p) <= n {
		return p
	}
	return p[:n] + "…"
}

func topNamed(m map[string]*NamedCount, limit int) []NamedCount {
	list := make([]NamedCount, 0, len(m))
	for _, v := range m {
		list = append(list, *v)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Count == list[j].Count {
			return list[i].Bytes > list[j].Bytes
		}
		return list[i].Count > list[j].Count
	})
	if len(list) > limit {
		list = list[:limit]
	}
	return list
}

func fillTimeSeries(existing []AnalyticsTimePoint, hours int) []AnalyticsTimePoint {
	if hours <= 0 {
		hours = 24
	}
	now := time.Now().Truncate(time.Hour)
	byHour := make(map[int64]AnalyticsTimePoint, len(existing))
	for _, tp := range existing {
		key, ok := parseTimePointHour(tp.Time)
		if !ok {
			continue
		}
		byHour[key] = tp
	}

	points := make([]AnalyticsTimePoint, 0, hours)
	for i := hours - 1; i >= 0; i-- {
		t := now.Add(-time.Duration(i) * time.Hour)
		key := t.Unix()
		if tp, ok := byHour[key]; ok {
			tp.Time = t.Format("2006-01-02 15:04")
			points = append(points, tp)
			continue
		}
		points = append(points, AnalyticsTimePoint{Time: t.Format("2006-01-02 15:04")})
	}
	return points
}

func parseTimePointHour(raw string) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	layouts := []string{"2006-01-02 15:04", "2006-01-02T15:04:05-07:00", time.RFC3339}
	for _, layout := range layouts {
		t, err := time.ParseInLocation(layout, raw, time.Local)
		if err != nil {
			continue
		}
		return t.Truncate(time.Hour).Unix(), true
	}
	return 0, false
}

func buildStorageHistory(series []AnalyticsTimePoint, current int64, hours int) []StoragePoint {
	if len(series) == 0 {
		now := time.Now()
		return []StoragePoint{{Time: now.Format("2006-01-02 15:04"), Bytes: current}}
	}
	out := make([]StoragePoint, 0, len(series))
	var cumulative int64
	for _, tp := range series {
		cumulative += tp.EgressSaved
		val := current
		if cumulative > 0 && cumulative < current {
			val = cumulative
		}
		out = append(out, StoragePoint{Time: tp.Time, Bytes: val})
	}
	if len(out) > hours {
		out = out[len(out)-hours:]
	}
	return out
}

func buildSparklines(series []AnalyticsTimePoint, hours int) ([]int64, []int64) {
	n := 24
	if hours <= 6 {
		n = 12
	}
	req := make([]int64, n)
	bw := make([]int64, n)
	if len(series) == 0 {
		return req, bw
	}
	step := len(series) / n
	if step < 1 {
		step = 1
	}
	for i := 0; i < n; i++ {
		start := i * step
		end := start + step
		if i == n-1 {
			end = len(series)
		}
		if start >= len(series) {
			break
		}
		if end > len(series) {
			end = len(series)
		}
		for _, tp := range series[start:end] {
			req[i] += tp.Requests
			bw[i] += tp.Bandwidth
		}
	}
	return req, bw
}

func (s *Service) RecordSnapshot(domain string, report *AnalyticsReport) error {
	if report == nil {
		return nil
	}
	snap := models.CacheSnapshot{
		Domain:       domain,
		Requests:     report.Summary.TotalRequests,
		CachedReqs:   report.Summary.CachedRequests,
		HitRate:      report.Summary.CacheHitRate,
		StorageBytes: report.Summary.CurrentStorage,
	}
	return s.db.Create(&snap).Error
}

func (s *Service) StorageHistoryFromDB(hours int, domain string) []StoragePoint {
	if hours <= 0 {
		hours = 24
	}
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	var snaps []models.CacheSnapshot
	q := s.db.Where("created_at >= ?", cutoff).Order("created_at asc")
	if domain != "" {
		q = q.Where("domain = ?", domain)
	}
	if err := q.Find(&snaps).Error; err != nil || len(snaps) == 0 {
		return nil
	}
	out := make([]StoragePoint, 0, len(snaps))
	for _, snap := range snaps {
		out = append(out, StoragePoint{
			Time:  snap.CreatedAt.Format("2006-01-02 15:04"),
			Bytes: snap.StorageBytes,
		})
	}
	return out
}
