package analytics

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/performance"
	"github.com/luuuunet/owpanel/internal/services/waf"
	"github.com/oschwald/geoip2-golang"
	"gorm.io/gorm"
)

var (
	ipLineRe     = regexp.MustCompile(`^(\d{1,3}(?:\.\d{1,3}){3})`)
	nginxCombRe  = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) (\S+) [^"]*" (\d+) (\d+)(?: "([^"]*)")?`)
	bytesFieldRe = regexp.MustCompile(`\b(\d+)\b`)
)

type Service struct {
	db       *gorm.DB
	dataDir  string
	waf      *waf.Service
	perf     *performance.Service
	mu       sync.Mutex
	offsets  map[string]int64
	geoDB    *geoip2.Reader
	geoDBMu  sync.RWMutex
	stopCh   chan struct{}

	mapCacheMu sync.Mutex
	mapCache   map[int]trafficMapCacheEntry
	lastPrune  time.Time
	offsetStore *offsetStore
}

type trafficMapCacheEntry struct {
	at   time.Time
	resp *TrafficMapResponse
}

const trafficMapCacheTTL = 60 * time.Second
const trafficKeepDays = 1
const trafficPruneBatch = 10000
const maxIngestLinesPerFile = 3000
const maxMapCacheEntries = 8
const maxInitialIngestBytes int64 = 5 * 1024 * 1024
const tailIngestBytes int64 = 512 * 1024

func NewService(db *gorm.DB, dataDir string, wafSvc *waf.Service, perf *performance.Service) *Service {
	s := &Service{
		db:      db,
		dataDir: dataDir,
		waf:     wafSvc,
		perf:    perf,
		offsets: make(map[string]int64),
		stopCh:  make(chan struct{}),
		mapCache: make(map[int]trafficMapCacheEntry),
		offsetStore: newOffsetStore(dataDir),
	}
	if saved := s.offsetStore.load(); len(saved) > 0 {
		s.offsets = saved
	}
	s.reloadGeoDB()
	go s.pollLoop()
	go s.startupMaintenance()
	return s
}

func (s *Service) Stop() {
	close(s.stopCh)
	s.geoDBMu.Lock()
	if s.geoDB != nil {
		s.geoDB.Close()
		s.geoDB = nil
	}
	s.geoDBMu.Unlock()
}

func (s *Service) ReloadGeoDB() {
	s.reloadGeoDB()
}

func (s *Service) InstallGeoIP() (*waf.GeoIPInstallResult, error) {
	if s.waf == nil {
		return nil, fmt.Errorf("waf service unavailable")
	}
	result, err := s.waf.InstallGeoDB()
	if err != nil {
		return nil, err
	}
	s.ReloadGeoDB()
	return result, nil
}

func (s *Service) reloadGeoDB() {
	s.geoDBMu.Lock()
	defer s.geoDBMu.Unlock()
	if s.geoDB != nil {
		s.geoDB.Close()
		s.geoDB = nil
	}
	paths := s.geoDBPaths()
	for _, p := range paths {
		if p == "" {
			continue
		}
		if db, err := geoip2.Open(p); err == nil {
			s.geoDB = db
			return
		}
	}
}

func (s *Service) geoDBPaths() []string {
	var paths []string
	city := filepath.Join(s.dataDir, "security", "GeoLite2-City.mmdb")
	country := filepath.Join(s.dataDir, "security", "GeoLite2-Country.mmdb")
	paths = append(paths, city, country)
	if s.waf != nil {
		if cfg, err := s.waf.GetConfig(); err == nil && cfg != nil {
			if cfg.GeoDbPath != "" {
				paths = append([]string{cfg.GeoDbPath}, paths...)
			}
		}
	}
	return paths
}

func (s *Service) lookup(ipStr string) (code, name, city string, lat, lng float64) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "", "", "", 0, 0
	}
	s.geoDBMu.RLock()
	db := s.geoDB
	s.geoDBMu.RUnlock()
	if db == nil {
		return "", "", "", 0, 0
	}
	if rec, err := db.City(ip); err == nil && rec.Country.IsoCode != "" {
		code = rec.Country.IsoCode
		name = rec.Country.Names["en"]
		city = rec.City.Names["en"]
		lat = rec.Location.Latitude
		lng = rec.Location.Longitude
		if lat != 0 || lng != 0 {
			return
		}
	}
	if rec, err := db.Country(ip); err == nil && rec.Country.IsoCode != "" {
		code = rec.Country.IsoCode
		name = rec.Country.Names["en"]
		lat, lng = countryCentroid(code)
	}
	return
}

