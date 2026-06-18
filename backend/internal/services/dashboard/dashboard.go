package dashboard

import (
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/docker"
	"github.com/luuuunet/owpanel/internal/services/performance"
	"github.com/shirou/gopsutil/v3/host"
)

const maxHistoryPoints = 2880 // ~12 hours at 15s interval in memory cache
const overviewCacheTTL = 60 * time.Second

type Stats struct {
	CPU      CPUStats      `json:"cpu"`
	Memory   MemoryStats   `json:"memory"`
	Swap     SwapStats     `json:"swap"`
	Load     LoadStats     `json:"load"`
	Disk     []DiskStats   `json:"disk"`
	DiskIO   DiskIOStats   `json:"disk_io"`
	Network  NetworkStats  `json:"network"`
	System   SystemInfo    `json:"system"`
	Overview OverviewStats `json:"overview,omitempty"`
}

type CPUStats struct {
	UsagePercent float64 `json:"usage_percent"`
	Cores        int     `json:"cores"`
}

type MemoryStats struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"used_percent"`
}

type SwapStats struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"used_percent"`
}

type LoadStats struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

type DiskStats struct {
	Mount       string  `json:"mount"`
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"used_percent"`
}

type DiskIOStats struct {
	ReadBytes  uint64  `json:"read_bytes"`
	WriteBytes uint64  `json:"write_bytes"`
	ReadRate   float64 `json:"read_rate"`
	WriteRate  float64 `json:"write_rate"`
}

type NetworkStats struct {
	BytesSent    uint64  `json:"bytes_sent"`
	BytesRecv    uint64  `json:"bytes_recv"`
	UploadRate   float64 `json:"upload_rate"`
	DownloadRate float64 `json:"download_rate"`
}

type SystemInfo struct {
	Hostname        string `json:"hostname"`
	OS              string `json:"os"`
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platform_version"`
	Uptime          uint64 `json:"uptime"`
}

type OverviewStats struct {
	Websites          int `json:"websites"`
	WebsitesRunning   int `json:"websites_running"`
	Databases         int `json:"databases"`
	DockerTotal       int `json:"docker_total"`
	DockerRunning     int `json:"docker_running"`
	SoftwareInstalled int `json:"software_installed"`
	SoftwareRunning   int `json:"software_running"`
	SSLTotal          int `json:"ssl_total"`
	SSLExpiringSoon   int `json:"ssl_expiring_soon"`
	CronJobs          int `json:"cron_jobs"`
	FTPAccounts       int `json:"ftp_accounts"`
	WordPressSites    int `json:"wordpress_sites"`
}

type MetricPoint struct {
	Time      int64   `json:"time"`
	CPU       float64 `json:"cpu"`
	Memory    float64 `json:"memory"`
	Load1     float64 `json:"load1"`
	NetUp     float64 `json:"net_up"`
	NetDown   float64 `json:"net_down"`
	DiskRead  float64 `json:"disk_read"`
	DiskWrite float64 `json:"disk_write"`
}

type HistoryResponse struct {
	Points   []MetricPoint `json:"points"`
	Hours    float64       `json:"hours"`
	Interval int           `json:"interval_sec"`
}

type MonitorResponse struct {
	Current      *Stats           `json:"current"`
	History      []MetricPoint    `json:"history"`
	Hours        float64          `json:"hours"`
	Interval     int              `json:"interval_sec"`
	TopProcesses  []ProcessBrief        `json:"top_processes"`
	RunningApps   []RunningAppInfo      `json:"running_apps"`
	InstalledApps []InstalledAppMetrics `json:"installed_apps"`
	AIModels      []AIModelInfo         `json:"ai_models"`
}

type rateSnapshot struct {
	at        time.Time
	netSent   uint64
	netRecv   uint64
	diskRead  uint64
	diskWrite uint64
}

type overviewCache struct {
	at    time.Time
	stats OverviewStats
}

type Service struct {
	db                  *gorm.DB
	perf                *performance.Service
	mu                  sync.RWMutex
	sampleMetaMu        sync.Mutex
	sampleMetaAt        time.Time
	sampleCores         int
	sampleHost          host.InfoStat
	sampleDisks         []DiskStats
	sampleDisksAt       time.Time
	prev                *rateSnapshot
	history             []MetricPoint
	latest              rawSample
	latestRates         ioRates
	hasSample           bool
	samplesSincePersist int
	persistCount        int
	overviewMu          sync.Mutex
	overview            overviewCache
}

func NewService(db *gorm.DB, perf *performance.Service) *Service {
	s := &Service{db: db, perf: perf}
	s.StartCollector()
	return s
}

func (s *Service) currentStats(includeOverview bool) *Stats {
	s.mu.RLock()
	has := s.hasSample
	cur := s.latest
	rates := s.latestRates
	s.mu.RUnlock()

	if !has {
		cur = s.sampleRaw()
		rates = ioRates{}
	}
	st := rawToStats(cur, rates)
	if includeOverview {
		st.Overview = s.buildOverview()
	}
	return st
}

func (s *Service) GetStats() (*Stats, error) {
	return s.currentStats(true), nil
}

func (s *Service) collectIntervalSec() int {
	if s.perf != nil {
		return s.perf.GetProfile().CollectSec
	}
	return 15
}

func (s *Service) GetMonitor(hours float64) MonitorResponse {
	hours = clampHours(hours)
	points := s.loadHistory(hours)
	return MonitorResponse{
		Current:  s.currentStats(false),
		History:  points,
		Hours:    hours,
		Interval: s.collectIntervalSec(),
	}
}

