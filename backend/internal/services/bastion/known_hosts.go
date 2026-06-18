package bastion

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"
)

func fingerprintKey(key ssh.PublicKey) string {
	hash := sha256.Sum256(key.Marshal())
	return "SHA256:" + base64.StdEncoding.EncodeToString(hash[:])
}

func (s *Service) ListKnownHosts() ([]models.BastionKnownHost, error) {
	var list []models.BastionKnownHost
	if err := s.db.Order("asset_id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		if a, err := s.GetAsset(list[i].AssetID); err == nil {
			list[i].AssetName = a.Name
		}
	}
	return list, nil
}

func (s *Service) GetKnownHost(assetID uint) (*models.BastionKnownHost, error) {
	var kh models.BastionKnownHost
	err := s.db.Where("asset_id = ?", assetID).First(&kh).Error
	if err != nil {
		return nil, err
	}
	return &kh, nil
}

func (s *Service) CaptureHostKey(assetID uint) (*models.BastionKnownHost, error) {
	a, err := s.GetAsset(assetID)
	if err != nil {
		return nil, err
	}
	port := a.Port
	if port <= 0 {
		port = 22
	}
	addr := fmt.Sprintf("%s:%d", a.Host, port)
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("无法连接主机: %w", err)
	}
	defer conn.Close()

	var capturedKey ssh.PublicKey
	_, _, _, keyErr := ssh.NewClientConn(conn, addr, &ssh.ClientConfig{
		User: "probe",
		Auth: []ssh.AuthMethod{},
		HostKeyCallback: func(hostname string, remote net.Addr, k ssh.PublicKey) error {
			capturedKey = k
			return fmt.Errorf("probe")
		},
		Timeout: 10 * time.Second,
	})
	if capturedKey == nil {
		if keyErr != nil && !strings.Contains(keyErr.Error(), "probe") {
			return nil, fmt.Errorf("捕获密钥失败: %w", keyErr)
		}
		return nil, fmt.Errorf("未能捕获主机密钥")
	}

	fp := fingerprintKey(capturedKey)
	kh := models.BastionKnownHost{
		AssetID: assetID, Host: a.Host, Port: port,
		KeyType: capturedKey.Type(), PublicKey: string(ssh.MarshalAuthorizedKey(capturedKey)),
		Fingerprint: fp, Status: "pending",
	}
	var existing models.BastionKnownHost
	err = s.db.Where("asset_id = ?", assetID).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		if err := s.db.Create(&kh).Error; err != nil {
			return nil, err
		}
		if a2, e := s.GetAsset(assetID); e == nil {
			kh.AssetName = a2.Name
		}
		return &kh, nil
	}
	if err != nil {
		return nil, err
	}
	existing.Host = kh.Host
	existing.Port = kh.Port
	existing.KeyType = kh.KeyType
	existing.PublicKey = kh.PublicKey
	existing.Fingerprint = kh.Fingerprint
	existing.Status = "pending"
	if err := s.db.Save(&existing).Error; err != nil {
		return nil, err
	}
	if a2, e := s.GetAsset(assetID); e == nil {
		existing.AssetName = a2.Name
	}
	return &existing, nil
}

func (s *Service) AcceptKnownHost(assetID uint) (*models.BastionKnownHost, error) {
	var kh models.BastionKnownHost
	if err := s.db.Where("asset_id = ?", assetID).First(&kh).Error; err != nil {
		return nil, err
	}
	kh.Status = "accepted"
	if err := s.db.Save(&kh).Error; err != nil {
		return nil, err
	}
	return &kh, nil
}

func (s *Service) HostKeyCallback(assetID uint) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		if assetID == 0 {
			return ssh.InsecureIgnoreHostKey()(hostname, remote, key)
		}
		kh, err := s.GetKnownHost(assetID)
		if err != nil || kh.Status != "accepted" {
			return ssh.InsecureIgnoreHostKey()(hostname, remote, key)
		}
		pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(kh.PublicKey))
		if err != nil {
			return ssh.InsecureIgnoreHostKey()(hostname, remote, key)
		}
		if ssh.FingerprintSHA256(pub) != ssh.FingerprintSHA256(key) {
			return fmt.Errorf("主机密钥不匹配 (expected %s)", kh.Fingerprint)
		}
		return nil
	}
}

func (s *Service) dialSSHForAsset(assetID uint, host string, port int, user, password, privateKey, authMethod string) (*ssh.Client, error) {
	if port <= 0 {
		port = 22
	}
	if user == "" {
		user = "root"
	}
	var authMethods []ssh.AuthMethod
	if authMethod == "key" && strings.TrimSpace(privateKey) != "" {
		signer, err := ssh.ParsePrivateKey([]byte(privateKey))
		if err != nil {
			return nil, fmt.Errorf("私钥解析失败: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	} else if strings.TrimSpace(password) != "" {
		authMethods = append(authMethods, ssh.Password(password))
	} else {
		return nil, fmt.Errorf("未配置 SSH 凭据")
	}
	cfg := &ssh.ClientConfig{
		User: user, Auth: authMethods,
		HostKeyCallback: s.HostKeyCallback(assetID),
		Timeout:         15 * time.Second,
	}
	return ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), cfg)
}