func (s *Service) pollLoop() {
	s.pruneOldTrafficHits(false)
	s.ingestAll()
	for {
		interval := 30 * time.Second
		if s.perf != nil {
			interval = s.perf.TrafficPollInterval()
		}
		timer := time.NewTimer(interval)
		select {
		case <-timer.C:
			timer.Stop()
			s.pruneOldTrafficHits(false)
			s.ingestAll()
		case <-s.stopCh:
			timer.Stop()
			return
		}
	}
}

func (s *Service) startupMaintenance() {
	go s.ensureGeoIP()
	time.Sleep(3 * time.Second)
	s.pruneOldTrafficHits(true)
	s.dedupeTrafficHits()
	if s.db != nil {
		_ = s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error
	}
	go func() {
		time.Sleep(30 * time.Second)
		var n int64
		s.db.Model(&models.TrafficHit{}).Count(&n)
		if n > 200000 {
			s.dedupeTrafficHits()
			s.pruneOldTrafficHits(true)
			_ = s.db.Exec("VACUUM").Error
			_ = s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error
		}
	}()
}

func (s *Service) ensureGeoIP() {
	time.Sleep(2 * time.Second)
	if s.geoDBReady() {
		return
	}
	if _, err := s.InstallGeoIP(); err != nil {
		log.Printf("OWPanel: GeoLite2 auto-install skipped: %v", err)
		return
	}
	log.Printf("OWPanel: GeoLite2 database ready for traffic map")
}

// RecordHTTPAccess stores a live panel HTTP hit for the traffic map.
func (s *Service) RecordHTTPAccess(ip, method, path string, status int, bytes uint64, referer, userAgent string) {
	if s.db == nil || isPrivateIP(ip) {
		return
	}
	code, name, city, lat, lng := s.lookup(ip)
	if code == "" {
		code = "XX"
		name = "Unknown"
	}
	if lat == 0 && lng == 0 && code != "XX" {
		lat, lng = countryCentroid(code)
	}
	source := filepath.Join(s.dataDir, "logs", "panel_access.log")
	hit := models.TrafficHit{
		CreatedAt:   time.Now(),
		IP:          ip,
		CountryCode: code,
		CountryName: mapCountryName(code, name),
		City:        city,
		Latitude:    lat,
		Longitude:   lng,
		Bytes:       bytes,
		Status:      status,
		Method:      method,
		Path:        truncate(path, 500),
		Host:        "panel",
		Referer:     truncate(referer, 500),
		UserAgent:   truncate(userAgent, 500),
		LogSource:   source,
	}
	if err := s.db.Create(&hit).Error; err != nil {
		return
	}
	s.mapCacheMu.Lock()
	s.mapCache = make(map[int]trafficMapCacheEntry)
	s.mapCacheMu.Unlock()
}

func (s *Service) dedupeTrafficHits() {
	if s.db == nil {
		return
	}
	// Remove duplicate rows re-imported after restarts (same log line fingerprint).
	_ = s.db.Exec(`
		DELETE FROM traffic_hits
		WHERE id NOT IN (
			SELECT MIN(id) FROM traffic_hits
			GROUP BY ip, host, path, method, status, bytes, log_source,
				strftime('%s', created_at) / 60
		)
	`).Error
}

func (s *Service) pruneOldTrafficHits(force bool) {
	if s.db == nil {
		return
	}
	now := time.Now()
	s.mapCacheMu.Lock()
	if !force && !s.lastPrune.IsZero() && now.Sub(s.lastPrune) < time.Hour {
		s.mapCacheMu.Unlock()
		return
	}
	s.lastPrune = now
	s.mapCacheMu.Unlock()

	cutoff := now.AddDate(0, 0, -trafficKeepDays)
	for i := 0; i < 100; i++ {
		res := s.db.Where("created_at < ?", cutoff).Limit(trafficPruneBatch).Delete(&models.TrafficHit{})
		if res.Error != nil || res.RowsAffected == 0 {
			break
		}
	}
}

