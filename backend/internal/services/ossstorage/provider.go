package ossstorage

import (
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

type ObjectInfo struct {
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	IsDir        bool   `json:"is_dir"`
	LastModified string `json:"last_modified,omitempty"`
}

type ObjectStore interface {
	Test() error
	List(prefix string, limit int) ([]ObjectInfo, error)
	UploadFile(localPath, key string) error
	DownloadFile(key, localPath string) error
	Delete(key string) error
	Walk(prefix string, fn func(ObjectInfo) error) error
	DisplayName() string
}

func NewStore(st *models.OSSStorage, dataDir string) (ObjectStore, error) {
	provider := strings.ToLower(strings.TrimSpace(st.Provider))
	switch provider {
	case "local":
		return newLocalStore(st, dataDir)
	case "minio", "aliyun", "tencent", "aws", "google", "ibm", "custom":
		return newS3Store(st)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", st.Provider)
	}
}

func joinKey(prefix, key string) string {
	prefix = strings.Trim(prefix, "/")
	key = strings.TrimLeft(key, "/")
	if prefix == "" {
		return key
	}
	if key == "" {
		return prefix
	}
	return prefix + "/" + key
}
