package database

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

const postgresSuperPasswordKey = "postgresql_superuser_password"

func (s *Service) pgSuperExec(args ...string) ([]byte, error) {
	bin, err := findBinary("psql")
	if err != nil {
		return nil, err
	}
	if runtime.GOOS == "linux" {
		if out, err := runPgAsLocalSuperuser(bin, args...); err == nil {
			return out, nil
		}
		for _, pass := range s.postgresSuperPasswordCandidates() {
			if pass == "" {
				continue
			}
			if out, err := runPgTCP(bin, "postgres", pass, args...); err == nil {
				return out, nil
			}
		}
	}
	if pass := s.getStoredPostgresSuperPassword(); pass != "" {
		return runPgTCP(bin, "postgres", pass, args...)
	}
	cmd := exec.Command(bin, append([]string{"-h", "127.0.0.1", "-U", "postgres", "-v", "ON_ERROR_STOP=1"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	if strings.Contains(string(out), "ERROR") {
		return out, fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return out, nil
}

func runPgAsLocalSuperuser(bin string, args ...string) ([]byte, error) {
	base := append([]string{"-v", "ON_ERROR_STOP=1"}, args...)
	cmd := exec.Command("sudo", append([]string{"-n", "-u", "postgres", bin}, base...)...)
	out, err := cmd.CombinedOutput()
	outStr := string(out)
	if err != nil {
		return out, fmt.Errorf("%s", strings.TrimSpace(outStr))
	}
	if strings.Contains(outStr, "ERROR") || strings.Contains(outStr, "fe_sendauth") || strings.Contains(outStr, "Password for user") {
		return out, fmt.Errorf("%s", strings.TrimSpace(outStr))
	}
	return out, nil
}

func runPgTCP(bin, user, password string, args ...string) ([]byte, error) {
	base := []string{"-h", "127.0.0.1", "-U", user, "-v", "ON_ERROR_STOP=1"}
	base = append(base, args...)
	cmd := exec.Command(bin, base...)
	if password != "" {
		cmd.Env = append(os.Environ(), "PGPASSWORD="+password)
	}
	out, err := cmd.CombinedOutput()
	outStr := string(out)
	if err != nil {
		return out, fmt.Errorf("%s", strings.TrimSpace(outStr))
	}
	if strings.Contains(outStr, "ERROR") || strings.Contains(outStr, "fe_sendauth") {
		return out, fmt.Errorf("%s", strings.TrimSpace(outStr))
	}
	return out, nil
}

func (s *Service) postgresSuperPasswordCandidates() []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	add(s.getStoredPostgresSuperPassword())
	if s.db != nil {
		var inst models.DatabaseInstance
		if s.db.Where("type IN ? AND username = ? AND host IN ? AND password != ''",
			[]string{"postgresql", "postgres"}, "postgres", localHosts()).
			Order("id desc").First(&inst).Error == nil {
			add(inst.Password)
		}
	}
	return out
}

func (s *Service) getStoredPostgresSuperPassword() string {
	if s.db == nil {
		return ""
	}
	var row models.PanelSetting
	if s.db.Where("key = ?", postgresSuperPasswordKey).First(&row).Error == nil {
		return strings.TrimSpace(row.Value)
	}
	return ""
}

func (s *Service) storePostgresSuperPassword(password string) {
	if s.db == nil || strings.TrimSpace(password) == "" {
		return
	}
	s.db.Where(models.PanelSetting{Key: postgresSuperPasswordKey}).
		Assign(models.PanelSetting{Value: password}).
		FirstOrCreate(&models.PanelSetting{Key: postgresSuperPasswordKey})
}

// EnsurePostgresSuperuserAuth sets a known postgres password for TCP clients (127.0.0.1).
func (s *Service) EnsurePostgresSuperuserAuth() error {
	bin, err := findBinary("psql")
	if err != nil {
		return nil
	}
	if pass := s.getStoredPostgresSuperPassword(); pass != "" {
		if _, err := runPgTCP(bin, "postgres", pass, "-tAc", "SELECT 1;"); err == nil {
			return nil
		}
	}
	if _, err := runPgAsLocalSuperuser(bin, "-tAc", "SELECT 1;"); err != nil {
		return err
	}
	pass, err := randomPostgresPassword(20)
	if err != nil {
		return err
	}
	esc := strings.ReplaceAll(pass, "'", "''")
	q := fmt.Sprintf("ALTER USER postgres WITH PASSWORD '%s';", esc)
	if _, err := runPgAsLocalSuperuser(bin, "-c", q); err != nil {
		return err
	}
	s.storePostgresSuperPassword(pass)
	return nil
}

func randomPostgresPassword(n int) (string, error) {
	b := make([]byte, (n+1)/2)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b)[:n], nil
}
