package ossstorage

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

const exportVersion = 1

type StorageExport struct {
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	Endpoint     string `json:"endpoint"`
	Region       string `json:"region"`
	Bucket       string `json:"bucket"`
	AccessKey    string `json:"access_key,omitempty"`
	SecretKey    string `json:"secret_key,omitempty"`
	LocalPath    string `json:"local_path"`
	PathPrefix   string `json:"path_prefix"`
	UsePathStyle bool   `json:"use_path_style"`
	Enabled      bool   `json:"enabled"`
	Remark       string `json:"remark"`
}

type SyncTaskExport struct {
	Name              string `json:"name"`
	Mode              string `json:"mode"`
	SourceStorageName string `json:"source_storage_name,omitempty"`
	TargetStorageName string `json:"target_storage_name,omitempty"`
	ExtraTargetNames  []string `json:"extra_target_names,omitempty"`
	SourcePath        string `json:"source_path"`
	TargetPath        string `json:"target_path"`
	LocalPath         string `json:"local_path"`
	DeleteExtra       bool   `json:"delete_extra"`
	Schedule          string `json:"schedule"`
	Enabled           bool   `json:"enabled"`
}

type ConfigExport struct {
	Version    int              `json:"version"`
	ExportedAt string           `json:"exported_at"`
	Storages   []StorageExport  `json:"storages"`
	SyncTasks  []SyncTaskExport `json:"sync_tasks"`
}

type ImportRequest struct {
	Version   int              `json:"version"`
	Storages  []StorageExport  `json:"storages"`
	SyncTasks []SyncTaskExport `json:"sync_tasks"`
	Mode      string           `json:"mode"` // merge (default) or replace
}

type ImportResult struct {
	StoragesCreated  int      `json:"storages_created"`
	StoragesUpdated  int      `json:"storages_updated"`
	StoragesSkipped  int      `json:"storages_skipped"`
	TasksCreated     int      `json:"tasks_created"`
	TasksSkipped     int      `json:"tasks_skipped"`
	Warnings         []string `json:"warnings,omitempty"`
}

func (s *Service) ExportConfig(includeSecrets bool) (*ConfigExport, error) {
	var storages []models.OSSStorage
	if err := s.db.Order("id asc").Find(&storages).Error; err != nil {
		return nil, err
	}
	var tasks []models.OSSSyncTask
	if err := s.db.Order("id asc").Find(&tasks).Error; err != nil {
		return nil, err
	}
	idToName := map[uint]string{}
	out := &ConfigExport{
		Version:    exportVersion,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Storages:   make([]StorageExport, 0, len(storages)),
		SyncTasks:  make([]SyncTaskExport, 0, len(tasks)),
	}
	for _, st := range storages {
		idToName[st.ID] = st.Name
		exp := StorageExport{
			Name:         st.Name,
			Provider:     st.Provider,
			Endpoint:     st.Endpoint,
			Region:       st.Region,
			Bucket:       st.Bucket,
			LocalPath:    st.LocalPath,
			PathPrefix:   st.PathPrefix,
			UsePathStyle: st.UsePathStyle,
			Enabled:      st.Enabled,
			Remark:       st.Remark,
		}
		if includeSecrets {
			exp.AccessKey = st.AccessKey
			exp.SecretKey = st.SecretKey
		}
		out.Storages = append(out.Storages, exp)
	}
	for _, task := range tasks {
		exp := SyncTaskExport{
			Name:        task.Name,
			Mode:        task.Mode,
			SourcePath:  task.SourcePath,
			TargetPath:  task.TargetPath,
			LocalPath:   task.LocalPath,
			DeleteExtra: task.DeleteExtra,
			Schedule:    task.Schedule,
			Enabled:     task.Enabled,
		}
		if task.SourceStorageID != nil {
			exp.SourceStorageName = idToName[*task.SourceStorageID]
		}
		if task.TargetStorageID != nil {
			exp.TargetStorageName = idToName[*task.TargetStorageID]
		}
		for _, id := range parseTargetIDs(task.ExtraTargetIDs) {
			if name := idToName[id]; name != "" {
				exp.ExtraTargetNames = append(exp.ExtraTargetNames, name)
			}
		}
		out.SyncTasks = append(out.SyncTasks, exp)
	}
	return out, nil
}

