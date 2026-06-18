package bastion

import (
	"fmt"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

type AssetInput struct {
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Protocol   string `json:"protocol"`
	Username   string `json:"username"`
	AuthMethod string `json:"auth_method"`
	Password   string `json:"password"`
	KeyID      *uint  `json:"key_id"`
	GroupID    *uint  `json:"group_id"`
	Tags       string `json:"tags"`
	Remark     string `json:"remark"`
	NodeID     *uint  `json:"node_id"`
}

func (s *Service) enrichAsset(a *models.BastionAsset) {
	a.HasPassword = strings.TrimSpace(a.PasswordEnc) != ""
	if a.GroupID != nil && *a.GroupID > 0 {
		var g models.BastionAssetGroup
		if s.db.First(&g, *a.GroupID).Error == nil {
			a.GroupName = g.Name
		}
	}
}

func (s *Service) ListAssets(userID uint, role string) ([]models.BastionAsset, error) {
	var list []models.BastionAsset
	q := s.db.Order("id desc")
	if role != "admin" {
		ids, err := s.permittedAssetIDs(userID)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return []models.BastionAsset{}, nil
		}
		q = q.Where("id IN ?", ids)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		s.enrichAsset(&list[i])
	}
	return list, nil
}

func (s *Service) GetAsset(id uint) (*models.BastionAsset, error) {
	var a models.BastionAsset
	if err := s.db.First(&a, id).Error; err != nil {
		return nil, err
	}
	s.enrichAsset(&a)
	return &a, nil
}

func (s *Service) CreateAsset(in AssetInput) (*models.BastionAsset, error) {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Host) == "" {
		return nil, fmt.Errorf("名称和主机不能为空")
	}
	port := in.Port
	if port <= 0 {
		port = defaultPort(in.Protocol)
	}
	proto := strings.TrimSpace(in.Protocol)
	if proto == "" {
		proto = "ssh"
	}
	auth := strings.TrimSpace(in.AuthMethod)
	if auth == "" {
		if in.KeyID != nil && *in.KeyID > 0 {
			auth = "key"
		} else {
			auth = "password"
		}
	}
	enc, err := EncryptCredential(s.secret, in.Password)
	if err != nil {
		return nil, err
	}
	a := models.BastionAsset{
		Name: in.Name, Host: strings.TrimSpace(in.Host), Port: port,
		Protocol: proto, Username: strings.TrimSpace(in.Username),
		AuthMethod: auth, PasswordEnc: enc, KeyID: in.KeyID,
		GroupID: in.GroupID, Tags: in.Tags, Remark: in.Remark, NodeID: in.NodeID,
	}
	if err := s.db.Create(&a).Error; err != nil {
		return nil, err
	}
	s.enrichAsset(&a)
	return &a, nil
}

func (s *Service) UpdateAsset(id uint, in AssetInput) (*models.BastionAsset, error) {
	var a models.BastionAsset
	if err := s.db.First(&a, id).Error; err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) != "" {
		a.Name = in.Name
	}
	if strings.TrimSpace(in.Host) != "" {
		a.Host = strings.TrimSpace(in.Host)
	}
	if in.Port > 0 {
		a.Port = in.Port
	}
	if strings.TrimSpace(in.Protocol) != "" {
		a.Protocol = in.Protocol
	}
	if strings.TrimSpace(in.Username) != "" {
		a.Username = in.Username
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
	a.GroupID = in.GroupID
	a.Tags = in.Tags
	a.Remark = in.Remark
	if in.NodeID != nil {
		a.NodeID = in.NodeID
	}
	if err := s.db.Save(&a).Error; err != nil {
		return nil, err
	}
	s.enrichAsset(&a)
	return &a, nil
}

func (s *Service) DeleteAsset(id uint) error {
	return s.db.Delete(&models.BastionAsset{}, id).Error
}

func defaultPort(proto string) int {
	switch strings.ToLower(proto) {
	case "mysql":
		return 3306
	case "pgsql", "postgres", "postgresql":
		return 5432
	case "redis":
		return 6379
	default:
		return 22
	}
}

func (s *Service) ResolveAssetConfig(assetID, accountID, userID uint, role string) (host string, port int, user, password, privateKey, authMethod string, err error) {
	if role != "admin" {
		ok, perm, err := s.CheckPermission(userID, assetID)
		if err != nil {
			return "", 0, "", "", "", "", err
		}
		if !ok {
			return "", 0, "", "", "", "", fmt.Errorf("无权访问该资产")
		}
		if perm == "readonly" {
			// still allow connect but command filter will restrict input later
		}
	}
	a, err := s.GetAsset(assetID)
	if err != nil {
		return "", 0, "", "", "", "", fmt.Errorf("资产不存在")
	}
	if a.Protocol != "ssh" && a.Protocol != "" {
		return "", 0, "", "", "", "", fmt.Errorf("当前仅支持 SSH 协议连接")
	}
	host = a.Host
	port = a.Port
	if port <= 0 {
		port = 22
	}

	if accountID > 0 {
		var acc models.BastionAccount
		if err := s.db.First(&acc, accountID).Error; err != nil {
			return "", 0, "", "", "", "", fmt.Errorf("账号不存在")
		}
		if acc.AssetID != assetID {
			return "", 0, "", "", "", "", fmt.Errorf("账号与资产不匹配")
		}
		if acc.Status != accountStatusActive {
			return "", 0, "", "", "", "", fmt.Errorf("账号已禁用或锁定")
		}
		user, password, privateKey, authMethod, err = s.resolveAccountCred(&acc)
		if err != nil {
			return "", 0, "", "", "", "", err
		}
		return host, port, user, password, privateKey, authMethod, nil
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
		pk, err := s.sshmgr.PrivateKey(*a.KeyID)
		if err != nil {
			return "", 0, "", "", "", "", err
		}
		privateKey = pk
	} else if strings.TrimSpace(a.PasswordEnc) != "" {
		password, err = DecryptCredential(s.secret, a.PasswordEnc)
		if err != nil {
			return "", 0, "", "", "", "", err
		}
		authMethod = "password"
	}
	return host, port, user, password, privateKey, authMethod, nil
}

