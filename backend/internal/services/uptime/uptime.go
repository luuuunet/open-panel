package uptime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/performance"
	"gorm.io/gorm"
)

type Service struct {
	db     *gorm.DB
	client *http.Client
	mu     sync.Mutex
	stop   chan struct{}
	perf   *performance.Service
}

func NewService(db *gorm.DB, perf *performance.Service) *Service {
	return &Service{
		db:     db,
		client: &http.Client{Timeout: 30 * time.Second},
		stop:   make(chan struct{}),
		perf:   perf,
	}
}

func (s *Service) Start() {
	go s.loop()
}

func (s *Service) Stop() {
	close(s.stop)
}

func (s *Service) List() ([]models.UptimeMonitor, error) {
	var list []models.UptimeMonitor
	err := s.db.Order("id desc").Find(&list).Error
	return list, err
}

func (s *Service) Create(m *models.UptimeMonitor) error {
	if m.Method == "" {
		m.Method = "GET"
	}
	if m.IntervalSec <= 0 {
		m.IntervalSec = 60
	}
	if m.TimeoutSec <= 0 {
		m.TimeoutSec = 10
	}
	if m.ExpectedStatus == 0 {
		m.ExpectedStatus = 200
	}
	if err := s.db.Create(m).Error; err != nil {
		return err
	}
	go s.checkOne(m)
	return nil
}

func (s *Service) Update(id uint, updates map[string]interface{}) error {
	return s.db.Model(&models.UptimeMonitor{}).Where("id = ?", id).Updates(updates).Error
}

func (s *Service) Delete(id uint) error {
	return s.db.Delete(&models.UptimeMonitor{}, id).Error
}

func (s *Service) CheckNow(id uint) (*models.UptimeMonitor, error) {
	var m models.UptimeMonitor
	if err := s.db.First(&m, id).Error; err != nil {
		return nil, err
	}
	s.runCheck(&m)
	if err := s.db.First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Service) loop() {
	for {
		interval := 15 * time.Second
		if s.perf != nil {
			interval = s.perf.UptimeScanInterval()
		}
		timer := time.NewTimer(interval)
		select {
		case <-s.stop:
			timer.Stop()
			return
		case <-timer.C:
			timer.Stop()
			s.scanDue()
		}
	}
}

func (s *Service) scanDue() {
	var list []models.UptimeMonitor
	if err := s.db.Where("enabled = ?", true).Find(&list).Error; err != nil {
		return
	}
	now := time.Now()
	for i := range list {
		m := &list[i]
		if m.LastCheckAt != nil {
			next := m.LastCheckAt.Add(time.Duration(m.IntervalSec) * time.Second)
			if now.Before(next) {
				continue
			}
		}
		s.runCheck(m)
	}
}

func (s *Service) checkOne(m *models.UptimeMonitor) {
	s.runCheck(m)
}

func (s *Service) runCheck(m *models.UptimeMonitor) {
	prevStatus := m.LastStatus
	status, latency, errMsg := s.probe(m)
	now := time.Now()

	updates := map[string]interface{}{
		"last_check_at":   now,
		"last_status":     status,
		"last_latency_ms": latency,
		"last_error":      errMsg,
	}
	if status == "down" {
		updates["fail_count"] = m.FailCount + 1
	} else if status == "up" {
		updates["fail_count"] = 0
	}
	s.db.Model(m).Updates(updates)

	if prevStatus != "" && prevStatus != "unknown" && prevStatus != status && strings.TrimSpace(m.NotifyWebhook) != "" {
		s.notifyWebhook(m, status, errMsg)
	}
}

func (s *Service) probe(m *models.UptimeMonitor) (status string, latencyMs int, errMsg string) {
	method := strings.ToUpper(strings.TrimSpace(m.Method))
	if method == "" {
		method = "GET"
	}
	timeout := time.Duration(m.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(method, m.URL, nil)
	if err != nil {
		return "down", 0, err.Error()
	}
	start := time.Now()
	resp, err := client.Do(req)
	latencyMs = int(time.Since(start).Milliseconds())
	if err != nil {
		return "down", latencyMs, err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if m.ExpectedStatus > 0 && resp.StatusCode != m.ExpectedStatus {
		return "down", latencyMs, fmt.Sprintf("expected HTTP %d, got %d", m.ExpectedStatus, resp.StatusCode)
	}
	if kw := strings.TrimSpace(m.Keyword); kw != "" && !strings.Contains(string(body), kw) {
		return "down", latencyMs, fmt.Sprintf("keyword %q not found", kw)
	}
	return "up", latencyMs, ""
}

func (s *Service) notifyWebhook(m *models.UptimeMonitor, status, errMsg string) {
	payload := map[string]interface{}{
		"event":     "uptime_status_change",
		"monitor":   m.Name,
		"url":       m.URL,
		"status":    status,
		"error":     errMsg,
		"timestamp": time.Now().Format(time.RFC3339),
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, m.NotifyWebhook, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}
