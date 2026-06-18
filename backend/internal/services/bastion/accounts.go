package bastion

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

const (
	accountSourceManual     = "manual"
	accountSourceDiscovered = "discovered"
	accountSourcePushed     = "pushed"
	accountStatusActive     = "active"
	discoverPasswdCmd       = "getent passwd | awk -F: '$3==0 || ($3>=1000 && $3<65534) {print $1\":\"$3\":\"$6}'"
)

type AccountInput struct {
	AssetID      uint       `json:"asset_id"`
	Username     string     `json:"username"`
	AuthMethod   string     `json:"auth_method"`
	Password     string     `json:"password"`
	KeyID        *uint      `json:"key_id"`
	IsPrivileged bool       `json:"is_privileged"`
	Source       string     `json:"source"`
	Status       string     `json:"status"`
	AutoRotate          bool       `json:"auto_rotate"`
	RotateAfterSession  bool       `json:"rotate_after_session"`
	RotateDays          int        `json:"rotate_days"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	Remark       string     `json:"remark"`
}

type PushAccountInput struct {
	CreateUser bool `json:"create_user"`
}

type RotateBatchInput struct {
	AccountIDs []uint `json:"account_ids"`
}

type VaultExport struct {
	Version   int                    `json:"version"`
	Exported  time.Time              `json:"exported"`
	Encrypted string                 `json:"encrypted"`
	Accounts  []vaultAccountSnapshot `json:"-"`
}

type vaultAccountSnapshot struct {
	AssetID      uint   `json:"asset_id"`
	Username     string `json:"username"`
	AuthMethod   string `json:"auth_method"`
	PasswordEnc  string `json:"password_enc"`
	KeyID        *uint  `json:"key_id,omitempty"`
	IsPrivileged bool   `json:"is_privileged"`
	Source       string `json:"source"`
	Status       string `json:"status"`
	AutoRotate   bool   `json:"auto_rotate"`
	RotateDays   int    `json:"rotate_days"`
	Remark       string `json:"remark"`
}

func (s *Service) initAccounts() {
	s.migrateLegacyAssetAccounts()
	go s.rotationScheduler()
}

func (s *Service) migrateLegacyAssetAccounts() {
	var assets []models.BastionAsset
	if err := s.db.Find(&assets).Error; err != nil {
		return
	}
	for _, a := range assets {
		if a.Protocol != "ssh" && a.Protocol != "" {
			continue
		}
		user := strings.TrimSpace(a.Username)
		if user == "" {
			user = "root"
		}
		var cnt int64
		s.db.Model(&models.BastionAccount{}).Where("asset_id = ? AND username = ?", a.ID, user).Count(&cnt)
		if cnt > 0 {
			continue
		}
		isPriv := user == "root" || user == "administrator"
		acc := models.BastionAccount{
			AssetID: a.ID, Username: user,
			AuthMethod: a.AuthMethod, PasswordEnc: a.PasswordEnc, KeyID: a.KeyID,
			IsPrivileged: isPriv, Source: accountSourceManual, Status: accountStatusActive,
			AutoRotate: isPriv, RotateDays: 90,
			Remark: "从资产凭据迁移",
		}
		if acc.AuthMethod == "" {
			acc.AuthMethod = "password"
		}
		_ = s.db.Create(&acc).Error
	}
}

func (s *Service) enrichAccount(a *models.BastionAccount) {
	a.HasPassword = strings.TrimSpace(a.PasswordEnc) != "" || (a.KeyID != nil && *a.KeyID > 0)
	if a.AssetID > 0 {
		if asset, err := s.GetAsset(a.AssetID); err == nil {
			a.AssetName = asset.Name
			a.AssetHost = asset.Host
		}
	}
}

func (s *Service) ListAccounts(userID uint, role string, assetID uint) ([]models.BastionAccount, error) {
	var list []models.BastionAccount
	q := s.db.Order("asset_id asc, username asc")
	if assetID > 0 {
		q = q.Where("asset_id = ?", assetID)
	}
	if role != "admin" {
		ids, err := s.permittedAssetIDs(userID)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return []models.BastionAccount{}, nil
		}
		q = q.Where("asset_id IN ?", ids)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		s.enrichAccount(&list[i])
	}
	return list, nil
}

func (s *Service) GetAccount(id uint) (*models.BastionAccount, error) {
	var a models.BastionAccount
	if err := s.db.First(&a, id).Error; err != nil {
		return nil, err
	}
	s.enrichAccount(&a)
	return &a, nil
}

func (s *Service) CreateAccount(in AccountInput) (*models.BastionAccount, error) {
	if in.AssetID == 0 || strings.TrimSpace(in.Username) == "" {
		return nil, fmt.Errorf("资产和用户名不能为空")
	}
	if _, err := s.GetAsset(in.AssetID); err != nil {
		return nil, fmt.Errorf("资产不存在")
	}
	var cnt int64
	s.db.Model(&models.BastionAccount{}).Where("asset_id = ? AND username = ?", in.AssetID, strings.TrimSpace(in.Username)).Count(&cnt)
	if cnt > 0 {
		return nil, fmt.Errorf("该资产下账号已存在")
	}
	auth := strings.TrimSpace(in.AuthMethod)
	if auth == "" {
		auth = "password"
	}
	enc, err := EncryptCredential(s.secret, in.Password)
	if err != nil {
		return nil, err
	}
	src := strings.TrimSpace(in.Source)
	if src == "" {
		src = accountSourceManual
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = accountStatusActive
	}
	rotateDays := in.RotateDays
	if rotateDays <= 0 {
		rotateDays = 90
	}
	autoRotate := in.AutoRotate
	if in.IsPrivileged && !autoRotate {
		autoRotate = true
	}
	acc := models.BastionAccount{
		AssetID: in.AssetID, Username: strings.TrimSpace(in.Username),
		AuthMethod: auth, PasswordEnc: enc, KeyID: in.KeyID,
		IsPrivileged: in.IsPrivileged, Source: src, Status: status,
		AutoRotate: autoRotate, RotateAfterSession: in.RotateAfterSession, RotateDays: rotateDays,
		ExpiresAt: in.ExpiresAt, Remark: in.Remark,
	}
	if err := s.db.Create(&acc).Error; err != nil {
		return nil, err
	}
	s.enrichAccount(&acc)
	return &acc, nil
}

func (s *Service) UpdateAccount(id uint, in AccountInput) (*models.BastionAccount, error) {
	var a models.BastionAccount
	if err := s.db.First(&a, id).Error; err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Username) != "" && in.Username != a.Username {
		var cnt int64
		s.db.Model(&models.BastionAccount{}).Where("asset_id = ? AND username = ? AND id != ?", a.AssetID, in.Username, id).Count(&cnt)
		if cnt > 0 {
			return nil, fmt.Errorf("该资产下账号已存在")
		}
		a.Username = strings.TrimSpace(in.Username)
	}
	if strings.TrimSpace(in.AuthMethod) != "" {
		a.AuthMethod = in.AuthMethod
	}
	if strings.TrimSpace(in.Password) != "" {
		enc, err := EncryptCredential(s.secret, in.Password)
		if err != nil {
			return nil, err
		}
		a.PasswordEnc = enc
	}
	if in.KeyID != nil {
		a.KeyID = in.KeyID
	}
	a.IsPrivileged = in.IsPrivileged
	if strings.TrimSpace(in.Status) != "" {
		a.Status = in.Status
	}
	a.AutoRotate = in.AutoRotate
	a.RotateAfterSession = in.RotateAfterSession
	if in.RotateDays > 0 {
		a.RotateDays = in.RotateDays
	}
	if in.ExpiresAt != nil {
		a.ExpiresAt = in.ExpiresAt
	}
	a.Remark = in.Remark
	if err := s.db.Save(&a).Error; err != nil {
		return nil, err
	}
	s.enrichAccount(&a)
	return &a, nil
}

func (s *Service) DeleteAccount(id uint) error {
	return s.db.Delete(&models.BastionAccount{}, id).Error
}

func (s *Service) ListRotationLogs(limit int) ([]models.BastionAccountRotationLog, error) {
	if limit <= 0 {
		limit = 100
	}
	var list []models.BastionAccountRotationLog
	if err := s.db.Order("rotated_at desc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Service) logRotation(accountID, assetID uint, username, status, message string) {
	_ = s.db.Create(&models.BastionAccountRotationLog{
		AccountID: accountID, AssetID: assetID, Username: username,
		Status: status, Message: message, RotatedAt: time.Now(),
	}).Error
}

func (s *Service) resolvePrivilegedSSH(assetID uint) (host string, port int, user, password, privateKey, authMethod string, err error) {
	var priv models.BastionAccount
	err = s.db.Where("asset_id = ? AND is_privileged = ? AND status = ?", assetID, true, accountStatusActive).First(&priv).Error
	if err == nil {
		a, e := s.GetAsset(assetID)
		if e != nil {
			return "", 0, "", "", "", "", e
		}
		host = a.Host
		port = a.Port
		if port <= 0 {
			port = 22
		}
		user = priv.Username
		authMethod = priv.AuthMethod
		if authMethod == "" {
			authMethod = "password"
		}
		if authMethod == "key" && priv.KeyID != nil && *priv.KeyID > 0 {
			privateKey, err = s.sshmgr.PrivateKey(*priv.KeyID)
		} else if strings.TrimSpace(priv.PasswordEnc) != "" {
			password, err = DecryptCredential(s.secret, priv.PasswordEnc)
		} else {
			err = fmt.Errorf("特权账号未配置凭据")
		}
		return host, port, user, password, privateKey, authMethod, err
	}
	if err != gorm.ErrRecordNotFound {
		return "", 0, "", "", "", "", err
	}
	return s.resolveAssetAdminCred(assetID)
}

func (s *Service) resolveAssetAdminCred(assetID uint) (host string, port int, user, password, privateKey, authMethod string, err error) {
	a, err := s.GetAsset(assetID)
	if err != nil {
		return "", 0, "", "", "", "", err
	}
	if a.Protocol != "ssh" && a.Protocol != "" {
		return "", 0, "", "", "", "", fmt.Errorf("仅支持 SSH 资产")
	}
	host = a.Host
	port = a.Port
	if port <= 0 {
		port = 22
	}
	user = a.Username
	if user == "" {
		user = "root"
	}
	authMethod = a.AuthMethod
	if authMethod == "" {
		authMethod = "password"
	}
	if authMethod == "key" && a.KeyID != nil && *a.KeyID > 0 {
		privateKey, err = s.sshmgr.PrivateKey(*a.KeyID)
	} else if strings.TrimSpace(a.PasswordEnc) != "" {
		password, err = DecryptCredential(s.secret, a.PasswordEnc)
	} else {
		err = fmt.Errorf("资产未配置管理凭据")
	}
	return
}

func generateSecurePassword(length int) (string, error) {
	if length < 16 {
		length = 16
	}
	const chars = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%^&*"
	out := make([]byte, length)
	for i := range out {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return "", err
		}
		out[i] = chars[n.Int64()]
	}
	return string(out), nil
}

func (s *Service) DiscoverAccounts(assetID uint) ([]models.BastionAccount, error) {
	a, err := s.GetAsset(assetID)
	if err != nil {
		return nil, err
	}
	if a.Protocol != "ssh" && a.Protocol != "" {
		return nil, fmt.Errorf("仅支持 SSH 资产发现")
	}
	host, port, user, password, privateKey, authMethod, err := s.resolvePrivilegedSSH(assetID)
	if err != nil {
		return nil, fmt.Errorf("无法连接管理账号: %w", err)
	}
	client, err := s.dialSSH(assetID, host, port, user, password, privateKey, authMethod)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()

	out, exitCode, execErr := sshRunWithTimeout(client, discoverPasswdCmd, 30*time.Second)
	if execErr != nil && exitCode != 0 {
		return nil, fmt.Errorf("发现命令失败: %s", out)
	}

	var discovered []models.BastionAccount
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 2 {
			continue
		}
		uname := parts[0]
		uid := parts[1]
		isPriv := uid == "0" || uname == "root"
		var existing models.BastionAccount
		err := s.db.Where("asset_id = ? AND username = ?", assetID, uname).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			acc := models.BastionAccount{
				AssetID: assetID, Username: uname,
				AuthMethod: "password", IsPrivileged: isPriv,
				Source: accountSourceDiscovered, Status: accountStatusActive,
				AutoRotate: isPriv, RotateDays: 90,
				Remark: fmt.Sprintf("自动发现 uid=%s", uid),
			}
			if err := s.db.Create(&acc).Error; err != nil {
				continue
			}
			s.enrichAccount(&acc)
			discovered = append(discovered, acc)
		} else if err == nil {
			s.enrichAccount(&existing)
			discovered = append(discovered, existing)
		}
	}
	return discovered, nil
}

func (s *Service) RotateAccount(id uint) (*models.BastionAccount, error) {
	acc, err := s.GetAccount(id)
	if err != nil {
		return nil, err
	}
	newPass, err := generateSecurePassword(18)
	if err != nil {
		return nil, err
	}
	if err := s.changeRemotePassword(acc.AssetID, acc.Username, newPass); err != nil {
		s.logRotation(acc.ID, acc.AssetID, acc.Username, "failed", err.Error())
		return nil, err
	}
	enc, err := EncryptCredential(s.secret, newPass)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	expires := now.AddDate(0, 0, acc.RotateDays)
	updates := map[string]interface{}{
		"password_enc": enc, "last_rotated_at": now, "expires_at": expires,
	}
	if err := s.db.Model(&models.BastionAccount{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	s.logRotation(acc.ID, acc.AssetID, acc.Username, "success", "密码已轮换")
	s.emitSyslog("password_rotation", fmt.Sprintf("account_id=%d user=%s status=success", acc.ID, acc.Username))
	return s.GetAccount(id)
}

func (s *Service) changeRemotePassword(assetID uint, username, newPass string) error {
	host, port, user, password, privateKey, authMethod, err := s.resolvePrivilegedSSH(assetID)
	if err != nil {
		return err
	}
	client, err := s.dialSSH(assetID, host, port, user, password, privateKey, authMethod)
	if err != nil {
		return fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()
	escapedUser := strings.ReplaceAll(username, "'", "'\\''")
	escapedPass := strings.ReplaceAll(newPass, "'", "'\\''")
	cmd := fmt.Sprintf("echo '%s:%s' | chpasswd 2>&1", escapedUser, escapedPass)
	out, exitCode, execErr := sshRunWithTimeout(client, cmd, 30*time.Second)
	if execErr != nil || exitCode != 0 {
		msg := strings.TrimSpace(out)
		if msg == "" && execErr != nil {
			msg = execErr.Error()
		}
		return fmt.Errorf("chpasswd 失败: %s", msg)
	}
	return nil
}

func (s *Service) RotateBatch(accountIDs []uint) (ok, fail int, errors []string) {
	var accounts []models.BastionAccount
	q := s.db.Where("auto_rotate = ? AND status = ?", true, accountStatusActive)
	if len(accountIDs) > 0 {
		q = q.Where("id IN ?", accountIDs)
	} else {
		q = q.Where("last_rotated_at IS NULL OR datetime(last_rotated_at, '+' || rotate_days || ' days') <= datetime('now')")
	}
	if err := q.Find(&accounts).Error; err != nil {
		return 0, 0, []string{err.Error()}
	}
	for _, acc := range accounts {
		if _, err := s.RotateAccount(acc.ID); err != nil {
			fail++
			errors = append(errors, fmt.Sprintf("%s@%d: %s", acc.Username, acc.AssetID, err.Error()))
		} else {
			ok++
		}
	}
	return ok, fail, errors
}

func (s *Service) PushAccount(id uint, in PushAccountInput) error {
	acc, err := s.GetAccount(id)
	if err != nil {
		return err
	}
	pass, err := DecryptCredential(s.secret, acc.PasswordEnc)
	if err != nil {
		return fmt.Errorf("解密凭据失败: %w", err)
	}
	if pass == "" {
		return fmt.Errorf("账号未设置密码，无法推送")
	}
	host, port, user, password, privateKey, authMethod, err := s.resolvePrivilegedSSH(acc.AssetID)
	if err != nil {
		return err
	}
	client, err := s.dialSSH(acc.AssetID, host, port, user, password, privateKey, authMethod)
	if err != nil {
		return fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()

	if in.CreateUser {
		checkCmd := fmt.Sprintf("id %q >/dev/null 2>&1 || useradd -m %q 2>&1", acc.Username, acc.Username)
		out, exitCode, _ := sshRunWithTimeout(client, checkCmd, 30*time.Second)
		if exitCode != 0 && !strings.Contains(out, "already exists") {
			return fmt.Errorf("创建用户失败: %s", strings.TrimSpace(out))
		}
	}
	if err := s.changeRemotePassword(acc.AssetID, acc.Username, pass); err != nil {
		return err
	}
	_ = s.db.Model(&models.BastionAccount{}).Where("id = ?", id).Updates(map[string]interface{}{
		"source": accountSourcePushed,
	}).Error
	return nil
}

func (s *Service) TestAccount(id uint) (bool, string, error) {
	acc, err := s.GetAccount(id)
	if err != nil {
		return false, "", err
	}
	a, err := s.GetAsset(acc.AssetID)
	if err != nil {
		return false, "", err
	}
	host := a.Host
	port := a.Port
	if port <= 0 {
		port = 22
	}
	user := acc.Username
	authMethod := acc.AuthMethod
	if authMethod == "" {
		authMethod = "password"
	}
	var password, privateKey string
	if authMethod == "key" && acc.KeyID != nil && *acc.KeyID > 0 {
		privateKey, err = s.sshmgr.PrivateKey(*acc.KeyID)
		if err != nil {
			return false, "", err
		}
	} else {
		password, err = DecryptCredential(s.secret, acc.PasswordEnc)
		if err != nil {
			return false, "", err
		}
		if password == "" {
			return false, "未配置密码或密钥", nil
		}
	}
	client, err := s.dialSSH(acc.AssetID, host, port, user, password, privateKey, authMethod)
	if err != nil {
		return false, err.Error(), nil
	}
	defer client.Close()
	out, exitCode, _ := sshRunWithTimeout(client, "echo OK", 15*time.Second)
	if exitCode == 0 && strings.Contains(out, "OK") {
		now := time.Now()
		_ = s.db.Model(&models.BastionAccount{}).Where("id = ?", id).Update("last_login_at", now).Error
		return true, "连接成功", nil
	}
	return false, strings.TrimSpace(out), nil
}

func (s *Service) ExportVault() ([]byte, error) {
	var accounts []models.BastionAccount
	if err := s.db.Find(&accounts).Error; err != nil {
		return nil, err
	}
	snaps := make([]vaultAccountSnapshot, 0, len(accounts))
	for _, a := range accounts {
		snaps = append(snaps, vaultAccountSnapshot{
			AssetID: a.AssetID, Username: a.Username, AuthMethod: a.AuthMethod,
			PasswordEnc: a.PasswordEnc, KeyID: a.KeyID, IsPrivileged: a.IsPrivileged,
			Source: a.Source, Status: a.Status, AutoRotate: a.AutoRotate,
			RotateDays: a.RotateDays, Remark: a.Remark,
		})
	}
	raw, err := json.Marshal(snaps)
	if err != nil {
		return nil, err
	}
	enc, err := EncryptCredential(s.secret, string(raw))
	if err != nil {
		return nil, err
	}
	export := VaultExport{Version: 1, Exported: time.Now(), Encrypted: enc}
	return json.Marshal(export)
}

func (s *Service) ImportVault(data []byte) (imported, skipped int, err error) {
	var export VaultExport
	if err := json.Unmarshal(data, &export); err != nil {
		return 0, 0, fmt.Errorf("无效的备份文件")
	}
	if export.Version != 1 || export.Encrypted == "" {
		return 0, 0, fmt.Errorf("不支持的备份版本")
	}
	plain, err := DecryptCredential(s.secret, export.Encrypted)
	if err != nil {
		return 0, 0, fmt.Errorf("解密备份失败: %w", err)
	}
	var snaps []vaultAccountSnapshot
	if err := json.Unmarshal([]byte(plain), &snaps); err != nil {
		return 0, 0, fmt.Errorf("解析账号数据失败")
	}
	for _, snap := range snaps {
		var cnt int64
		s.db.Model(&models.BastionAccount{}).Where("asset_id = ? AND username = ?", snap.AssetID, snap.Username).Count(&cnt)
		if cnt > 0 {
			skipped++
			continue
		}
		acc := models.BastionAccount{
			AssetID: snap.AssetID, Username: snap.Username,
			AuthMethod: snap.AuthMethod, PasswordEnc: snap.PasswordEnc, KeyID: snap.KeyID,
			IsPrivileged: snap.IsPrivileged, Source: snap.Source, Status: snap.Status,
			AutoRotate: snap.AutoRotate, RotateDays: snap.RotateDays, Remark: snap.Remark,
		}
		if acc.RotateDays <= 0 {
			acc.RotateDays = 90
		}
		if err := s.db.Create(&acc).Error; err != nil {
			skipped++
			continue
		}
		imported++
	}
	return imported, skipped, nil
}

func (s *Service) onSessionClosed(accountID, assetID uint) {
	if accountID == 0 {
		return
	}
	go func() {
		var acc models.BastionAccount
		if s.db.First(&acc, accountID).Error != nil || !acc.RotateAfterSession {
			return
		}
		if _, err := s.RotateAccount(accountID); err != nil {
			s.emitSyslog("password_rotation", fmt.Sprintf("account_id=%d user=%s status=failed err=%s", accountID, acc.Username, err.Error()))
		}
	}()
}

func (s *Service) rotationScheduler() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		s.RotateBatch(nil)
	}
}

func (s *Service) resolveAccountCred(acc *models.BastionAccount) (user, password, privateKey, authMethod string, err error) {
	user = acc.Username
	authMethod = acc.AuthMethod
	if authMethod == "" {
		authMethod = "password"
	}
	if authMethod == "key" && acc.KeyID != nil && *acc.KeyID > 0 {
		privateKey, err = s.sshmgr.PrivateKey(*acc.KeyID)
	} else if strings.TrimSpace(acc.PasswordEnc) != "" {
		password, err = DecryptCredential(s.secret, acc.PasswordEnc)
	} else {
		err = fmt.Errorf("账号未配置凭据")
	}
	return
}
