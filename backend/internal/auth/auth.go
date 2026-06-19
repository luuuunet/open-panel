package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

var ErrInvalidCredentials = errors.New("invalid username or password")

type Claims struct {
	UserID      uint   `json:"user_id"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	Permissions string `json:"permissions,omitempty"`
	DiskQuotaMB int64  `json:"disk_quota_mb,omitempty"`
	TotpPending bool   `json:"totp_pending,omitempty"`
	jwt.RegisteredClaims
}

type Service struct {
	db        *gorm.DB
	jwtSecret []byte
	tokenTTL  func() time.Duration
}

func NewService(db *gorm.DB, jwtSecret string) *Service {
	return &Service{db: db, jwtSecret: []byte(jwtSecret)}
}

func (s *Service) SetTokenTTL(fn func() time.Duration) {
	s.tokenTTL = fn
}

func (s *Service) tokenDuration() time.Duration {
	if s.tokenTTL != nil {
		if d := s.tokenTTL(); d > 0 {
			return d
		}
	}
	return 24 * time.Hour
}

func (s *Service) Login(username, password string) (string, *models.User, error) {
	var user models.User
	if err := s.db.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil, ErrInvalidCredentials
		}
		return "", nil, err
	}

	if !user.CheckPassword(password) {
		return "", nil, ErrInvalidCredentials
	}

	token, err := s.issueToken(&user)
	if err != nil {
		return "", nil, err
	}
	return token, &user, nil
}

func (s *Service) ParseToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func (s *Service) issueToken(user *models.User) (string, error) {
	claims := Claims{
		UserID:      user.ID,
		Username:    user.Username,
		Role:        user.Role,
		Permissions: user.Permissions,
		DiskQuotaMB: user.DiskQuotaMB,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.tokenDuration())),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *Service) ListUsers() ([]models.User, error) {
	var users []models.User
	err := s.db.Find(&users).Error
	return users, err
}

func (s *Service) CreateUser(username, password, role string) (*models.User, error) {
	return s.CreateUserFull(username, password, role, nil, "", 0)
}

type CreateUserRequest struct {
	Username    string
	Password    string
	Role        string
	ParentID    *uint
	Permissions string
	DiskQuotaMB int64
	Remark      string
}

func (s *Service) CreateUserFull(username, password, role string, parentID *uint, permissions string, diskQuotaMB int64) (*models.User, error) {
	return s.CreateUserExtended(CreateUserRequest{
		Username: username, Password: password, Role: role,
		ParentID: parentID, Permissions: permissions, DiskQuotaMB: diskQuotaMB,
	})
}

func (s *Service) CreateUserExtended(req CreateUserRequest) (*models.User, error) {
	if req.Role == "" {
		req.Role = "user"
	}
	if req.Role == "subuser" && req.Permissions == "" {
		req.Permissions = DefaultPermissions().JSON()
	}
	user := &models.User{
		Username: req.Username, Role: req.Role, ParentID: req.ParentID,
		Permissions: req.Permissions, DiskQuotaMB: req.DiskQuotaMB, Remark: req.Remark,
	}
	if err := user.SetPassword(req.Password); err != nil {
		return nil, err
	}
	if err := s.db.Create(user).Error; err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Service) UpdateUser(id uint, role, permissions, remark string, diskQuotaMB int64) error {
	updates := map[string]interface{}{}
	if role != "" {
		updates["role"] = role
	}
	if permissions != "" {
		updates["permissions"] = permissions
	}
	if remark != "" {
		updates["remark"] = remark
	}
	if diskQuotaMB >= 0 {
		updates["disk_quota_mb"] = diskQuotaMB
	}
	return s.db.Model(&models.User{}).Where("id = ?", id).Updates(updates).Error
}

func (s *Service) DeleteUser(id uint) error {
	var user models.User
	if err := s.db.First(&user, id).Error; err != nil {
		return err
	}
	if user.Role == "admin" {
		var count int64
		s.db.Model(&models.User{}).Where("role = ?", "admin").Count(&count)
		if count <= 1 {
			return errors.New("不能删除最后一个管理员")
		}
	}
	return s.db.Delete(&models.User{}, id).Error
}

func (s *Service) AddDiskUsage(userID uint, mb int64) error {
	if userID == 0 || mb <= 0 {
		return nil
	}
	return s.db.Model(&models.User{}).Where("id = ?", userID).
		UpdateColumn("disk_used_mb", gorm.Expr("disk_used_mb + ?", mb)).Error
}

func (s *Service) SubDiskUsage(userID uint, mb int64) error {
	if userID == 0 || mb <= 0 {
		return nil
	}
	return s.db.Model(&models.User{}).Where("id = ?", userID).
		UpdateColumn("disk_used_mb", gorm.Expr("CASE WHEN disk_used_mb > ? THEN disk_used_mb - ? ELSE 0 END", mb, mb)).Error
}

func (s *Service) GetUser(id uint) (*models.User, error) {
	var user models.User
	if err := s.db.First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Service) CheckDiskQuota(userID uint, addBytes int64) error {
	if userID == 0 || addBytes <= 0 {
		return nil
	}
	user, err := s.GetUser(userID)
	if err != nil || user.DiskQuotaMB <= 0 {
		return nil
	}
	addMB := QuotaMBFromBytes(addBytes)
	if user.DiskUsedMB+addMB > user.DiskQuotaMB {
		return fmt.Errorf("磁盘配额不足（已用 %d MB / 限额 %d MB）", user.DiskUsedMB, user.DiskQuotaMB)
	}
	return nil
}

func (s *Service) ChangePassword(userID uint, newPassword string) error {
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return err
	}
	if err := user.SetPassword(newPassword); err != nil {
		return err
	}
	user.MustChangePassword = false
	return s.db.Save(&user).Error
}

func (s *Service) ChangePasswordByUsername(username, newPassword string) error {
	var user models.User
	if err := s.db.Where("username = ?", username).First(&user).Error; err != nil {
		return err
	}
	if err := user.SetPassword(newPassword); err != nil {
		return err
	}
	return s.db.Save(&user).Error
}

func (s *Service) ChangeUsername(oldName, newName string) error {
	var user models.User
	if err := s.db.Where("username = ?", oldName).First(&user).Error; err != nil {
		return err
	}
	user.Username = newName
	return s.db.Save(&user).Error
}

func (s *Service) FirstAdmin() (*models.User, error) {
	var user models.User
	err := s.db.Where("role = ?", "admin").Order("id asc").First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}
