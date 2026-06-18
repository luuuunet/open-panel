package bastion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/robfig/cron/v3"
	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"
)

type TemplateInput struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Language string `json:"language"`
	Content  string `json:"content"`
	Remark   string `json:"remark"`
}

type JobInput struct {
	Name       string `json:"name"`
	TemplateID uint   `json:"template_id"`
	AssetIDs   []uint `json:"asset_ids"`
	Schedule   string `json:"schedule"`
	TimeoutSec int    `json:"timeout_sec"`
	Cwd        string `json:"cwd"`
	Enabled    bool   `json:"enabled"`
}

type AdhocInput struct {
	AssetIDs   []uint `json:"asset_ids"`
	Command    string `json:"command"`
	Language   string `json:"language"`
	TimeoutSec int    `json:"timeout_sec"`
	Cwd        string `json:"cwd"`
}

var builtinTemplates = []models.OpsTemplate{
	{Name: "系统信息", Type: "command", Language: "shell", Builtin: true, Remark: "uname / free / df / uptime",
		Content: "uname -a; free -h; df -h; uptime"},
	{Name: "磁盘清理", Type: "command", Language: "shell", Builtin: true, Remark: "apt/yum 缓存清理",
		Content: "if command -v apt-get >/dev/null; then apt-get clean; elif command -v yum >/dev/null; then yum clean all; else echo 'unsupported pkg mgr'; fi"},
	{Name: "释放内存", Type: "command", Language: "shell", Builtin: true, Remark: "drop caches (需 root)",
		Content: "sync; echo 3 > /proc/sys/vm/drop_caches 2>/dev/null || echo '需要 root 权限'"},
	{Name: "Docker 状态", Type: "command", Language: "shell", Builtin: true, Remark: "容器与磁盘占用",
		Content: "docker ps -a 2>/dev/null; docker system df 2>/dev/null || echo 'docker 未安装'"},
	{Name: "Nginx 检测", Type: "command", Language: "shell", Builtin: true, Remark: "nginx -t 配置检测",
		Content: "nginx -t 2>&1 || openresty -t 2>&1 || echo 'nginx/openresty 未安装'"},
	{Name: "安全更新检查", Type: "command", Language: "shell", Builtin: true, Remark: "列出可更新包",
		Content: "if command -v apt-get >/dev/null; then apt-get update -qq && apt-get -s upgrade | grep -E '^Inst'; elif command -v yum >/dev/null; then yum check-update; else echo 'unsupported'; fi"},
	{Name: "进程 TOP10", Type: "command", Language: "shell", Builtin: true, Remark: "CPU 占用前 10",
		Content: "ps aux --sort=-%cpu 2>/dev/null | head -11 || ps -eo pid,user,pcpu,pmem,comm --sort=-pcpu | head -11"},
	{Name: "监听端口", Type: "command", Language: "shell", Builtin: true, Remark: "ss -tlnp",
		Content: "ss -tlnp 2>/dev/null || netstat -tlnp 2>/dev/null || echo 'ss/netstat 不可用'"},
	{Name: "MySQL 状态", Type: "command", Language: "mysql", Builtin: true, Remark: "SHOW STATUS 摘要",
		Content: "SHOW GLOBAL STATUS LIKE 'Threads_connected'; SHOW GLOBAL STATUS LIKE 'Uptime'; SHOW GLOBAL STATUS LIKE 'Queries';"},
	{Name: "PostgreSQL 状态", Type: "command", Language: "pgsql", Builtin: true, Remark: "pg 活动连接",
		Content: "SELECT count(*) AS connections FROM pg_stat_activity; SELECT pg_postmaster_start_time();"},
}

func (s *Service) opsRunsDir() string {
	return filepath.Join(s.dataDir, "bastion", "ops", "runs")
}

func (s *Service) initOps() {
	_ = os.MkdirAll(s.opsRunsDir(), 0755)
	s.seedBuiltinTemplates()
	s.opsCron = cron.New(cron.WithSeconds())
	s.opsCron.Start()
	s.reloadOpsSchedules()
}

