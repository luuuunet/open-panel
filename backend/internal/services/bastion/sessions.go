package bastion

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/luuuunet/owpanel/internal/models"
)

type SessionRecorder struct {
	service    *Service
	sessionKey string
	dbID       uint
	assetID    uint
	accountID  uint
	assetName  string
	username   string
	logFile    *os.File
	inputBuf   strings.Builder
	commands   []string
	cmdMu      sync.Mutex
	readonly   bool
}

func (s *Service) StartRecorder(userID uint, username string, assetID uint, accountID uint, assetName, host string, port int, readonly bool) (*SessionRecorder, error) {
	key := uuid.New().String()
	logPath := filepath.Join(s.sessionsDir(), key+".log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return nil, err
	}
	var aid *uint
	if assetID > 0 {
		aid = &assetID
	}
	var acctID *uint
	if accountID > 0 {
		acctID = &accountID
	}
	rec := models.BastionSession{
		SessionKey: key, UserID: userID, Username: username,
		AssetID: aid, AccountID: acctID, AssetName: assetName, Host: host, Port: port,
		StartTime: time.Now(), LogPath: logPath, Status: "active",
	}
	if err := s.db.Create(&rec).Error; err != nil {
		f.Close()
		return nil, err
	}
	r := &SessionRecorder{
		service: s, sessionKey: key, dbID: rec.ID,
		assetID: assetID, accountID: accountID, assetName: assetName, username: username,
		logFile: f, readonly: readonly,
	}
	s.activeMu.Lock()
	s.active[key] = &liveSession{
		SessionKey: key, UserID: userID, AssetID: assetID,
		Username: username, Host: host,
	}
	s.activeMu.Unlock()
	s.emitSyslog("session_start", fmt.Sprintf("user=%s asset=%s host=%s", username, assetName, host))
	return r, nil
}

func (r *SessionRecorder) SessionKey() string { return r.sessionKey }

func (r *SessionRecorder) SetKill(fn func()) {
	r.service.activeMu.Lock()
	if ls, ok := r.service.active[r.sessionKey]; ok {
		ls.Kill = fn
	}
	r.service.activeMu.Unlock()
}

func (r *SessionRecorder) WriteOutput(data []byte) {
	if len(data) == 0 {
		return
	}
	_, _ = r.logFile.Write(data)
}

func (r *SessionRecorder) ProcessInput(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}
	_, _ = r.logFile.Write([]byte("\n[stdin] "))
	_, _ = r.logFile.Write(data)

	if r.readonly {
		// block all interactive input except resize handled elsewhere
		if isControlOnly(data) {
			return data, nil
		}
		r.service.logBlocked(r.dbID, r.sessionKey, string(data), "blocked", 0, "")
		return nil, fmt.Errorf("只读会话：禁止输入")
	}

	filtered, blocked, cmd := r.service.commands.FilterInput(&r.inputBuf, data)
	if blocked {
		r.service.logBlocked(r.dbID, r.sessionKey, cmd, "blocked", 0, "")
		return nil, fmt.Errorf("命令已被策略拦截: %s", cmd)
	}
	if cmd != "" {
		r.appendCommand(cmd)
	}
	return filtered, nil
}

func isControlOnly(data []byte) bool {
	for _, b := range data {
		if b >= 32 && b != 127 {
			return false
		}
	}
	return true
}

func (r *SessionRecorder) appendCommand(cmd string) {
	r.cmdMu.Lock()
	defer r.cmdMu.Unlock()
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return
	}
	r.commands = append(r.commands, cmd)
}

func (r *SessionRecorder) Close(status string) {
	if status == "" {
		status = "closed"
	}
	st, _ := r.logFile.Stat()
	size := int64(0)
	if st != nil {
		size = st.Size()
	}
	_ = r.logFile.Close()
	now := time.Now()
	cmdJSON, _ := json.Marshal(r.commands)
	updates := map[string]interface{}{
		"end_time": &now, "status": status, "log_size": size, "commands": string(cmdJSON),
	}
	_ = r.service.db.Model(&models.BastionSession{}).Where("id = ?", r.dbID).Updates(updates).Error
	r.service.activeMu.Lock()
	delete(r.service.active, r.sessionKey)
	r.service.activeMu.Unlock()
	r.service.emitSyslog("session_end", fmt.Sprintf("user=%s asset=%s status=%s", r.username, r.assetName, status))
	r.service.onSessionClosed(r.accountID, r.assetID)
}

