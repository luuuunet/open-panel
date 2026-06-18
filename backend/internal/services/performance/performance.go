package performance

import (
	"sync"
	"time"

	"github.com/luuuunet/owpanel/internal/services/settings"
)

const settingKey = "power_save_enabled"

const (
	dashboardPresenceTTL = 90 * time.Second
	liveCollectSec       = 10
	idleCollectSec       = 120
	idleCollectSecPower  = 180
	liveTrafficPollSec   = 30
	idleTrafficPollSec   = 300
)

// Profile describes polling/collection intervals exposed to the dashboard UI.
type Profile struct {
	Enabled         bool `json:"enabled"`
	DashboardLive   bool `json:"dashboard_live"`
	CollectSec      int  `json:"collect_sec"`
	IdleCollectSec  int  `json:"idle_collect_sec"`
	MonitorLiteSec  int  `json:"monitor_lite_sec"`
	MonitorFullSec  int  `json:"monitor_full_sec"`
	TrafficPollSec  int  `json:"traffic_poll_sec"`
	TrafficMapSec   int  `json:"traffic_map_sec"`
	ClusterSyncSec  int  `json:"cluster_sync_sec"`
	UptimeScanSec   int  `json:"uptime_scan_sec"`
}

var normalProfile = Profile{
	Enabled:        false,
	CollectSec:     15,
	MonitorLiteSec: 15,
	MonitorFullSec: 60,
	TrafficPollSec: 30,
	TrafficMapSec:  90,
	ClusterSyncSec: 60,
	UptimeScanSec:  15,
}

var powerSaveProfile = Profile{
	Enabled:        true,
	CollectSec:     60,
	MonitorLiteSec: 60,
	MonitorFullSec: 120,
	TrafficPollSec: 120,
	TrafficMapSec:  180,
	ClusterSyncSec: 300,
	UptimeScanSec:  60,
}

type Service struct {
	settings *settings.Service

	presenceMu      sync.Mutex
	dashboardLiveAt time.Time
}

func NewService(settingsSvc *settings.Service) *Service {
	return &Service{settings: settingsSvc}
}

// TouchDashboardLive marks the dashboard as actively viewed (monitor/traffic-map polling).
func (s *Service) TouchDashboardLive() {
	s.presenceMu.Lock()
	s.dashboardLiveAt = time.Now()
	s.presenceMu.Unlock()
}

// DashboardLive is true while a client has polled dashboard APIs recently.
func (s *Service) DashboardLive() bool {
	s.presenceMu.Lock()
	at := s.dashboardLiveAt
	s.presenceMu.Unlock()
	return !at.IsZero() && time.Since(at) < dashboardPresenceTTL
}

func (s *Service) profileBase() Profile {
	if s.Enabled() {
		return powerSaveProfile
	}
	return normalProfile
}

func (s *Service) GetProfile() Profile {
	p := s.profileBase()
	p.DashboardLive = s.DashboardLive()
	if p.DashboardLive {
		p.CollectSec = liveCollectSec
		if s.Enabled() {
			p.CollectSec = 15
		}
	} else {
		p.CollectSec = idleCollectSec
		if s.Enabled() {
			p.CollectSec = idleCollectSecPower
		}
	}
	p.IdleCollectSec = idleCollectSec
	if s.Enabled() {
		p.IdleCollectSec = idleCollectSecPower
	}
	return p
}

func (s *Service) Enabled() bool {
	if s.settings == nil {
		return false
	}
	all, err := s.settings.GetAll()
	if err != nil {
		return false
	}
	return all[settingKey] == "true"
}

func (s *Service) SetEnabled(on bool) error {
	val := "false"
	if on {
		val = "true"
	}
	return s.settings.Update(map[string]string{settingKey: val})
}

func (s *Service) CollectInterval() time.Duration {
	return time.Duration(s.GetProfile().CollectSec) * time.Second
}

func (s *Service) TrafficPollInterval() time.Duration {
	if !s.DashboardLive() {
		return idleTrafficPollSec * time.Second
	}
	sec := s.profileBase().TrafficPollSec
	if sec <= 0 {
		sec = liveTrafficPollSec
	}
	return time.Duration(sec) * time.Second
}

func (s *Service) ClusterSyncInterval() time.Duration {
	sec := s.GetProfile().ClusterSyncSec
	if sec <= 0 {
		sec = normalProfile.ClusterSyncSec
	}
	return time.Duration(sec) * time.Second
}

func (s *Service) UptimeScanInterval() time.Duration {
	sec := s.GetProfile().UptimeScanSec
	if sec <= 0 {
		sec = normalProfile.UptimeScanSec
	}
	return time.Duration(sec) * time.Second
}

func (s *Service) TrafficMapCacheInterval() time.Duration {
	sec := s.GetProfile().TrafficMapSec
	if sec <= 0 {
		sec = normalProfile.TrafficMapSec
	}
	return time.Duration(sec) * time.Second
}