func (s *Service) ImportFromClusterNode(nodeID uint) (*models.BastionAsset, error) {
	var node models.ClusterNode
	if err := s.db.First(&node, nodeID).Error; err != nil {
		return nil, fmt.Errorf("节点不存在")
	}
	host := strings.TrimSpace(node.SSHHost)
	if host == "" {
		host = node.Host
	}
	port := node.SSHPort
	if port <= 0 {
		port = 22
	}
	user := strings.TrimSpace(node.SSHUser)
	if user == "" {
		user = "root"
	}
	nid := node.ID
	in := AssetInput{
		Name: node.Name, Host: host, Port: port, Protocol: "ssh",
		Username: user, AuthMethod: "password", Password: node.SSHPassword,
		NodeID: &nid, Remark: "从集群节点导入",
	}
	if strings.TrimSpace(node.SSHPassword) == "" {
		in.Password = ""
	}
	return s.CreateAsset(in)
}

func (s *Service) ListGroups() ([]models.BastionAssetGroup, error) {
	var list []models.BastionAssetGroup
	if err := s.db.Order("sort asc, id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Service) CreateGroup(name, remark string, parentID *uint) (*models.BastionAssetGroup, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("分组名称不能为空")
	}
	g := models.BastionAssetGroup{Name: name, Remark: remark, ParentID: parentID}
	if err := s.db.Create(&g).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *Service) UpdateGroup(id uint, name, remark string) error {
	updates := map[string]interface{}{}
	if strings.TrimSpace(name) != "" {
		updates["name"] = name
	}
	updates["remark"] = remark
	return s.db.Model(&models.BastionAssetGroup{}).Where("id = ?", id).Updates(updates).Error
}

func (s *Service) DeleteGroup(id uint) error {
	var cnt int64
	s.db.Model(&models.BastionAsset{}).Where("group_id = ?", id).Count(&cnt)
	if cnt > 0 {
		return fmt.Errorf("分组下仍有资产，无法删除")
	}
	return s.db.Delete(&models.BastionAssetGroup{}, id).Error
}

// ConnectTarget for terminal picker
type ConnectTarget struct {
	ID          uint   `json:"id"`
	Label       string `json:"label"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	User        string `json:"user"`
	AssetID     uint   `json:"asset_id"`
	AccountID   uint   `json:"account_id,omitempty"`
	HasPassword bool   `json:"has_password"`
	Permission  string `json:"permission"`
}

func (s *Service) ConnectTargets(userID uint, role string) ([]ConnectTarget, error) {
	assets, err := s.ListAssets(userID, role)
	if err != nil {
		return nil, err
	}
	out := make([]ConnectTarget, 0)
	for _, a := range assets {
		if a.Protocol != "ssh" && a.Protocol != "" {
			continue
		}
		perm := "connect"
		if role != "admin" {
			p, _ := s.GetUserAssetPermission(userID, a.ID)
			if p != "" {
				perm = p
			}
		}
		var accounts []models.BastionAccount
		s.db.Where("asset_id = ? AND status = ?", a.ID, accountStatusActive).Order("username asc").Find(&accounts)
		if len(accounts) == 0 {
			user := a.Username
			if user == "" {
				user = "root"
			}
			out = append(out, ConnectTarget{
				ID: a.ID, Label: a.Name + " (" + a.Host + ")",
				Host: a.Host, Port: a.Port, User: user,
				AssetID: a.ID, HasPassword: a.HasPassword || a.KeyID != nil,
				Permission: perm,
			})
			continue
		}
		for _, acc := range accounts {
			s.enrichAccount(&acc)
			label := a.Name + " / " + acc.Username + " (" + a.Host + ")"
			if acc.IsPrivileged {
				label = a.Name + " / " + acc.Username + " [priv] (" + a.Host + ")"
			}
			out = append(out, ConnectTarget{
				ID: acc.ID, Label: label,
				Host: a.Host, Port: a.Port, User: acc.Username,
				AssetID: a.ID, AccountID: acc.ID,
				HasPassword: acc.HasPassword, Permission: perm,
			})
		}
	}
	return out, nil
}

func (s *Service) GetUserAssetPermission(userID, assetID uint) (string, error) {
	var p models.BastionPermission
	err := s.db.Where("user_id = ? AND asset_id = ?", userID, assetID).First(&p).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", err
	}
	if p.ExpiresAt != nil && p.ExpiresAt.Before(time.Now()) {
		return "", nil
	}
	return p.Permission, nil
}
