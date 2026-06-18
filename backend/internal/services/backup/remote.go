package backup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type RemoteRequest struct {
	Name           string `json:"name"`
	Provider       string `json:"provider"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	RemotePath     string `json:"remote_path"`
	AccessToken    string `json:"access_token"`
	ExtraConfig    string `json:"extra_config"`
	Enabled        bool   `json:"enabled"`
	OSSStorageID   *uint  `json:"oss_storage_id"`
}

type RemoteDetail struct {
	models.BackupRemote
	HasPassword    bool `json:"has_password"`
	HasAccessToken bool `json:"has_access_token"`
}

func (s *Service) ListRemotes() ([]RemoteDetail, error) {
	var list []models.BackupRemote
	if err := s.db.Order("id desc").Find(&list).Error; err != nil {
		return nil, err
	}
	out := make([]RemoteDetail, 0, len(list))
	for i := range list {
		out = append(out, RemoteDetail{
			BackupRemote:   list[i],
			HasPassword:    list[i].Password != "",
			HasAccessToken: list[i].AccessToken != "",
		})
	}
	return out, nil
}

func (s *Service) GetRemote(id uint) (*RemoteDetail, error) {
	var r models.BackupRemote
	if err := s.db.First(&r, id).Error; err != nil {
		return nil, err
	}
	return &RemoteDetail{
		BackupRemote:   r,
		HasPassword:    r.Password != "",
		HasAccessToken: r.AccessToken != "",
	}, nil
}

func (s *Service) CreateRemote(req *RemoteRequest) (*models.BackupRemote, error) {
	r := &models.BackupRemote{
		Name:           strings.TrimSpace(req.Name),
		Provider:       strings.ToLower(strings.TrimSpace(req.Provider)),
		Host:           strings.TrimSpace(req.Host),
		Port:           req.Port,
		Username:       strings.TrimSpace(req.Username),
		Password:       req.Password,
		RemotePath:     strings.TrimSpace(req.RemotePath),
		AccessToken:    strings.TrimSpace(req.AccessToken),
		ExtraConfig:    req.ExtraConfig,
		Enabled:        req.Enabled,
		OSSStorageID:   req.OSSStorageID,
	}
	s.applyRemoteDefaults(r)
	if err := s.db.Create(r).Error; err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Service) UpdateRemote(id uint, req *RemoteRequest) (*models.BackupRemote, error) {
	var r models.BackupRemote
	if err := s.db.First(&r, id).Error; err != nil {
		return nil, err
	}
	r.Name = strings.TrimSpace(req.Name)
	r.Provider = strings.ToLower(strings.TrimSpace(req.Provider))
	r.Host = strings.TrimSpace(req.Host)
	r.Port = req.Port
	r.Username = strings.TrimSpace(req.Username)
	if req.Password != "" {
		r.Password = req.Password
	}
	r.RemotePath = strings.TrimSpace(req.RemotePath)
	if req.AccessToken != "" {
		r.AccessToken = req.AccessToken
	}
	r.ExtraConfig = req.ExtraConfig
	r.Enabled = req.Enabled
	r.OSSStorageID = req.OSSStorageID
	s.applyRemoteDefaults(&r)
	if err := s.db.Save(&r).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Service) DeleteRemote(id uint) error {
	return s.db.Delete(&models.BackupRemote{}, id).Error
}

func (s *Service) TestRemote(id uint) error {
	if _, err := s.GetRemote(id); err != nil {
		return err
	}
	tmp := filepath.Join(os.TempDir(), "owpanel-backup-test.txt")
	if err := os.WriteFile(tmp, []byte("owpanel backup test"), 0644); err != nil {
		return err
	}
	defer os.Remove(tmp)
	return s.uploadToRemote(id, tmp, "owpanel-backup-test.txt")
}

func (s *Service) applyRemoteDefaults(r *models.BackupRemote) {
	switch r.Provider {
	case "sftp":
		if r.Port == 0 {
			r.Port = 22
		}
	case "ftp":
		if r.Port == 0 {
			r.Port = 21
		}
	case "webdav", "custom":
		if r.Port == 0 {
			r.Port = 443
		}
	}
	if r.RemotePath == "" {
		r.RemotePath = "/"
	}
}

func (s *Service) uploadToRemote(remoteID uint, localFile, remoteName string) error {
	var r models.BackupRemote
	if err := s.db.First(&r, remoteID).Error; err != nil {
		return err
	}
	if !r.Enabled {
		return fmt.Errorf("远程目标已禁用")
	}
	switch r.Provider {
	case "ftp":
		return uploadFTP(&r, localFile, remoteName)
	case "sftp":
		return uploadSFTP(&r, localFile, remoteName)
	case "webdav", "custom":
		return uploadWebDAV(&r, localFile, remoteName)
	case "google_drive", "gdrive":
		return uploadGoogleDrive(&r, localFile, remoteName)
	case "onedrive", "microsoft":
		return uploadOneDrive(&r, localFile, remoteName)
	case "oss", "s3", "object_storage":
		if r.OSSStorageID == nil || *r.OSSStorageID == 0 {
			return fmt.Errorf("OSS 存储未配置")
		}
		return s.uploadToOSS(*r.OSSStorageID, localFile, path.Join(strings.Trim(r.RemotePath, "/"), remoteName))
	default:
		return fmt.Errorf("不支持的远程类型: %s", r.Provider)
	}
}

func uploadFTP(r *models.BackupRemote, localFile, remoteName string) error {
	addr := fmt.Sprintf("%s:%d", r.Host, r.Port)
	conn, err := ftp.Dial(addr, ftp.DialWithTimeout(30*time.Second))
	if err != nil {
		return err
	}
	defer conn.Quit()
	if err := conn.Login(r.Username, r.Password); err != nil {
		return err
	}
	dir := strings.Trim(r.RemotePath, "/")
	if dir != "" {
		for _, part := range strings.Split(dir, "/") {
			_ = conn.MakeDir(part)
			_ = conn.ChangeDir(part)
		}
	}
	f, err := os.Open(localFile)
	if err != nil {
		return err
	}
	defer f.Close()
	return conn.Stor(remoteName, f)
}

func uploadSFTP(r *models.BackupRemote, localFile, remoteName string) error {
	addr := fmt.Sprintf("%s:%d", r.Host, r.Port)
	cfg := &ssh.ClientConfig{
		User:            r.Username,
		Auth:            []ssh.AuthMethod{ssh.Password(r.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return err
	}
	defer client.Close()
	sc, err := sftp.NewClient(client)
	if err != nil {
		return err
	}
	defer sc.Close()
	remotePath := path.Join(r.RemotePath, remoteName)
	_ = sc.MkdirAll(path.Dir(remotePath))
	dst, err := sc.Create(remotePath)
	if err != nil {
		return err
	}
	defer dst.Close()
	src, err := os.Open(localFile)
	if err != nil {
		return err
	}
	defer src.Close()
	_, err = io.Copy(dst, src)
	return err
}

func uploadWebDAV(r *models.BackupRemote, localFile, remoteName string) error {
	base := strings.TrimRight(r.Host, "/")
	if !strings.HasPrefix(base, "http") {
		base = "https://" + base
	}
	remoteURL := strings.TrimRight(base, "/") + "/" + strings.Trim(r.RemotePath, "/") + "/" + remoteName
	req, err := http.NewRequest(http.MethodPut, remoteURL, nil)
	if err != nil {
		return err
	}
	f, err := os.Open(localFile)
	if err != nil {
		return err
	}
	defer f.Close()
	req.Body = io.NopCloser(f)
	req.ContentLength = fileSize(localFile)
	if r.Username != "" {
		req.SetBasicAuth(r.Username, r.Password)
	}
	if r.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.AccessToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("WebDAV 上传失败 (%d): %s", resp.StatusCode, string(b))
	}
	return nil
}

func uploadGoogleDrive(r *models.BackupRemote, localFile, remoteName string) error {
	token := r.AccessToken
	folderID := r.RemotePath
	if token == "" {
		return fmt.Errorf("请填写 Google Drive Access Token")
	}
	meta := map[string]interface{}{"name": remoteName}
	if folderID != "" && folderID != "/" {
		meta["parents"] = []string{folderID}
	}
	metaBytes, _ := json.Marshal(meta)
	f, err := os.Open(localFile)
	if err != nil {
		return err
	}
	defer f.Close()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("metadata", string(metaBytes))
	part, _ := w.CreateFormFile("file", remoteName)
	_, _ = io.Copy(part, f)
	_ = w.Close()

	u := "https://www.googleapis.com/upload/drive/v3/files?uploadType=multipart"
	req, err := http.NewRequest(http.MethodPost, u, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Google Drive 上传失败 (%d): %s", resp.StatusCode, string(b))
	}
	return nil
}

func uploadOneDrive(r *models.BackupRemote, localFile, remoteName string) error {
	token := r.AccessToken
	if token == "" {
		return fmt.Errorf("请填写 OneDrive Access Token")
	}
	remoteDir := strings.Trim(r.RemotePath, "/")
	if remoteDir == "" {
		remoteDir = "OWPanelBackups"
	}
	u := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/drive/root:/%s/%s:/content", url.PathEscape(remoteDir), url.PathEscape(remoteName))
	f, err := os.Open(localFile)
	if err != nil {
		return err
	}
	defer f.Close()
	req, err := http.NewRequest(http.MethodPut, u, f)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/zip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OneDrive 上传失败 (%d): %s", resp.StatusCode, string(b))
	}
	return nil
}

func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}
