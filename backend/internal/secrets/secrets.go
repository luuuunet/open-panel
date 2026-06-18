package secrets

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
)

const (
	jwtSecretFile = ".jwt_secret"
	edgeWorkerSecretFile = ".edge_worker_secret"
	credFileName  = "INITIAL_CREDENTIALS.txt"
)

// LoadOrCreateJWTSecret returns env override, persisted secret, or a newly generated one.
func LoadOrCreateJWTSecret(dataDir string) string {
	if v := strings.TrimSpace(os.Getenv("OWPANEL_JWT_SECRET")); v != "" {
		return v
	}
	path := filepath.Join(dataDir, jwtSecretFile)
	if b, err := os.ReadFile(path); err == nil {
		if s := strings.TrimSpace(string(b)); len(s) >= 32 {
			return s
		}
	}
	secret := randomHex(32)
	_ = os.WriteFile(path, []byte(secret+"\n"), 0600)
	return secret
}

// LoadOrCreateEdgeWorkerSecret returns persisted secret for edge worker internal API.
func LoadOrCreateEdgeWorkerSecret(dataDir string) string {
	path := filepath.Join(dataDir, edgeWorkerSecretFile)
	if b, err := os.ReadFile(path); err == nil {
		if s := strings.TrimSpace(string(b)); len(s) >= 16 {
			return s
		}
	}
	secret := randomHex(24)
	_ = os.WriteFile(path, []byte(secret+"\n"), 0600)
	return secret
}

// GeneratePassword returns a random alphanumeric password.
func GeneratePassword(length int) (string, error) {
	const chars = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	if length < 8 {
		length = 12
	}
	out := make([]byte, length)
	max := big.NewInt(int64(len(chars)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = chars[n.Int64()]
	}
	return string(out), nil
}

// WriteInitialAdminCredentials saves first-login credentials for the operator.
func WriteInitialAdminCredentials(dataDir, username, password string) (string, error) {
	path := filepath.Join(dataDir, credFileName)
	body := fmt.Sprintf(`OWPanel — initial admin credentials
Username: %s
Password: %s

Change this password immediately after first login.
This file is stored only on the server at: %s
`, username, password, path)
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
