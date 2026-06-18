package filemgr

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const defaultTrashRetentionDays = 30

type TrashMeta struct {
	ID           string `json:"id"`
	OriginalPath string `json:"original_path"`
	Name         string `json:"name"`
	IsDir        bool   `json:"is_dir"`
	Size         int64  `json:"size"`
	DeletedAt    int64  `json:"deleted_at"`
	DeletedBy    string `json:"deleted_by"`
	UserID       uint   `json:"user_id"`
}

type TrashItem struct {
	TrashMeta
}

func (s *Service) recycleDir() string {
	if s.dataDir == "" {
		return filepath.Join(os.TempDir(), "owpanel-recycle")
	}
	return filepath.Join(s.dataDir, "recycle")
}

func (s *Service) trashEntryDir(id string) string {
	return filepath.Join(s.recycleDir(), id)
}

func (s *Service) trashMetaPath(id string) string {
	return filepath.Join(s.trashEntryDir(id), "meta.json")
}

func (s *Service) trashItemPath(id, name string) string {
	return filepath.Join(s.trashEntryDir(id), name)
}

func (s *Service) MoveToTrash(path string, userID uint, username string) (*TrashMeta, error) {
	p, err := s.resolvePath(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	recycle := s.recycleDir()
	if err := os.MkdirAll(recycle, 0755); err != nil {
		return nil, err
	}
	cleanRecycle := filepath.Clean(recycle)
	if strings.HasPrefix(filepath.Clean(p), cleanRecycle+string(filepath.Separator)) || filepath.Clean(p) == cleanRecycle {
		return nil, fmt.Errorf("cannot delete recycle bin contents this way")
	}

	size, err := s.TreeSize(path)
	if err != nil {
		return nil, err
	}

	id := uuid.New().String()
	entryDir := s.trashEntryDir(id)
	if err := os.MkdirAll(entryDir, 0755); err != nil {
		return nil, err
	}

	dest := s.trashItemPath(id, info.Name())
	if err := os.Rename(p, dest); err != nil {
		_ = os.RemoveAll(entryDir)
		return nil, err
	}

	meta := TrashMeta{
		ID:           id,
		OriginalPath: p,
		Name:         info.Name(),
		IsDir:        info.IsDir(),
		Size:         size,
		DeletedAt:    time.Now().Unix(),
		DeletedBy:    username,
		UserID:       userID,
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		_ = os.Rename(dest, p)
		_ = os.RemoveAll(entryDir)
		return nil, err
	}
	if err := os.WriteFile(s.trashMetaPath(id), data, 0644); err != nil {
		_ = os.Rename(dest, p)
		_ = os.RemoveAll(entryDir)
		return nil, err
	}
	return &meta, nil
}

type BatchTrashResult struct {
	Moved  int      `json:"moved"`
	Failed int      `json:"failed"`
	Errors []string `json:"errors,omitempty"`
}

func (s *Service) MoveManyToTrash(paths []string, userID uint, username string) (*BatchTrashResult, error) {
	paths = filterNestedPaths(paths)
	out := &BatchTrashResult{}
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if _, err := s.MoveToTrash(p, userID, username); err != nil {
			out.Failed++
			out.Errors = append(out.Errors, fmt.Sprintf("%s: %v", p, err))
			continue
		}
		out.Moved++
	}
	return out, nil
}

func filterNestedPaths(paths []string) []string {
	seen := map[string]bool{}
	var cleaned []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		cleaned = append(cleaned, p)
	}
	sort.Slice(cleaned, func(i, j int) bool {
		return len(cleaned[i]) > len(cleaned[j])
	})
	var out []string
	for _, p := range cleaned {
		skip := false
		cleanP := filepath.Clean(strings.ReplaceAll(p, "\\", "/"))
		for _, kept := range out {
			cleanK := filepath.Clean(strings.ReplaceAll(kept, "\\", "/"))
			if cleanP == cleanK || strings.HasPrefix(cleanP, cleanK+"/") {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, p)
		}
	}
	return out
}

func (s *Service) ListTrash() ([]TrashItem, error) {
	recycle := s.recycleDir()
	entries, err := os.ReadDir(recycle)
	if err != nil {
		if os.IsNotExist(err) {
			return []TrashItem{}, nil
		}
		return nil, err
	}
	items := make([]TrashItem, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		meta, err := s.readTrashMeta(e.Name())
		if err != nil {
			continue
		}
		items = append(items, TrashItem{TrashMeta: *meta})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].DeletedAt > items[j].DeletedAt
	})
	return items, nil
}

func (s *Service) readTrashMeta(id string) (*TrashMeta, error) {
	data, err := os.ReadFile(s.trashMetaPath(id))
	if err != nil {
		return nil, err
	}
	var meta TrashMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	if meta.ID == "" {
		meta.ID = id
	}
	return &meta, nil
}

func (s *Service) RestoreTrash(id string) (string, error) {
	meta, err := s.readTrashMeta(id)
	if err != nil {
		return "", err
	}
	src := s.trashItemPath(id, meta.Name)
	if _, err := os.Stat(src); err != nil {
		return "", fmt.Errorf("trash item missing")
	}

	restorePath := meta.OriginalPath
	if _, err := os.Stat(restorePath); err == nil {
		return "", fmt.Errorf("restore target already exists: %s", restorePath)
	}
	if err := os.MkdirAll(filepath.Dir(restorePath), 0755); err != nil {
		return "", err
	}
	if err := os.Rename(src, restorePath); err != nil {
		return "", err
	}
	entryDir := s.trashEntryDir(id)
	_ = os.RemoveAll(entryDir)
	return restorePath, nil
}

func (s *Service) DeleteTrashPermanent(id string) (int64, error) {
	meta, err := s.readTrashMeta(id)
	if err != nil {
		return 0, err
	}
	entryDir := s.trashEntryDir(id)
	size := meta.Size
	if err := os.RemoveAll(entryDir); err != nil {
		return 0, err
	}
	return size, nil
}

func (s *Service) EmptyTrash() (int64, error) {
	items, err := s.ListTrash()
	if err != nil {
		return 0, err
	}
	var total int64
	for _, item := range items {
		n, err := s.DeleteTrashPermanent(item.ID)
		if err != nil {
			continue
		}
		total += n
	}
	return total, nil
}

func (s *Service) PurgeExpiredTrash(retentionDays int) (int, int64, error) {
	if retentionDays <= 0 {
		retentionDays = defaultTrashRetentionDays
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()
	items, err := s.ListTrash()
	if err != nil {
		return 0, 0, err
	}
	var purged int
	var freed int64
	for _, item := range items {
		if item.DeletedAt >= cutoff {
			continue
		}
		n, err := s.DeleteTrashPermanent(item.ID)
		if err != nil {
			continue
		}
		purged++
		freed += n
	}
	return purged, freed, nil
}

func ParseTrashRetentionDays(settings map[string]string) int {
	if settings == nil {
		return defaultTrashRetentionDays
	}
	raw := strings.TrimSpace(settings["file_trash_retention_days"])
	if raw == "" {
		return defaultTrashRetentionDays
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultTrashRetentionDays
	}
	return n
}