func (s *Service) logBlocked(sessionID uint, sessionKey, cmd, action string, userID uint, username string) {
	if userID == 0 {
		var rec models.BastionSession
		if s.db.Where("session_key = ?", sessionKey).First(&rec).Error == nil {
			userID = rec.UserID
			username = rec.Username
			sessionID = rec.ID
		}
	}
	_ = s.db.Create(&models.BastionCommandAudit{
		SessionID: sessionID, UserID: userID, Username: username,
		Command: cmd, Action: action,
	}).Error
	s.emitSyslog("command_block", fmt.Sprintf("user=%s command=%q", username, cmd))
}

func (s *Service) ListSessions(userID uint, role string, q string, limit int) ([]models.BastionSession, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	dbq := s.db.Order("start_time desc").Limit(limit)
	if role != "admin" {
		dbq = dbq.Where("user_id = ?", userID)
	}
	q = strings.TrimSpace(q)
	if q != "" {
		like := "%" + q + "%"
		dbq = dbq.Where("username LIKE ? OR host LIKE ? OR asset_name LIKE ?", like, like, like)
	}
	var list []models.BastionSession
	if err := dbq.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Service) GetSession(id uint, userID uint, role string) (*models.BastionSession, error) {
	var rec models.BastionSession
	if err := s.db.First(&rec, id).Error; err != nil {
		return nil, err
	}
	if role != "admin" && rec.UserID != userID {
		return nil, fmt.Errorf("无权查看该会话")
	}
	return &rec, nil
}

func (s *Service) ReadSessionLog(id uint, userID uint, role string) (string, error) {
	rec, err := s.GetSession(id, userID, role)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(rec.LogPath)
	if err != nil {
		return "", fmt.Errorf("日志文件不存在")
	}
	return stripANSI(string(b)), nil
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x07]*(\x07|\x1b\\)`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

func (s *Service) SessionLogPath(id uint, userID uint, role string) (string, string, error) {
	rec, err := s.GetSession(id, userID, role)
	if err != nil {
		return "", "", err
	}
	name := fmt.Sprintf("bastion-session-%d.log", rec.ID)
	return rec.LogPath, name, nil
}

func (s *Service) ListActiveSessions() []liveSession {
	s.activeMu.RLock()
	defer s.activeMu.RUnlock()
	out := make([]liveSession, 0, len(s.active))
	for _, ls := range s.active {
		out = append(out, *ls)
	}
	return out
}

func (s *Service) KillSession(sessionKey string) error {
	s.activeMu.RLock()
	ls, ok := s.active[sessionKey]
	s.activeMu.RUnlock()
	if !ok || ls.Kill == nil {
		return fmt.Errorf("会话不存在或已结束")
	}
	ls.Kill()
	now := time.Now()
	_ = s.db.Model(&models.BastionSession{}).Where("session_key = ?", sessionKey).
		Updates(map[string]interface{}{"status": "killed", "end_time": &now}).Error
	return nil
}

func (s *Service) EnrichActiveWithDB() ([]map[string]interface{}, error) {
	active := s.ListActiveSessions()
	out := make([]map[string]interface{}, 0, len(active))
	for _, ls := range active {
		var rec models.BastionSession
		s.db.Where("session_key = ?", ls.SessionKey).First(&rec)
		out = append(out, map[string]interface{}{
			"session_key": ls.SessionKey,
			"user_id":     ls.UserID,
			"username":    ls.Username,
			"asset_id":    ls.AssetID,
			"host":        ls.Host,
			"start_time":  rec.StartTime,
			"asset_name":  rec.AssetName,
			"id":          rec.ID,
		})
	}
	return out, nil
}

func (s *Service) ParseCommandsFromLog(logText string) []string {
	lines := strings.Split(logText, "\n")
	var cmds []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[stdin]") {
			cmd := strings.TrimSpace(strings.TrimPrefix(line, "[stdin]"))
			if cmd != "" {
				cmds = append(cmds, cmd)
			}
		}
	}
	return cmds
}

func (s *Service) GetSessionCommands(id uint, userID uint, role string) ([]string, error) {
	rec, err := s.GetSession(id, userID, role)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(rec.Commands) != "" {
		var cmds []string
		if json.Unmarshal([]byte(rec.Commands), &cmds) == nil && len(cmds) > 0 {
			return cmds, nil
		}
	}
	logText, err := s.ReadSessionLog(id, userID, role)
	if err != nil {
		return nil, err
	}
	return s.ParseCommandsFromLog(logText), nil
}
