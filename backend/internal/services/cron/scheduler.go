package cron

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

const defaultJobTimeout = 30 * time.Minute

type JobView struct {
	models.CronJob
	NextRunAt *time.Time `json:"next_run_at,omitempty"`
	Executor  string     `json:"executor,omitempty"`
}

type Service struct {
	db      *gorm.DB
	dataDir string
	cron    *cron.Cron
	mu      sync.Mutex
	entries map[uint]cron.EntryID

	systemCrontabActive bool
	running             map[uint]bool

	onFailure func(job models.CronJob, message string)
}

func NewService(db *gorm.DB, dataDir string) *Service {
	return &Service{
		db:      db,
		dataDir: dataDir,
		cron:    cron.New(cron.WithSeconds()),
		entries: map[uint]cron.EntryID{},
		running: map[uint]bool{},
	}
}

func (s *Service) SetFailureHook(h func(job models.CronJob, message string)) {
	s.onFailure = h
}

func (s *Service) Start() {
	s.cron.Start()
	s.ReloadAll()
}

func (s *Service) List() ([]JobView, error) {
	var jobs []models.CronJob
	if err := s.db.Order("id desc").Find(&jobs).Error; err != nil {
		return nil, err
	}
	out := make([]JobView, 0, len(jobs))
	for i := range jobs {
		out = append(out, s.enrichJob(&jobs[i]))
	}
	return out, nil
}

func (s *Service) enrichJob(job *models.CronJob) JobView {
	v := JobView{CronJob: *job}
	if s.systemCrontabActive {
		v.Executor = "system_crontab"
	} else {
		v.Executor = "panel_scheduler"
	}
	if job.Enabled {
		if next, err := nextRunTime(job.Schedule); err == nil {
			v.NextRunAt = &next
		}
	}
	return v
}

func validateSchedule(schedule string) error {
	spec := normalizeSchedule(strings.TrimSpace(schedule))
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.DowOptional)
	if _, err := parser.Parse(spec); err != nil {
		return fmt.Errorf("无效的 Cron 表达式: %w", err)
	}
	return nil
}

func nextRunTime(schedule string) (time.Time, error) {
	spec := normalizeSchedule(strings.TrimSpace(schedule))
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.DowOptional)
	sch, err := parser.Parse(spec)
	if err != nil {
		return time.Time{}, err
	}
	return sch.Next(time.Now()), nil
}

func (s *Service) Create(job *models.CronJob) error {
	job.Schedule = strings.TrimSpace(job.Schedule)
	job.Command = strings.TrimSpace(job.Command)
	if job.Name == "" || job.Command == "" {
		return fmt.Errorf("名称和命令不能为空")
	}
	if err := validateSchedule(job.Schedule); err != nil {
		return err
	}
	if err := s.db.Create(job).Error; err != nil {
		return err
	}
	return s.applyScheduling(job)
}

func (s *Service) Update(id uint, patch models.CronJob) (*models.CronJob, error) {
	var job models.CronJob
	if err := s.db.First(&job, id).Error; err != nil {
		return nil, err
	}
	if name := strings.TrimSpace(patch.Name); name != "" {
		job.Name = name
	}
	if sched := strings.TrimSpace(patch.Schedule); sched != "" {
		if err := validateSchedule(sched); err != nil {
			return nil, err
		}
		job.Schedule = sched
	}
	if cmd := strings.TrimSpace(patch.Command); cmd != "" {
		job.Command = cmd
	}
	if err := s.db.Save(&job).Error; err != nil {
		return nil, err
	}
	if err := s.applyScheduling(&job); err != nil {
		return &job, err
	}
	return &job, nil
}

func (s *Service) Delete(id uint) error {
	s.unschedule(id)
	if err := s.db.Delete(&models.CronJob{}, id).Error; err != nil {
		return err
	}
	return s.syncSystemCrontab()
}

func (s *Service) Toggle(id uint, enabled bool) error {
	if err := s.db.Model(&models.CronJob{}).Where("id = ?", id).Update("enabled", enabled).Error; err != nil {
		return err
	}
	var job models.CronJob
	if err := s.db.First(&job, id).Error; err != nil {
		return err
	}
	return s.applyScheduling(&job)
}

func (s *Service) reschedule(job *models.CronJob) {
	s.unschedule(job.ID)
	s.mu.Lock()
	active := s.systemCrontabActive
	s.mu.Unlock()
	if job.Enabled && !active {
		s.scheduleJob(job)
	}
}

func (s *Service) applyScheduling(job *models.CronJob) error {
	s.unschedule(job.ID)
	if err := s.syncSystemCrontab(); err != nil {
		return err
	}
	s.mu.Lock()
	active := s.systemCrontabActive
	s.mu.Unlock()
	if job.Enabled && !active {
		s.scheduleJob(job)
	}
	return nil
}

func (s *Service) RunNow(id uint) error {
	var job models.CronJob
	if err := s.db.First(&job, id).Error; err != nil {
		return err
	}
	go s.executeJob(&job)
	return nil
}