func (s *Service) GetHistory(hours float64) HistoryResponse {
	hours = clampHours(hours)
	return HistoryResponse{
		Points:   s.loadHistory(hours),
		Hours:    hours,
		Interval: s.collectIntervalSec(),
	}
}

func (s *Service) loadHistory(hours float64) []MetricPoint {
	cutoff := time.Now().Add(-time.Duration(hours * float64(time.Hour)))
	cutoffUnix := cutoff.Unix()

	var dbPoints []MetricPoint
	if s.db != nil {
		var rows []models.MetricSnapshot
		if err := s.db.Where("created_at >= ?", cutoff).Order("created_at asc").Find(&rows).Error; err == nil {
			dbPoints = make([]MetricPoint, 0, len(rows))
			for _, r := range rows {
				dbPoints = append(dbPoints, MetricPoint{
					Time: r.CreatedAt.Unix(), CPU: r.CPU, Memory: r.Memory, Load1: r.Load1,
					NetUp: r.NetUp, NetDown: r.NetDown, DiskRead: r.DiskRead, DiskWrite: r.DiskWrite,
				})
			}
		}
	}

	s.mu.RLock()
	memPoints := make([]MetricPoint, 0, len(s.history))
	for _, p := range s.history {
		if p.Time >= cutoffUnix {
			memPoints = append(memPoints, p)
		}
	}
	s.mu.RUnlock()

	merged := mergeMetricHistory(dbPoints, memPoints)
	return downsamplePoints(merged, maxChartPoints(hours))
}

// mergeMetricHistory combines DB snapshots with in-memory samples.
// Memory wins on duplicate timestamps so charts keep updating after restarts.
func mergeMetricHistory(dbPoints, memPoints []MetricPoint) []MetricPoint {
	if len(dbPoints) == 0 {
		return memPoints
	}
	if len(memPoints) == 0 {
		return dbPoints
	}
	byTime := make(map[int64]MetricPoint, len(dbPoints)+len(memPoints))
	for _, p := range dbPoints {
		byTime[p.Time] = p
	}
	for _, p := range memPoints {
		byTime[p.Time] = p
	}
	out := make([]MetricPoint, 0, len(byTime))
	for _, p := range byTime {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	return out
}

func clampHours(hours float64) float64 {
	if hours <= 0 {
		return 1
	}
	if hours > 24 {
		return 24
	}
	return hours
}

func maxChartPoints(hours float64) int {
	switch {
	case hours <= 1:
		return 120
	case hours <= 6:
		return 180
	default:
		return 240
	}
}

func downsamplePoints(points []MetricPoint, max int) []MetricPoint {
	if len(points) <= max || max <= 0 {
		return points
	}
	step := float64(len(points)) / float64(max)
	out := make([]MetricPoint, 0, max)
	for i := 0; i < max; i++ {
		idx := int(float64(i) * step)
		if idx >= len(points) {
			idx = len(points) - 1
		}
		out = append(out, points[idx])
	}
	return out
}

func (s *Service) buildOverview() OverviewStats {
	s.overviewMu.Lock()
	if time.Since(s.overview.at) < overviewCacheTTL {
		st := s.overview.stats
		s.overviewMu.Unlock()
		return st
	}
	s.overviewMu.Unlock()

	st := s.buildOverviewFresh()

	s.overviewMu.Lock()
	s.overview = overviewCache{at: time.Now(), stats: st}
	s.overviewMu.Unlock()
	return st
}

func (s *Service) buildOverviewFresh() OverviewStats {
	st := OverviewStats{}
	if s.db == nil {
		return st
	}

	var n int64
	s.db.Model(&models.Website{}).Count(&n)
	st.Websites = int(n)
	s.db.Model(&models.Website{}).Where("status = ?", "running").Count(&n)
	st.WebsitesRunning = int(n)

	s.db.Model(&models.DatabaseInstance{}).Count(&n)
	st.Databases = int(n)

	s.db.Model(&models.CronJob{}).Count(&n)
	st.CronJobs = int(n)

	s.db.Model(&models.FTPAccount{}).Count(&n)
	st.FTPAccounts = int(n)

	s.db.Model(&models.WordPressSite{}).Count(&n)
	st.WordPressSites = int(n)

	s.db.Model(&models.SSLCertificate{}).Count(&n)
	st.SSLTotal = int(n)

	soon := time.Now().Add(30 * 24 * time.Hour)
	s.db.Model(&models.SSLCertificate{}).Where("expires_at IS NOT NULL AND expires_at <= ?", soon).Count(&n)
	st.SSLExpiringSoon = int(n)

	s.db.Model(&models.App{}).Where("installed = ?", true).Count(&n)
	st.SoftwareInstalled = int(n)
	s.db.Model(&models.App{}).Where("installed = ? AND status = ?", true, "running").Count(&n)
	st.SoftwareRunning = int(n)

	if containers, err := docker.NewService(nil, "").ListContainers(); err == nil {
		st.DockerTotal = len(containers)
		for _, c := range containers {
			if isDockerRunning(c.Status) {
				st.DockerRunning++
			}
		}
	}

	return st
}

func isDockerRunning(status string) bool {
	return strings.HasPrefix(status, "running") || strings.HasPrefix(status, "Up")
}

func rateDelta(current, prev uint64, dt float64) float64 {
	if current < prev {
		return float64(current) / dt
	}
	return float64(current-prev) / dt
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
