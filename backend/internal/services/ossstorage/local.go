package ossstorage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

type localStore struct {
	root   string
	prefix string
	name   string
}

func newLocalStore(st *models.OSSStorage, dataDir string) (*localStore, error) {
	root := strings.TrimSpace(st.LocalPath)
	if root == "" {
		root = filepath.Join(dataDir, "oss-local", fmt.Sprintf("storage-%d", st.ID))
	}
	if !filepath.IsAbs(root) {
		root = filepath.Join(dataDir, root)
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, err
	}
	return &localStore{root: root, prefix: strings.Trim(st.PathPrefix, "/"), name: st.Name}, nil
}

func (l *localStore) DisplayName() string { return l.name }

func (l *localStore) absKey(key string) string {
	key = joinKey(l.prefix, key)
	return filepath.Join(l.root, filepath.FromSlash(key))
}

func (l *localStore) Test() error {
	f := filepath.Join(l.root, ".owpanel-test")
	if err := os.WriteFile(f, []byte("ok"), 0644); err != nil {
		return err
	}
	return os.Remove(f)
}

func (l *localStore) List(prefix string, limit int) ([]ObjectInfo, error) {
	base := l.absKey(prefix)
	var out []ObjectInfo
	_ = filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if path == base {
			return nil
		}
		rel, _ := filepath.Rel(l.root, path)
		key := filepath.ToSlash(rel)
		if l.prefix != "" && !strings.HasPrefix(key, l.prefix) {
			return nil
		}
		if l.prefix != "" {
			key = strings.TrimPrefix(key, l.prefix+"/")
		}
		out = append(out, ObjectInfo{
			Key:          key,
			Size:         info.Size(),
			IsDir:        info.IsDir(),
			LastModified: info.ModTime().Format(time.RFC3339),
		})
		if limit > 0 && len(out) >= limit {
			return io.EOF
		}
		return nil
	})
	return out, nil
}

func (l *localStore) UploadFile(localPath, key string) error {
	dst := l.absKey(key)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	src, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer src.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, src)
	return err
}

func (l *localStore) DownloadFile(key, localPath string) error {
	srcPath := l.absKey(key)
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	out, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, src)
	return err
}

func (l *localStore) Delete(key string) error {
	return os.RemoveAll(l.absKey(key))
}

func (l *localStore) Walk(prefix string, fn func(ObjectInfo) error) error {
	base := l.absKey(prefix)
	return filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(l.root, path)
		key := filepath.ToSlash(rel)
		if l.prefix != "" {
			key = strings.TrimPrefix(key, l.prefix+"/")
		}
		return fn(ObjectInfo{Key: key, Size: info.Size()})
	})
}