func (s *Service) ImportConfig(req *ImportRequest) (*ImportResult, error) {
	if req == nil {
		return nil, fmt.Errorf("empty import payload")
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "merge"
	}
	res := &ImportResult{}
	if mode == "replace" {
		s.db.Where("1=1").Delete(&models.OSSSyncTask{})
		s.db.Where("1=1").Delete(&models.OSSStorage{})
	}
	nameToID := map[string]uint{}
	for _, exp := range req.Storages {
		if strings.TrimSpace(exp.Name) == "" {
			res.Warnings = append(res.Warnings, "skipped storage with empty name")
			res.StoragesSkipped++
			continue
		}
		var existing models.OSSStorage
		err := s.db.Where("name = ?", exp.Name).First(&existing).Error
		if err == nil && mode == "merge" {
			_, err = s.UpdateStorage(existing.ID, &StorageRequest{
				Name: exp.Name, Provider: exp.Provider, Endpoint: exp.Endpoint,
				Region: exp.Region, Bucket: exp.Bucket, AccessKey: exp.AccessKey,
				SecretKey: exp.SecretKey, LocalPath: exp.LocalPath, PathPrefix: exp.PathPrefix,
				UsePathStyle: exp.UsePathStyle, Enabled: exp.Enabled, Remark: exp.Remark,
			})
			if err != nil {
				res.Warnings = append(res.Warnings, fmt.Sprintf("update %s: %v", exp.Name, err))
				res.StoragesSkipped++
				continue
			}
			nameToID[exp.Name] = existing.ID
			res.StoragesUpdated++
			continue
		}
		if err == nil && mode != "replace" {
			nameToID[exp.Name] = existing.ID
			res.StoragesSkipped++
			continue
		}
		st, err := s.CreateStorage(&StorageRequest{
			Name: exp.Name, Provider: exp.Provider, Endpoint: exp.Endpoint,
			Region: exp.Region, Bucket: exp.Bucket, AccessKey: exp.AccessKey,
			SecretKey: exp.SecretKey, LocalPath: exp.LocalPath, PathPrefix: exp.PathPrefix,
			UsePathStyle: exp.UsePathStyle, Enabled: exp.Enabled, Remark: exp.Remark,
		})
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("create %s: %v", exp.Name, err))
			res.StoragesSkipped++
			continue
		}
		nameToID[exp.Name] = st.ID
		res.StoragesCreated++
	}
	// refresh name map with all storages
	var all []models.OSSStorage
	_ = s.db.Find(&all).Error
	for _, st := range all {
		nameToID[st.Name] = st.ID
	}
	for _, exp := range req.SyncTasks {
		if strings.TrimSpace(exp.Name) == "" {
			res.TasksSkipped++
			continue
		}
		var existing models.OSSSyncTask
		if s.db.Where("name = ?", exp.Name).First(&existing).Error == nil && mode == "merge" {
			res.TasksSkipped++
			continue
		}
		reqTask := &SyncTaskRequest{
			Name: exp.Name, Mode: exp.Mode, SourcePath: exp.SourcePath,
			TargetPath: exp.TargetPath, LocalPath: exp.LocalPath,
			DeleteExtra: exp.DeleteExtra, Schedule: exp.Schedule, Enabled: exp.Enabled,
		}
		if exp.SourceStorageName != "" {
			if id, ok := nameToID[exp.SourceStorageName]; ok {
				reqTask.SourceStorageID = &id
			} else {
				res.Warnings = append(res.Warnings, fmt.Sprintf("task %s: unknown source %s", exp.Name, exp.SourceStorageName))
			}
		}
		if exp.TargetStorageName != "" {
			if id, ok := nameToID[exp.TargetStorageName]; ok {
				reqTask.TargetStorageID = &id
			} else {
				res.Warnings = append(res.Warnings, fmt.Sprintf("task %s: unknown target %s", exp.Name, exp.TargetStorageName))
			}
		}
		for _, name := range exp.ExtraTargetNames {
			if id, ok := nameToID[name]; ok {
				reqTask.ExtraTargetIDs = append(reqTask.ExtraTargetIDs, id)
			}
		}
		if _, err := s.CreateSyncTask(reqTask); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("task %s: %v", exp.Name, err))
			res.TasksSkipped++
			continue
		}
		res.TasksCreated++
	}
	return res, nil
}

func parseTargetIDs(raw string) []uint {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var ids []uint
	if strings.HasPrefix(raw, "[") {
		_ = json.Unmarshal([]byte(raw), &ids)
		return ids
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var id uint
		if _, err := fmt.Sscanf(part, "%d", &id); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func formatTargetIDs(ids []uint) string {
	if len(ids) == 0 {
		return ""
	}
	b, _ := json.Marshal(ids)
	return string(b)
}

func allTargetIDs(task *models.OSSSyncTask) []uint {
	seen := map[uint]bool{}
	var out []uint
	add := func(id *uint) {
		if id == nil || *id == 0 || seen[*id] {
			return
		}
		seen[*id] = true
		out = append(out, *id)
	}
	add(task.TargetStorageID)
	for _, id := range parseTargetIDs(task.ExtraTargetIDs) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
