package auth

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image/png"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/pquerna/otp/totp"
)

var ErrInvalidTotpCode = errors.New("invalid TOTP code")

type TotpSetupResult struct {
	Secret string `json:"secret"`
	QRData string `json:"qr_data"` // base64 PNG
	URL    string `json:"url"`
}

func (s *Service) IssueTotpPendingToken(user *models.User) (string, error) {
	claims := Claims{
		UserID:      user.ID,
		Username:    user.Username,
		Role:        user.Role,
		Permissions: user.Permissions,
		DiskQuotaMB: user.DiskQuotaMB,
		TotpPending: true,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *Service) CompleteTotpLogin(tempToken, code string, decryptFn func(string) (string, error)) (string, *models.User, error) {
	claims, err := s.ParseToken(tempToken)
	if err != nil {
		return "", nil, err
	}
	if !claims.TotpPending {
		return "", nil, errors.New("invalid temp token")
	}
	var user models.User
	if err := s.db.First(&user, claims.UserID).Error; err != nil {
		return "", nil, err
	}
	if !user.TotpEnabled {
		return "", nil, errors.New("2FA not enabled")
	}
	secret, err := decryptFn(user.TotpSecret)
	if err != nil || !totp.Validate(code, secret) {
		return "", nil, ErrInvalidTotpCode
	}
	token, err := s.issueToken(&user)
	if err != nil {
		return "", nil, err
	}
	return token, &user, nil
}

func (s *Service) SetupTotp(userID uint, encryptFn func(string) (string, error)) (*TotpSetupResult, error) {
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, err
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "OWPanel",
		AccountName: user.Username,
		SecretSize:  20,
	})
	if err != nil {
		return nil, err
	}
	enc, err := encryptFn(key.Secret())
	if err != nil {
		return nil, err
	}
	if err := s.db.Model(&user).Updates(map[string]interface{}{
		"totp_secret": enc, "totp_enabled": false,
	}).Error; err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	img, err := key.Image(200, 200)
	if err != nil {
		return nil, err
	}
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return &TotpSetupResult{
		Secret: key.Secret(),
		QRData: base64.StdEncoding.EncodeToString(buf.Bytes()),
		URL:    key.URL(),
	}, nil
}

func (s *Service) VerifyAndEnableTotp(userID uint, code string, decryptFn func(string) (string, error)) error {
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return err
	}
	if strings.TrimSpace(user.TotpSecret) == "" {
		return errors.New("请先发起 2FA 设置")
	}
	secret, err := decryptFn(user.TotpSecret)
	if err != nil {
		return err
	}
	if !totp.Validate(code, secret) {
		return ErrInvalidTotpCode
	}
	return s.db.Model(&user).Update("totp_enabled", true).Error
}

func (s *Service) DisableTotp(userID uint, password string, isAdmin bool) error {
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return err
	}
	if !isAdmin && !user.CheckPassword(password) {
		return ErrInvalidCredentials
	}
	return s.db.Model(&user).Updates(map[string]interface{}{
		"totp_enabled": false, "totp_secret": "",
	}).Error
}