func (s *Service) seedBuiltinTemplates() {
	for _, tpl := range builtinTemplates {
		var existing models.OpsTemplate
		err := s.db.Where("name = ? AND builtin = ?", tpl.Name, true).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			_ = s.db.Create(&tpl).Error
		}
	}
}

func (s *Service) reloadOpsSchedules() {
	if s.opsCron == nil {
		return
	}
	s.opsSchedMu.Lock()
	defer s.opsSchedMu.Unlock()
	for id, eid := range s.opsEntries {
		s.opsCron.Remove(eid)
		delete(s.opsEntries, id)
	}
	var jobs []models.OpsJob
	if err := s.db.Where("enabled = ? AND schedule != ''", true).Find(&jobs).Error; err != nil {
		return
	}
	for _, job := range jobs {
		s.scheduleJob(job)
	}
}

func (s *Service) scheduleJob(job models.OpsJob) {
	sched := strings.TrimSpace(job.Schedule)
	if sched == "" || !job.Enabled {
		return
	}
	jid := job.ID
	eid, err := s.opsCron.AddFunc(normalizeCron(sched), func() {
		_, _ = s.RunJob(jid, 0, "cron", "")
	})
	if err != nil {
		return
	}
	s.opsEntries[jid] = eid
}

func normalizeCron(sched string) string {
	parts := strings.Fields(strings.TrimSpace(sched))
	if len(parts) == 5 {
		return "0 " + sched
	}
	return sched
}

func parseAssetIDs(raw string) []uint {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	var ids []uint
	_ = json.Unmarshal([]byte(raw), &ids)
	return ids
}

func encodeAssetIDs(ids []uint) string {
	if len(ids) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(ids)
	return string(b)
}

func (s *Service) filterAssetIDs(userID uint, role string, ids []uint) ([]uint, error) {
	if role == "admin" {
		return ids, nil
	}
	allowed, err := s.permittedAssetIDs(userID)
	if err != nil {
		return nil, err
	}
	allowedSet := map[uint]bool{}
	for _, id := range allowed {
		allowedSet[id] = true
	}
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if allowedSet[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

func (s *Service) ListTemplates() ([]models.OpsTemplate, error) {
	var list []models.OpsTemplate
	if err := s.db.Order("builtin desc, id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Service) GetTemplate(id uint) (*models.OpsTemplate, error) {
	var tpl models.OpsTemplate
	if err := s.db.First(&tpl, id).Error; err != nil {
		return nil, err
	}
	return &tpl, nil
}

func (s *Service) CreateTemplate(in TemplateInput) (*models.OpsTemplate, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("模板名称不能为空")
	}
	tpl := models.OpsTemplate{
		Name: in.Name, Type: defaultStr(in.Type, "command"), Language: defaultStr(in.Language, "shell"),
		Content: in.Content, Remark: in.Remark, Builtin: false,
	}
	if err := s.db.Create(&tpl).Error; err != nil {
		return nil, err
	}
	return &tpl, nil
}

func (s *Service) UpdateTemplate(id uint, in TemplateInput) (*models.OpsTemplate, error) {
	var tpl models.OpsTemplate
	if err := s.db.First(&tpl, id).Error; err != nil {
		return nil, err
	}
	if tpl.Builtin {
		return nil, fmt.Errorf("内置模板不可修改")
	}
	if strings.TrimSpace(in.Name) != "" {
		tpl.Name = in.Name
	}
	if strings.TrimSpace(in.Type) != "" {
		tpl.Type = in.Type
	}
	if strings.TrimSpace(in.Language) != "" {
		tpl.Language = in.Language
	}
	tpl.Content = in.Content
	tpl.Remark = in.Remark
	if err := s.db.Save(&tpl).Error; err != nil {
		return nil, err
	}
	return &tpl, nil
}

func (s *Service) DeleteTemplate(id uint) error {
	var tpl models.OpsTemplate
	if err := s.db.First(&tpl, id).Error; err != nil {
		return err
	}
	if tpl.Builtin {
		return fmt.Errorf("内置模板不可删除")
	}
	return s.db.Delete(&models.OpsTemplate{}, id).Error
}

func defaultStr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return strings.TrimSpace(v)
}

func (s *Service) enrichJob(j *models.OpsJob) {
	if j.TemplateID > 0 {
		var tpl models.OpsTemplate
		if s.db.First(&tpl, j.TemplateID).Error == nil {
			j.TemplateName = tpl.Name
		}
	}
}

func (s *Service) ListJobs(userID uint, role string) ([]models.OpsJob, error) {
	var list []models.OpsJob
	q := s.db.Order("id desc")
	if role != "admin" && userID > 0 {
		var runJobIDs []uint
		if err := s.db.Model(&models.OpsJobRun{}).
			Where("user_id = ? AND job_id IS NOT NULL", userID).
			Distinct("job_id").
			Pluck("job_id", &runJobIDs).Error; err != nil {
			return nil, err
		}
		if len(runJobIDs) == 0 {
			return []models.OpsJob{}, nil
		}
		q = q.Where("id IN ?", runJobIDs)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		s.enrichJob(&list[i])
	}
	return list, nil
}

func (s *Service) GetJob(id uint) (*models.OpsJob, error) {
	var job models.OpsJob
	if err := s.db.First(&job, id).Error; err != nil {
		return nil, err
	}
	s.enrichJob(&job)
	return &job, nil
}

func (s *Service) CreateJob(in JobInput) (*models.OpsJob, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("作业名称不能为空")
	}
	if in.TemplateID == 0 {
		return nil, fmt.Errorf("请选择模板")
	}
	if len(in.AssetIDs) == 0 {
		return nil, fmt.Errorf("请选择至少一个资产")
	}
	timeout := in.TimeoutSec
	if timeout <= 0 {
		timeout = 30
	}
	job := models.OpsJob{
		Name: in.Name, TemplateID: in.TemplateID, AssetIDs: encodeAssetIDs(in.AssetIDs),
		Schedule: strings.TrimSpace(in.Schedule), TimeoutSec: timeout, Cwd: in.Cwd, Enabled: in.Enabled,
	}
	if err := s.db.Create(&job).Error; err != nil {
		return nil, err
	}
	s.enrichJob(&job)
	if job.Enabled && job.Schedule != "" {
		s.opsSchedMu.Lock()
		s.scheduleJob(job)
		s.opsSchedMu.Unlock()
	}
	return &job, nil
}