func (s *Service) logPaths() []string {
	seen := map[string]bool{}
	var paths []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			return
		}
		base := strings.ToLower(filepath.Base(p))
		if strings.Contains(base, "security") || strings.Contains(base, "error") {
			return
		}
		seen[p] = true
		paths = append(paths, p)
	}
	logDir := filepath.Join(s.dataDir, "logs")
	if entries, err := os.ReadDir(logDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := strings.ToLower(e.Name())
			if !strings.HasSuffix(name, ".log") {
				continue
			}
			add(filepath.Join(logDir, e.Name()))
		}
	}
	add(filepath.Join(s.dataDir, "logs", "panel_access.log"))
	add(filepath.Join(s.dataDir, "logs", "access.log"))
	add(filepath.Join(s.dataDir, "server", "nginx", "logs", "access.log"))
	add(filepath.Join(s.dataDir, "server", "openresty", "nginx", "logs", "access.log"))
	add("/var/log/nginx/access.log")
	add("/usr/local/nginx/logs/access.log")
	add("/usr/local/openresty/nginx/logs/access.log")
	return paths
}

func (s *Service) ingestAll() {
	for _, p := range s.logPaths() {
		s.ingestFile(p)
	}
}

func (s *Service) ingestFile(path string) {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	s.mu.Lock()
	offset := s.offsets[path]
	known := offset > 0
	s.mu.Unlock()

	if !known {
		if st.Size() <= maxInitialIngestBytes {
			offset = 0
		} else {
			offset = st.Size() - tailIngestBytes
			if offset < 0 {
				offset = 0
			}
		}
	} else if offset > st.Size() {
		offset = 0
	}

	if _, err := f.Seek(offset, 0); err != nil {
		return
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var hits []models.TrafficHit
	linesRead := 0
	for sc.Scan() {
		if linesRead >= maxIngestLinesPerFile {
			break
		}
		if hit, ok := s.parseLine(sc.Text(), path); ok {
			hits = append(hits, hit)
		}
		linesRead++
	}
	newOffset, _ := f.Seek(0, 1)

	if len(hits) > 0 {
		const batch = 200
		for i := 0; i < len(hits); i += batch {
			end := i + batch
			if end > len(hits) {
				end = len(hits)
			}
			_ = s.db.Create(hits[i:end]).Error
		}
	}

	s.mu.Lock()
	s.offsets[path] = newOffset
	s.mu.Unlock()
	s.offsetStore.markDirty()
	s.offsetStore.save(s.offsets)
}

func (s *Service) parseLine(line, source string) (models.TrafficHit, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return models.TrafficHit{}, false
	}

	ip, method, path, status, bytes := "", "GET", "/", 200, uint64(0)
	referer := ""
	if m := nginxCombRe.FindStringSubmatch(line); len(m) >= 7 {
		ip, method, path, status, bytes = m[1], m[3], m[4], parseInt(m[5]), parseUint(m[6])
		if len(m) >= 8 {
			referer = m[7]
		}
	} else if m := ipLineRe.FindStringSubmatch(line); len(m) >= 2 {
		ip = m[1]
		if parts := bytesFieldRe.FindAllString(line, -1); len(parts) >= 2 {
			bytes = parseUint(parts[len(parts)-2])
			status = parseInt(parts[len(parts)-3])
		}
	} else {
		return models.TrafficHit{}, false
	}
	if ip == "" || isPrivateIP(ip) {
		return models.TrafficHit{}, false
	}

	code, name, city, lat, lng := s.lookup(ip)
	if code == "" {
		code = "XX"
		name = "Unknown"
	}
	if lat == 0 && lng == 0 && code != "XX" {
		lat, lng = countryCentroid(code)
	}

	return models.TrafficHit{
		CreatedAt:   time.Now(),
		IP:          ip,
		CountryCode: code,
		CountryName: mapCountryName(code, name),
		City:        city,
		Latitude:    lat,
		Longitude:   lng,
		Bytes:       bytes,
		Status:      status,
		Method:      method,
		Path:        truncate(path, 500),
		Host:        hostFromLogSource(source),
		Referer:     truncate(referer, 500),
		LogSource:   source,
	}, true
}

func hostFromLogSource(source string) string {
	base := strings.ToLower(filepath.Base(source))
	base = strings.TrimSuffix(base, ".log")
	for _, suffix := range []string{"_ssl_access", "_access"} {
		if strings.HasSuffix(base, suffix) {
			return strings.TrimSuffix(base, suffix)
		}
	}
	return ""
}

func parseInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func parseUint(s string) uint64 {
	n, _ := strconv.ParseUint(s, 10, 64)
	return n
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	return false
}
