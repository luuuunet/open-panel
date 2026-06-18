package bastion

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/luuuunet/owpanel/internal/services/sshmgr"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

type Service struct {
	db      *gorm.DB
	dataDir string
	secret  string
	sshmgr  *sshmgr.Service

	commands *CommandFilter

	activeMu sync.RWMutex
	active   map[string]*liveSession

	opsCron    *cron.Cron
	opsEntries map[uint]cron.EntryID
	opsSchedMu sync.Mutex

	onSyslog func(eventType, message string)
}

type liveSession struct {
	SessionKey string
	UserID     uint
	AssetID    uint
	Username   string
	Host       string
	Kill       func() // closes websocket / ssh
}

func NewService(db *gorm.DB, dataDir, secret string, ssh *sshmgr.Service) *Service {
	s := &Service{
		db:         db,
		dataDir:    dataDir,
		secret:     secret,
		sshmgr:     ssh,
		active:     make(map[string]*liveSession),
		opsEntries: map[uint]cron.EntryID{},
	}
	_ = os.MkdirAll(filepath.Join(dataDir, "bastion", "sessions"), 0755)
	s.initCommands()
	s.initOps()
	s.initAccounts()
	s.initAccessRequests()
	return s
}

func (s *Service) SetSyslogEmitter(fn func(eventType, message string)) {
	s.onSyslog = fn
}

func (s *Service) emitSyslog(eventType, message string) {
	if s.onSyslog != nil {
		s.onSyslog(eventType, message)
	}
}

func (s *Service) sessionsDir() string {
	return filepath.Join(s.dataDir, "bastion", "sessions")
}