func (s *Service) UpdateJob(id uint, in JobInput) (*models.OpsJob, error) {
	job, err := s.GetJob(id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) != "" {
		job.Name = in.Name
	}
	if in.TemplateID > 0 {
		job.TemplateID = in.TemplateID
	}
	if len(in.AssetIDs) > 0 {
		job.AssetIDs = encodeAssetIDs(in.AssetIDs)
	}
	job.Schedule = strings.TrimSpace(in.Schedule)
	if in.TimeoutSec > 0 {
		job.TimeoutSec = in.TimeoutSec
	}
	job.Cwd = in.Cwd
	job.Enabled = in.Enabled
	if err := s.db.Save(job).Error; err != nil {
		return nil, err
	}
	s.opsSchedMu.Lock()
	if eid, ok := s.opsEntries[id]; ok {
		s.opsCron.Remove(eid)
		delete(s.opsEntries, id)
	}
	if job.Enabled && job.Schedule != "" {
		s.scheduleJob(*job)
	}
	s.opsSchedMu.Unlock()
	s.enrichJob(job)
	return job, nil
}

func (s *Service) DeleteJob(id uint) error {
	s.opsSchedMu.Lock()
	if eid, ok := s.opsEntries[id]; ok {
		s.opsCron.Remove(eid)
		delete(s.opsEntries, id)
	}
	s.opsSchedMu.Unlock()
	return s.db.Delete(&models.OpsJob{}, id).Error
}