func (s *Service) GetLog(id uint) (string, error) {
	data, err := os.ReadFile(s.logPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func (s *Service) ReloadAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, eid := range s.entries {
		s.cron.Remove(eid)
		delete(s.entries, id)
	}
	_ = s.syncSystemCrontabLocked()
	if s.systemCrontabActive {
		return
	}
	var jobs []models.CronJob
	if err := s.db.Where("enabled = ?", true).Find(&jobs).Error; err != nil {
		return
	}
	for i := range jobs {
		s.scheduleJobLocked(&jobs[i])
	}
}

func (s *Service) scheduleJob(job *models.CronJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scheduleJobLocked(job)
}

func (s *Service) scheduleJobLocked(job *models.CronJob) {
	s.unscheduleLocked(job.ID)
	if !job.Enabled || s.systemCrontabActive {
		if s.systemCrontabActive && job.Enabled {
			s.db.Model(job).Updates(map[string]interface{}{
				"sync_status":  "synced",
				"sync_message": "/etc/cron.d/owpanel",
			})
		}
		return
	}
	spec := normalizeSchedule(job.Schedule)
	j := *job
	eid, err := s.cron.AddFunc(spec, func() { s.executeJob(&j) })
	if err != nil {
		s.db.Model(job).Updates(map[string]interface{}{
			"sync_status":  "error",
			"sync_message": err.Error(),
		})
		return
	}
	s.entries[job.ID] = eid
	s.db.Model(job).Updates(map[string]interface{}{
		"sync_status":  "synced",
		"sync_message": "panel internal scheduler",
	})
}

func (s *Service) unschedule(id uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unscheduleLocked(id)
}

func (s *Service) unscheduleLocked(id uint) {
	if eid, ok := s.entries[id]; ok {
		s.cron.Remove(eid)
		delete(s.entries, id)
	}
}

func normalizeSchedule(schedule string) string {
	schedule = strings.TrimSpace(schedule)
	parts := strings.Fields(schedule)
	if len(parts) == 5 {
		return "0 " + schedule
	}
	return schedule
}

func (s *Service) executeJob(job *models.CronJob) {
	s.mu.Lock()
	if s.running[job.ID] {
		s.mu.Unlock()
		s.db.Model(job).Updates(map[string]interface{}{
			"last_status": "skipped",
			"last_output": "上次执行尚未结束，已跳过",
		})
		return
	}
	s.running[job.ID] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.running, job.ID)
		s.mu.Unlock()
	}()

	now := time.Now()
	s.db.Model(job).Updates(map[string]interface{}{
		"last_run_at": now,
		"last_status": "running",
	})
	logPath := s.logPath(job.ID)
	_ = os.MkdirAll(filepath.Dir(logPath), 0755)

	ctx, cancel := context.WithTimeout(context.Background(), defaultJobTimeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", job.Command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", job.Command)
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	out := strings.TrimSpace(stdout.String())
	if stderr.Len() > 0 {
		if out != "" {
			out += "\n"
		}
		out += strings.TrimSpace(stderr.String())
	}
	if ctx.Err() == context.DeadlineExceeded {
		if out != "" {
			out += "\n"
		}
		out += fmt.Sprintf("任务超时（超过 %v）", defaultJobTimeout)
		err = ctx.Err()
	}
	if len(out) > 8000 {
		out = out[len(out)-8000:]
	}
	_ = os.WriteFile(logPath, []byte(out), 0644)

	status := "success"
	msg := out
	if err != nil {
		status = "failed"
		if msg == "" {
			msg = err.Error()
		} else {
			msg = err.Error() + "\n" + msg
		}
	}
	if len(msg) > 1000 {
		msg = msg[:1000]
	}
	s.db.Model(job).Updates(map[string]interface{}{
		"last_status": status,
		"last_output": msg,
	})
	if status == "failed" && s.onFailure != nil {
		s.onFailure(*job, msg)
	}
}

func (s *Service) logPath(id uint) string {
	return filepath.Join(s.dataDir, "cron", "logs", fmt.Sprintf("job-%d.log", id))
}

func (s *Service) syncSystemCrontab() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.syncSystemCrontabLocked()
}

func (s *Service) syncSystemCrontabLocked() error {
	if runtime.GOOS == "windows" {
		s.systemCrontabActive = false
		return nil
	}
	var jobs []models.CronJob
	if err := s.db.Where("enabled = ?", true).Order("id asc").Find(&jobs).Error; err != nil {
		return err
	}
	dir := filepath.Join(s.dataDir, "cron")
	_ = os.MkdirAll(dir, 0755)
	var b strings.Builder
	b.WriteString("# OWPanel managed cron jobs — do not edit manually\n")
	b.WriteString("SHELL=/bin/bash\nPATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\n\n")
	for _, job := range jobs {
		logPath := s.logPath(job.ID)
		line := fmt.Sprintf("%s root %s >> %s 2>&1 # owpanel:%d:%s\n",
			strings.TrimSpace(job.Schedule), job.Command, logPath, job.ID, job.Name)
		b.WriteString(line)
	}
	cronFile := filepath.Join(dir, "owpanel.crontab")
	content := []byte(b.String())
	if err := os.WriteFile(cronFile, content, 0644); err != nil {
		return err
	}

	systemFile := "/etc/cron.d/owpanel"
	if err := os.WriteFile(systemFile, content, 0644); err == nil {
		s.systemCrontabActive = true
		for _, job := range jobs {
			s.db.Model(&models.CronJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
				"sync_status":  "synced",
				"sync_message": "/etc/cron.d/owpanel",
			})
		}
		// 系统 crontab 生效时移除内置调度，避免重复执行
		for id, eid := range s.entries {
			s.cron.Remove(eid)
			delete(s.entries, id)
		}
		return nil
	}

	s.systemCrontabActive = false
	for _, job := range jobs {
		s.db.Model(&models.CronJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
			"sync_status":  "synced",
			"sync_message": cronFile + " (internal scheduler)",
		})
	}
	return nil
}

func (s *Service) DataDir() string { return s.dataDir }

func (s *Service) SchedulerMode() string {
	if s.systemCrontabActive {
		return "system_crontab"
	}
	return "panel_scheduler"
}
