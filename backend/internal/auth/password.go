package auth

import (
	"strings"
	"unicode"

	"github.com/luuuunet/owpanel/internal/models"
)

// ValidatePassword checks length and optional strong-password rules.
func ValidatePassword(plain string, requireStrong bool) error {
	if len(plain) < models.MinPasswordLength {
		return models.ErrPasswordTooShort
	}
	if !requireStrong {
		return nil
	}
	var hasUpper, hasLower, hasDigit bool
	for _, r := range plain {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit {
		return models.ErrPasswordTooWeak
	}
	lower := strings.ToLower(plain)
	for _, weak := range []string{"password", "12345678", "admin123", "qwerty12", "openpanel"} {
		if strings.Contains(lower, weak) {
			return models.ErrPasswordTooCommon
		}
	}
	return nil
}