func (s *Service) RunJob(jobID, userID uint, triggeredBy, username string) (*models.OpsJobRun, error) {
	job, err := s.GetJob(jobID)
	if err != nil {
		return nil, err
	}
	tpl, err := s.GetTemplate(job.TemplateID)
	if err != nil {
		return nil, err
	}
	assetIDs := parseAssetIDs(job.AssetIDs)
	role := "admin"
	if userID > 0 {
		var u models.User
		if s.db.First(&u, userID).Error == nil {
			role = u.Role
		}
	}
	filtered, err := s.filterAssetIDs(userID, role, assetIDs)
	if err != nil {
		return nil, err
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("没有可执行的授权资产")
	}
	jid := job.ID
	run := models.OpsJobRun{
		JobID: &jid, Status: "running", StartedAt: time.Now(),
		TriggeredBy: triggeredBy, UserID: userID, Username: username,
		Language: tpl.Language, TimeoutSec: job.TimeoutSec, AssetIDs: job.AssetIDs,
	}
	if err := s.db.Create(&run).Error; err != nil {
		return nil, err
	}
	go s.executeRun(&run, tpl.Type, tpl.Language, tpl.Content, filtered, job.TimeoutSec, job.Cwd, userID, role)
	return &run, nil
}

func (s *Service) RunAdhoc(in AdhocInput, userID uint, role, username string) (*models.OpsJobRun, error) {
	if len(in.AssetIDs) == 0 {
		return nil, fmt.Errorf("请选择至少一个资产")
	}
	cmd := strings.TrimSpace(in.Command)
	if cmd == "" {
		return nil, fmt.Errorf("命令不能为空")
	}
	if err := s.ValidateCommand(cmd); err != nil {
		return nil, err
	}
	filtered, err := s.filterAssetIDs(userID, role, in.AssetIDs)
	if err != nil {
		return nil, err
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("没有可执行的授权资产")
	}
	timeout := in.TimeoutSec
	if timeout <= 0 {
		timeout = 30
	}
	lang := defaultStr(in.Language, "shell")
	run := models.OpsJobRun{
		Status: "running", StartedAt: time.Now(), TriggeredBy: "adhoc",
		UserID: userID, Username: username, Command: cmd, Language: lang,
		TimeoutSec: timeout, AssetIDs: encodeAssetIDs(filtered),
	}
	if err := s.db.Create(&run).Error; err != nil {
		return nil, err
	}
	go s.executeRun(&run, "command", lang, cmd, filtered, timeout, in.Cwd, userID, role)
	return &run, nil
}

func (s *Service) executeRun(run *models.OpsJobRun, tplType, language, content string, assetIDs []uint, timeoutSec int, cwd string, userID uint, role string) {
	if tplType == "playbook" {
		s.executePlaybookRun(run, content, assetIDs, timeoutSec, userID, role)
		return
	}
	if err := s.ValidateCommand(content); err != nil {
		now := time.Now()
		run.Status = "failed"
		run.FinishedAt = &now
		_ = s.db.Save(run).Error
		for _, aid := range assetIDs {
			a, _ := s.GetAsset(aid)
			name := ""
			if a != nil {
				name = a.Name
			}
			_ = s.db.Create(&models.OpsJobResult{
				RunID: run.ID, AssetID: aid, AssetName: name, Status: "blocked",
				Output: err.Error(), ExitCode: -1,
			}).Error
		}
		return
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make([]models.OpsJobResult, 0, len(assetIDs))
	sem := make(chan struct{}, 10)

	for _, aid := range assetIDs {
		wg.Add(1)
		go func(assetID uint) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res := s.execOnAsset(run.ID, assetID, language, content, timeoutSec, cwd, userID, role)
			mu.Lock()
			results = append(results, res)
			mu.Unlock()
		}(aid)
	}
	wg.Wait()

	s.finishRun(run, results)
	if run.JobID != nil {
		now := time.Now()
		_ = s.db.Model(&models.OpsJob{}).Where("id = ?", *run.JobID).Updates(map[string]interface{}{
			"last_run_at": now, "last_status": run.Status,
		}).Error
	}
}

func (s *Service) finishRun(run *models.OpsJobRun, results []models.OpsJobResult) {
	ok, fail := 0, 0
	for _, r := range results {
		if r.Status == "success" {
			ok++
		} else {
			fail++
		}
	}
	status := "success"
	if fail > 0 && ok > 0 {
		status = "partial"
	} else if fail > 0 {
		status = "failed"
	}
	now := time.Now()
	run.Status = status
	run.FinishedAt = &now
	_ = s.db.Save(run).Error
}

