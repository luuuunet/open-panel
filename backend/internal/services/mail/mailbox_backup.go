package mail

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

type backupManifest struct {
	Version    int                  `json:"version"`
	CreatedAt  string               `json:"created_at"`
	Domain     string               `json:"domain,omitempty"`
	Mailboxes  []MailboxExportRow   `json:"mailboxes"`
}

type BackupRequest struct {
	Domain         string `json:"domain"`
	IncludeMaildir bool   `json:"include_maildir"`
}

func (s *Service) backupDir() string {
	return filepath.Join(s.mailRoot(), "backups")
}

func (s *Service) ListMailBackups() ([]models.MailBackup, error) {
	var list []models.MailBackup
	return list, s.db.Order("id desc").Find(&list).Error
}

func (s *Service) RunMailBackup(req *BackupRequest) (*models.MailBackup, error) {
	domain := strings.TrimSpace(strings.ToLower(req.Domain))
	boxes, err := s.ListMailboxes(domain)
	if err != nil {
		return nil, err
	}
	if len(boxes) == 0 {
		return nil, fmt.Errorf("没有可备份的邮箱")
	}
	dir := s.backupDir()
	_ = os.MkdirAll(dir, 0755)
	ts := time.Now().Format("20060102-150405")
	name := "mail-backup-" + ts + ".tar.gz"
	if domain != "" {
		name = "mail-" + sanitizeName(domain) + "-" + ts + ".tar.gz"
	}
	dest := filepath.Join(dir, name)

	rec := &models.MailBackup{
		Domain:         domain,
		FilePath:       dest,
		MailboxCount:   len(boxes),
		IncludeMaildir: req.IncludeMaildir,
		Status:         "running",
	}
	if err := s.db.Create(rec).Error; err != nil {
		return nil, err
	}

	manifest := backupManifest{
		Version:   1,
		CreatedAt: time.Now().Format(time.RFC3339),
		Domain:    domain,
	}
	for _, m := range boxes {
		manifest.Mailboxes = append(manifest.Mailboxes, MailboxExportRow{
			Address:  m.Address,
			Domain:   m.Domain,
			Password: s.readPassSecret(m.Address),
			Quota:    m.Quota,
			Maildir:  m.Maildir,
		})
	}

	if err := s.writeBackupArchive(dest, &manifest, domain, req.IncludeMaildir); err != nil {
		rec.Status = "failed"
		rec.ErrorMsg = err.Error()
		_ = os.Remove(dest)
		_ = s.db.Save(rec).Error
		return rec, err
	}
	st, err := os.Stat(dest)
	if err != nil {
		rec.Status = "failed"
		rec.ErrorMsg = err.Error()
		_ = s.db.Save(rec).Error
		return rec, err
	}
	rec.Size = st.Size()
	rec.Status = "done"
	_ = s.db.Save(rec).Error
	s.pruneMailBackups(10)
	return rec, nil
}

func (s *Service) writeBackupArchive(dest string, manifest *backupManifest, domain string, includeMaildir bool) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := writeTarBytes(tw, "manifest.json", manifestData, 0644); err != nil {
		return err
	}

	if !includeMaildir {
		return nil
	}
	walkBase := s.vmailBase()
	if domain != "" {
		walkBase = filepath.Join(walkBase, domain)
	}
	if _, err := os.Stat(walkBase); os.IsNotExist(err) {
		return nil
	}
	return filepath.Walk(walkBase, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		rel, err := filepath.Rel(s.vmailBase(), path)
		if err != nil {
			return nil
		}
		name := filepath.ToSlash(filepath.Join("vmail", rel))
		if info.IsDir() {
			hdr := &tar.Header{Name: name + "/", Mode: 0755, Typeflag: tar.TypeDir}
			return tw.WriteHeader(hdr)
		}
		hdr := &tar.Header{Name: name, Mode: int64(info.Mode()) & 0777, Size: info.Size(), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return nil
		}
		_, copyErr := io.Copy(tw, in)
		in.Close()
		return copyErr
	})
}

func writeTarBytes(tw *tar.Writer, name string, data []byte, mode int64) error {
	hdr := &tar.Header{Name: name, Mode: mode, Size: int64(len(data)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func (s *Service) GetMailBackupFile(id uint) (string, error) {
	var rec models.MailBackup
	if err := s.db.First(&rec, id).Error; err != nil {
		return "", err
	}
	if rec.FilePath == "" {
		return "", fmt.Errorf("备份文件不存在")
	}
	if _, err := os.Stat(rec.FilePath); err != nil {
		return "", fmt.Errorf("备份文件已丢失")
	}
	return rec.FilePath, nil
}

func (s *Service) DeleteMailBackup(id uint) error {
	var rec models.MailBackup
	if err := s.db.First(&rec, id).Error; err != nil {
		return err
	}
	if rec.FilePath != "" {
		_ = os.Remove(rec.FilePath)
	}
	return s.db.Delete(&rec).Error
}

func (s *Service) RestoreMailBackup(id uint) error {
	path, err := s.GetMailBackupFile(id)
	if err != nil {
		return err
	}
	return s.restoreFromArchive(path, true)
}

func (s *Service) ImportMailBackupFile(srcPath string, restoreMaildir bool) error {
	return s.restoreFromArchive(srcPath, restoreMaildir)
}

func (s *Service) restoreFromArchive(srcPath string, restoreMaildir bool) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)

	tmpDir, err := os.MkdirTemp("", "mail-restore-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	var manifest backupManifest
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(tmpDir, filepath.FromSlash(hdr.Name))
		switch hdr.Typeflag {
		case tar.TypeDir:
			_ = os.MkdirAll(target, 0755)
		case tar.TypeReg:
			_ = os.MkdirAll(filepath.Dir(target), 0755)
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
			if hdr.Name == "manifest.json" {
				data, _ := os.ReadFile(target)
				_ = json.Unmarshal(data, &manifest)
			}
		}
	}
	if len(manifest.Mailboxes) == 0 {
		return fmt.Errorf("备份中无邮箱账号数据")
	}
	_, err = s.ImportMailboxes(&ImportMailboxRequest{
		Accounts:       manifest.Mailboxes,
		SkipExisting:     false,
		UpdatePassword: true,
	})
	if err != nil {
		return err
	}
	if restoreMaildir {
		vmailSrc := filepath.Join(tmpDir, "vmail")
		if _, err := os.Stat(vmailSrc); err == nil {
			destBase := s.vmailBase()
			_ = os.MkdirAll(destBase, 0750)
			if out, err := exec.Command("cp", "-a", vmailSrc+"/.", destBase).CombinedOutput(); err != nil {
				return fmt.Errorf("恢复邮件目录失败: %s", strings.TrimSpace(string(out)))
			}
			_ = exec.Command("chown", "-R", "vmail:vmail", destBase).Run()
		}
	}
	return s.syncMaps()
}

func (s *Service) pruneMailBackups(keep int) {
	if keep <= 0 {
		keep = 10
	}
	var list []models.MailBackup
	s.db.Where("status = ?", "done").Order("id desc").Find(&list)
	if len(list) <= keep {
		return
	}
	for _, old := range list[keep:] {
		_ = s.DeleteMailBackup(old.ID)
	}
}

func sanitizeName(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, "..", "")
	s = strings.ReplaceAll(s, "/", "_")
	return s
}
