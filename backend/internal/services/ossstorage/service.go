package ossstorage

import (
	"fmt"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

type Service struct {
	db      *gorm.DB
	dataDir string
}

type StorageRequest struct {
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	Endpoint     string `json:"endpoint"`
	Region       string `json:"region"`
	Bucket       string `json:"bucket"`
	AccessKey    string `json:"access_key"`
	SecretKey    string `json:"secret_key"`
	LocalPath    string `json:"local_path"`
	PathPrefix   string `json:"path_prefix"`
	UsePathStyle bool   `json:"use_path_style"`
	Enabled      bool   `json:"enabled"`
	Remark       string `json:"remark"`
}

type StorageDetail struct {
	models.OSSStorage
	HasAccessKey bool `json:"has_access_key"`
	HasSecretKey bool `json:"has_secret_key"`
}

type SyncTaskRequest struct {
	Name            string `json:"name"`
	Mode            string `json:"mode"`
	SourceStorageID *uint  `json:"source_storage_id"`
	TargetStorageID *uint  `json:"target_storage_id"`
	ExtraTargetIDs  []uint `json:"extra_target_ids"`
	SourcePath      string `json:"source_path"`
	TargetPath      string `json:"target_path"`
	LocalPath       string `json:"local_path"`
	DeleteExtra     bool   `json:"delete_extra"`
	Schedule        string `json:"schedule"`
	Enabled         bool   `json:"enabled"`
}

func NewService(db *gorm.DB, dataDir string) *Service {
	s := &Service{db: db, dataDir: dataDir}
	go s.scheduleLoop()
	return s
}

func (s *Service) ListProviders() []ProviderPreset {
	return ProviderPresets
}

func (s *Service) ListStorages() ([]StorageDetail, error) {
	var list []models.OSSStorage
	if err := s.db.Order("id desc").Find(&list).Error; err != nil {
		return nil, err
	}
	out := make([]StorageDetail, 0, len(list))
	for i := range list {
		out = append(out, StorageDetail{
			OSSStorage:   list[i],
			HasAccessKey: list[i].AccessKey != "",
			HasSecretKey: list[i].SecretKey != "",
		})
	}
	return out, nil
}

func (s *Service) GetStorage(id uint) (*StorageDetail, error) {
	var st models.OSSStorage
	if err := s.db.First(&st, id).Error; err != nil {
		return nil, err
	}
	return &StorageDetail{
		OSSStorage:   st,
		HasAccessKey: st.AccessKey != "",
		HasSecretKey: st.SecretKey != "",
	}, nil
}

func (s *Service) CreateStorage(req *StorageRequest) (*models.OSSStorage, error) {
	st := s.reqToStorage(req, &models.OSSStorage{})
	if err := s.db.Create(st).Error; err != nil {
		return nil, err
	}
	if st.Provider == "local" && st.LocalPath == "" {
		st.LocalPath = fmt.Sprintf("oss-local/storage-%d", st.ID)
		s.db.Save(st)
	}
	return st, nil
}

func (s *Service) UpdateStorage(id uint, req *StorageRequest) (*models.OSSStorage, error) {
	var st models.OSSStorage
	if err := s.db.First(&st, id).Error; err != nil {
		return nil, err
	}
	updated := s.reqToStorage(req, &st)
	if req.AccessKey == "" {
		updated.AccessKey = st.AccessKey
	}
	if req.SecretKey == "" {
		updated.SecretKey = st.SecretKey
	}
	if err := s.db.Save(updated).Error; err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *Service) reqToStorage(req *StorageRequest, st *models.OSSStorage) *models.OSSStorage {
	st.Name = strings.TrimSpace(req.Name)
	st.Provider = strings.ToLower(strings.TrimSpace(req.Provider))
	st.Endpoint = strings.TrimSpace(req.Endpoint)
	st.Region = strings.TrimSpace(req.Region)
	st.Bucket = strings.TrimSpace(req.Bucket)
	if req.AccessKey != "" {
		st.AccessKey = req.AccessKey
	}
	if req.SecretKey != "" {
		st.SecretKey = req.SecretKey
	}
	st.LocalPath = strings.TrimSpace(req.LocalPath)
	st.PathPrefix = strings.Trim(strings.TrimSpace(req.PathPrefix), "/")
	st.UsePathStyle = req.UsePathStyle
	if !req.UsePathStyle {
		st.UsePathStyle = DefaultPathStyle(st.Provider)
	}
	st.Enabled = req.Enabled
	st.Remark = req.Remark
	if st.Endpoint == "" && st.Provider != "local" {
		st.Endpoint = ResolveEndpoint(st.Provider, st.Region, "")
	}
	return st
}

func (s *Service) DeleteStorage(id uint) error {
	return s.db.Delete(&models.OSSStorage{}, id).Error
}

func (s *Service) TestStorage(id uint) error {
	st, err := s.GetStorage(id)
	if err != nil {
		return err
	}
	store, err := NewStore(&st.OSSStorage, s.dataDir)
	if err != nil {
		return err
	}
	return store.Test()
}

func (s *Service) BrowseStorage(id uint, prefix string, limit int) ([]ObjectInfo, error) {
	st, err := s.GetStorage(id)
	if err != nil {
		return nil, err
	}
	store, err := NewStore(&st.OSSStorage, s.dataDir)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 200
	}
	return store.List(prefix, limit)
}