func (s *Service) execOnAsset(runID, assetID uint, language, content string, timeoutSec int, cwd string, userID uint, role string) models.OpsJobResult {
	start := time.Now()
	a, err := s.GetAsset(assetID)
	name := ""
	if a != nil {
		name = a.Name
	}
	res := models.OpsJobResult{RunID: runID, AssetID: assetID, AssetName: name}

	host, port, user, password, privateKey, authMethod, err := s.ResolveAssetConfig(assetID, 0, userID, role)
	if err != nil {
		res.Status = "failed"
		res.Output = err.Error()
		res.ExitCode = -1
		res.DurationMs = time.Since(start).Milliseconds()
		_ = s.db.Create(&res).Error
		return res
	}

	client, err := s.dialSSH(assetID, host, port, user, password, privateKey, authMethod)
	if err != nil {
		res.Status = "failed"
		res.Output = err.Error()
		res.ExitCode = -1
		res.DurationMs = time.Since(start).Milliseconds()
		_ = s.db.Create(&res).Error
		return res
	}
	defer client.Close()

	cmdStr := wrapCommand(language, content, cwd)
	if timeoutSec <= 0 {
		timeoutSec = 30
	}

	out, exitCode, execErr := sshRunWithTimeout(client, cmdStr, time.Duration(timeoutSec)*time.Second)
	res.Output = out
	res.ExitCode = exitCode
	res.DurationMs = time.Since(start).Milliseconds()
	if execErr != nil {
		if strings.Contains(execErr.Error(), "timeout") {
			res.Status = "timeout"
		} else {
			res.Status = "failed"
		}
		if res.Output == "" {
			res.Output = execErr.Error()
		}
	} else if exitCode == 0 {
		res.Status = "success"
	} else {
		res.Status = "failed"
	}

	runDir := filepath.Join(s.opsRunsDir(), fmt.Sprintf("%d", runID))
	_ = os.MkdirAll(runDir, 0755)
	logPath := filepath.Join(runDir, fmt.Sprintf("%d.log", assetID))
	_ = os.WriteFile(logPath, []byte(res.Output), 0640)

	if len(res.Output) > 8000 {
		res.Output = res.Output[:8000] + "\n…(输出已截断，完整日志见服务器)"
	}
	_ = s.db.Create(&res).Error
	return res
}

func wrapCommand(language, content, cwd string) string {
	content = strings.TrimSpace(content)
	cd := ""
	if strings.TrimSpace(cwd) != "" {
		cd = fmt.Sprintf("cd %q && ", cwd)
	}
	switch language {
	case "python":
		return cd + fmt.Sprintf("python3 -c %q", content)
	case "mysql":
		return cd + fmt.Sprintf("mysql -e %q 2>&1", content)
	case "pgsql":
		return cd + fmt.Sprintf("psql -c %q 2>&1", content)
	default:
		return cd + content
	}
}

func (s *Service) dialSSH(assetID uint, host string, port int, user, password, privateKey, authMethod string) (*ssh.Client, error) {
	return s.dialSSHForAsset(assetID, host, port, user, password, privateKey, authMethod)
}

func sshRunWithTimeout(client *ssh.Client, cmd string, timeout time.Duration) (string, int, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", -1, err
	}
	defer session.Close()
	var buf bytes.Buffer
	session.Stdout = &buf
	session.Stderr = &buf
	done := make(chan error, 1)
	go func() { done <- session.Run(cmd) }()
	select {
	case err := <-done:
		exitCode := 0
		if err != nil {
			if ee, ok := err.(*ssh.ExitError); ok {
				exitCode = ee.ExitStatus()
			} else {
				exitCode = 1
			}
		}
		return buf.String(), exitCode, err
	case <-time.After(timeout):
		_ = session.Close()
		return buf.String(), -1, fmt.Errorf("执行超时 (%s)", timeout)
	}
}

