package ossstorage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

type syncLogger struct {
	mu    sync.Mutex
	lines []string
}

func newSyncLogger() *syncLogger {
	return &syncLogger{lines: []string{}}
}

func (l *syncLogger) log(format string, args ...interface{}) {
	line := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
	l.mu.Lock()
	l.lines = append(l.lines, line)
	if len(l.lines) > 3000 {
		l.lines = l.lines[len(l.lines)-3000:]
	}
	l.mu.Unlock()
}

func (l *syncLogger) text() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n")
}

func (s *Service) runSyncTask(taskID uint) error {
	var task models.OSSSyncTask
	if err := s.db.First(&task, taskID).Error; err != nil {
		return err
	}
	if task.Running {
		return fmt.Errorf("task already running")
	}
	s.db.Model(&task).Updates(map[string]interface{}{
		"running": true, "last_status": "running", "last_error": "", "last_log": "",
	})
	logger := newSyncLogger()
	var runErr error
	defer func() {
		now := time.Now()
		status := "success"
		errMsg := ""
		if runErr != nil {
			status = "failed"
			errMsg = runErr.Error()
		}
		s.db.Model(&models.OSSSyncTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
			"running":     false,
			"last_status": status,
			"last_error":  errMsg,
			"last_log":    logger.text(),
			"last_run_at": now,
		})
	}()

	mode := strings.ToLower(task.Mode)
	logger.log("开始任务: %s (%s)", task.Name, mode)
	switch mode {
	case "upload":
		runErr = s.syncLocalToRemote(&task, logger)
	case "download":
		runErr = s.syncRemoteToLocal(&task, logger)
	case "sync":
		runErr = s.syncBidirectional(&task, logger)
	case "migrate":
		runErr = s.syncRemoteToRemote(&task, logger)
	default:
		runErr = fmt.Errorf("unknown mode: %s", mode)
	}
	if runErr != nil {
		logger.log("失败: %v", runErr)
	} else {
		logger.log("完成")
	}
	return runErr
}

func (s *Service) openStore(id *uint) (ObjectStore, error) {
	if id == nil {
		return nil, fmt.Errorf("storage not configured")
	}
	var st models.OSSStorage
	if err := s.db.First(&st, *id).Error; err != nil {
		return nil, err
	}
	if !st.Enabled {
		return nil, fmt.Errorf("storage disabled")
	}
	return NewStore(&st, s.dataDir)
}

func (s *Service) syncLocalToRemote(task *models.OSSSyncTask, log *syncLogger) error {
	targets := allTargetIDs(task)
	if len(targets) == 0 {
		return fmt.Errorf("target storage required")
	}
	localRoot := task.LocalPath
	if localRoot == "" {
		localRoot = task.SourcePath
	}
	if localRoot == "" {
		return fmt.Errorf("local path required")
	}
	if !filepath.IsAbs(localRoot) {
		localRoot = filepath.Join(s.dataDir, localRoot)
	}
	prefix := strings.Trim(task.TargetPath, "/")
	for _, targetID := range targets {
		id := targetID
		t := *task
		t.TargetStorageID = &id
		log.log("目标存储 #%d", id)
		if err := s.syncLocalToOneRemote(&t, localRoot, prefix, log); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) syncLocalToOneRemote(task *models.OSSSyncTask, localRoot, prefix string, log *syncLogger) error {
	store, err := s.openStore(task.TargetStorageID)
	if err != nil {
		return err
	}
	uploaded := map[string]bool{}
	err = filepath.Walk(localRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(localRoot, path)
		key := filepath.ToSlash(rel)
		if prefix != "" {
			key = joinKey(prefix, key)
		}
		log.log("上传 %s -> %s", path, key)
		if err := store.UploadFile(path, key); err != nil {
			return err
		}
		uploaded[key] = true
		return nil
	})
	if err != nil {
		return err
	}
	if task.DeleteExtra && prefix != "" {
		_ = store.Walk(prefix, func(obj ObjectInfo) error {
			if obj.IsDir {
				return nil
			}
			if !uploaded[obj.Key] {
				log.log("删除多余远程对象 %s", obj.Key)
				return store.Delete(obj.Key)
			}
			return nil
		})
	}
	return nil
}

func (s *Service) syncRemoteToLocal(task *models.OSSSyncTask, log *syncLogger) error {
	store, err := s.openStore(task.SourceStorageID)
	if err != nil {
		return err
	}
	localRoot := task.LocalPath
	if localRoot == "" {
		localRoot = task.TargetPath
	}
	if localRoot == "" {
		return fmt.Errorf("local path required")
	}
	if !filepath.IsAbs(localRoot) {
		localRoot = filepath.Join(s.dataDir, localRoot)
	}
	prefix := strings.Trim(task.SourcePath, "/")
	return store.Walk(prefix, func(obj ObjectInfo) error {
		dst := filepath.Join(localRoot, filepath.FromSlash(obj.Key))
		log.log("下载 %s -> %s", obj.Key, dst)
		return store.DownloadFile(obj.Key, dst)
	})
}

func (s *Service) syncBidirectional(task *models.OSSSyncTask, log *syncLogger) error {
	if err := s.syncLocalToRemote(task, log); err != nil {
		return err
	}
	t2 := *task
	t2.SourceStorageID, t2.TargetStorageID = task.TargetStorageID, task.SourceStorageID
	t2.SourcePath, t2.TargetPath = task.TargetPath, task.SourcePath
	return s.syncRemoteToLocal(&t2, log)
}

func (s *Service) syncRemoteToRemote(task *models.OSSSyncTask, log *syncLogger) error {
	targets := allTargetIDs(task)
	if len(targets) == 0 {
		return fmt.Errorf("target storage required")
	}
	for _, targetID := range targets {
		id := targetID
		t := *task
		t.TargetStorageID = &id
		log.log("迁移至目标存储 #%d", id)
		if err := s.syncRemoteToOneRemote(&t, log); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) syncRemoteToOneRemote(task *models.OSSSyncTask, log *syncLogger) error {
	src, err := s.openStore(task.SourceStorageID)
	if err != nil {
		return err
	}
	dst, err := s.openStore(task.TargetStorageID)
	if err != nil {
		return err
	}
	srcPrefix := strings.Trim(task.SourcePath, "/")
	dstPrefix := strings.Trim(task.TargetPath, "/")
	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("owpanel-oss-migrate-%d", task.ID))
	defer os.RemoveAll(tmpDir)
	return src.Walk(srcPrefix, func(obj ObjectInfo) error {
		srcKey := obj.Key
		if srcPrefix != "" && !strings.HasPrefix(srcKey, srcPrefix) {
			srcKey = joinKey(srcPrefix, obj.Key)
		}
		dstKey := joinKey(dstPrefix, obj.Key)
		tmp := filepath.Join(tmpDir, filepath.FromSlash(obj.Key))
		log.log("迁移 %s -> %s", srcKey, dstKey)
		if err := os.MkdirAll(filepath.Dir(tmp), 0755); err != nil {
			return err
		}
		if err := src.DownloadFile(srcKey, tmp); err != nil {
			return err
		}
		if err := dst.UploadFile(tmp, dstKey); err != nil {
			return err
		}
		return os.Remove(tmp)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