func (s *Service) ListSyncTasks() ([]models.OSSSyncTask, error) {
	var list []models.OSSSyncTask
	return list, s.db.Order("id desc").Find(&list).Error
}

func (s *Service) CreateSyncTask(req *SyncTaskRequest) (*models.OSSSyncTask, error) {
	task := &models.OSSSyncTask{
		Name:            strings.TrimSpace(req.Name),
		Mode:            strings.ToLower(strings.TrimSpace(req.Mode)),
		SourceStorageID: req.SourceStorageID,
		TargetStorageID: req.TargetStorageID,
		ExtraTargetIDs:  formatTargetIDs(req.ExtraTargetIDs),
		SourcePath:      strings.Trim(req.SourcePath, "/"),
		TargetPath:      strings.Trim(req.TargetPath, "/"),
		LocalPath:       strings.TrimSpace(req.LocalPath),
		DeleteExtra:     req.DeleteExtra,
		Schedule:        strings.TrimSpace(req.Schedule),
		Enabled:         req.Enabled,
		LastStatus:      "idle",
	}
	if err := s.db.Create(task).Error; err != nil {
		return nil, err
	}
	return task, nil
}

func (s *Service) UpdateSyncTask(id uint, req *SyncTaskRequest) (*models.OSSSyncTask, error) {
	var task models.OSSSyncTask
	if err := s.db.First(&task, id).Error; err != nil {
		return nil, err
	}
	task.Name = strings.TrimSpace(req.Name)
	task.Mode = strings.ToLower(strings.TrimSpace(req.Mode))
	task.SourceStorageID = req.SourceStorageID
	task.TargetStorageID = req.TargetStorageID
	task.ExtraTargetIDs = formatTargetIDs(req.ExtraTargetIDs)
	task.SourcePath = strings.Trim(req.SourcePath, "/")
	task.TargetPath = strings.Trim(req.TargetPath, "/")
	task.LocalPath = strings.TrimSpace(req.LocalPath)
	task.DeleteExtra = req.DeleteExtra
	task.Schedule = strings.TrimSpace(req.Schedule)
	task.Enabled = req.Enabled
	if err := s.db.Save(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *Service) DeleteSyncTask(id uint) error {
	return s.db.Delete(&models.OSSSyncTask{}, id).Error
}

func (s *Service) RunSyncTask(id uint) error {
	go func() {
		_ = s.runSyncTask(id)
	}()
	return nil
}

func (s *Service) GetSyncTaskLogs(id uint) (*models.OSSSyncTask, error) {
	var task models.OSSSyncTask
	if err := s.db.First(&task, id).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *Service) scheduleLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.runDueTasks()
	}
}

func (s *Service) runDueTasks() {
	var tasks []models.OSSSyncTask
	if err := s.db.Where("enabled = ? AND schedule != '' AND running = ?", true, false).Find(&tasks).Error; err != nil {
		return
	}
	for _, task := range tasks {
		if s.shouldRunNow(task) {
			go func(id uint) { _ = s.runSyncTask(id) }(task.ID)
		}
	}
}

func (s *Service) shouldRunNow(task models.OSSSyncTask) bool {
	// Simple hourly/daily cron subset: "0 * * * *" hourly at :00, "0 2 * * *" daily 2am
	parts := strings.Fields(task.Schedule)
	if len(parts) != 5 {
		return false
	}
	now := time.Now()
	min, hour := now.Minute(), now.Hour()
	wantMin, wantHour := parseCronField(parts[0]), parseCronField(parts[1])
	if wantMin >= 0 && wantMin != min {
		return false
	}
	if wantHour >= 0 && wantHour != hour {
		return false
	}
	if task.LastRunAt != nil && time.Since(*task.LastRunAt) < 50*time.Minute {
		return false
	}
	return true
}

func parseCronField(s string) int {
	if s == "*" {
		return -1
	}
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

// UploadFile uploads a local file to an OSS storage endpoint (used by backup hooks).
func (s *Service) UploadFile(storageID uint, localFile, key string) error {
	store, err := s.openStore(&storageID)
	if err != nil {
		return err
	}
	return store.UploadFile(localFile, strings.TrimLeft(key, "/"))
}

// DownloadFile downloads an object from OSS to a local path.
func (s *Service) DownloadFile(storageID uint, key, localPath string) error {
	store, err := s.openStore(&storageID)
	if err != nil {
		return err
	}
	return store.DownloadFile(strings.TrimLeft(key, "/"), localPath)
}

// DeleteObject removes an object from OSS storage.
func (s *Service) DeleteObject(storageID uint, key string) error {
	store, err := s.openStore(&storageID)
	if err != nil {
		return err
	}
	return store.Delete(strings.TrimLeft(key, "/"))
}