func (s *Service) executePlaybookRun(run *models.OpsJobRun, content string, assetIDs []uint, timeoutSec int, userID uint, role string) {
	if _, err := exec.LookPath("ansible-playbook"); err != nil {
		now := time.Now()
		run.Status = "failed"
		run.FinishedAt = &now
		_ = s.db.Save(run).Error
		msg := "ansible-playbook 未安装或不在 PATH 中，无法执行 Playbook 类型作业"
		for _, aid := range assetIDs {
			a, _ := s.GetAsset(aid)
			name := ""
			if a != nil {
				name = a.Name
			}
			_ = s.db.Create(&models.OpsJobResult{
				RunID: run.ID, AssetID: aid, AssetName: name, Status: "failed", Output: msg, ExitCode: -1,
			}).Error
		}
		return
	}

	tmpDir, err := os.MkdirTemp("", "op-ops-playbook-*")
	if err != nil {
		now := time.Now()
		run.Status = "failed"
		run.FinishedAt = &now
		_ = s.db.Save(run).Error
		return
	}
	defer os.RemoveAll(tmpDir)

	pbPath := filepath.Join(tmpDir, "playbook.yml")
	invPath := filepath.Join(tmpDir, "inventory.ini")
	if err := os.WriteFile(pbPath, []byte(content), 0600); err != nil {
		return
	}

	var invLines []string
	for _, aid := range assetIDs {
		host, port, user, password, privateKey, authMethod, err := s.ResolveAssetConfig(aid, 0, userID, role)
		if err != nil {
			continue
		}
		line := fmt.Sprintf("%s ansible_host=%s ansible_port=%d ansible_user=%s", fmt.Sprintf("asset%d", aid), host, port, user)
		if authMethod == "password" && password != "" {
			line += fmt.Sprintf(" ansible_ssh_pass=%q", password)
		}
		if authMethod == "key" && privateKey != "" {
			keyPath := filepath.Join(tmpDir, fmt.Sprintf("key_%d", aid))
			_ = os.WriteFile(keyPath, []byte(privateKey), 0600)
			line += fmt.Sprintf(" ansible_ssh_private_key_file=%s", keyPath)
		}
		invLines = append(invLines, line)
	}
	_ = os.WriteFile(invPath, []byte(strings.Join(invLines, "\n")), 0600)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	if timeoutSec <= 0 {
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Minute)
	}
	defer cancel()
	cmd := exec.CommandContext(ctx, "ansible-playbook", "-i", invPath, pbPath)
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf
	err = cmd.Run()
	output := outBuf.String()
	exitCode := 0
	if err != nil {
		exitCode = 1
	}

	results := make([]models.OpsJobResult, 0, len(assetIDs))
	for _, aid := range assetIDs {
		a, _ := s.GetAsset(aid)
		name := ""
		if a != nil {
			name = a.Name
		}
		st := "success"
		if err != nil {
			st = "failed"
		}
		r := models.OpsJobResult{
			RunID: run.ID, AssetID: aid, AssetName: name, Status: st,
			Output: output, ExitCode: exitCode, DurationMs: 0,
		}
		_ = s.db.Create(&r).Error
		results = append(results, r)
	}
	s.finishRun(run, results)
	if run.JobID != nil {
		now := time.Now()
		_ = s.db.Model(&models.OpsJob{}).Where("id = ?", *run.JobID).Updates(map[string]interface{}{
			"last_run_at": now, "last_status": run.Status,
		}).Error
	}
}

func (s *Service) ListJobRuns(jobID uint, limit int) ([]models.OpsJobRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var list []models.OpsJobRun
	q := s.db.Where("job_id = ?", jobID).Order("id desc").Limit(limit)
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].JobID != nil {
			if job, err := s.GetJob(*list[i].JobID); err == nil {
				list[i].JobName = job.Name
			}
		}
	}
	return list, nil
}

func (s *Service) GetRun(runID uint) (*models.OpsJobRun, error) {
	var run models.OpsJobRun
	if err := s.db.First(&run, runID).Error; err != nil {
		return nil, err
	}
	if run.JobID != nil {
		if job, err := s.GetJob(*run.JobID); err == nil {
			run.JobName = job.Name
		}
	}
	var results []models.OpsJobResult
	if err := s.db.Where("run_id = ?", runID).Order("id asc").Find(&results).Error; err != nil {
		return nil, err
	}
	run.Results = results
	return &run, nil
}

func (s *Service) ListAdhocHistory(userID uint, role string, limit int) ([]models.OpsJobRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var list []models.OpsJobRun
	q := s.db.Where("triggered_by = ?", "adhoc")
	if role != "admin" && userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.Order("id desc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
