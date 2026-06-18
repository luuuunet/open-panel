package waf

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type GeoIPInstallResult struct {
	Installed []string `json:"installed"`
	DBPath    string   `json:"db_path"`
	DBSize    int64    `json:"db_size"`
	Source    string   `json:"source"`
}

func (s *Service) InstallGeoDB() (*GeoIPInstallResult, error) {
	if err := os.MkdirAll(s.confDir, 0755); err != nil {
		return nil, fmt.Errorf("create security dir: %w", err)
	}

	var installed []string
	var source string

	licenseKey := strings.TrimSpace(os.Getenv("MAXMIND_LICENSE_KEY"))
	if licenseKey != "" {
		for _, edition := range []struct {
			edition, filename string
		}{
			{"GeoLite2-Country", "GeoLite2-Country.mmdb"},
			{"GeoLite2-City", "GeoLite2-City.mmdb"},
		} {
			if err := s.downloadMaxMindEdition(licenseKey, edition.edition, edition.filename); err == nil {
				installed = append(installed, edition.filename)
				source = "maxmind"
			}
		}
	}

	if len(installed) == 0 {
		dest := filepath.Join(s.confDir, "GeoLite2-Country.mmdb")
		if err := s.downloadLoyalsoldierCountry(dest); err != nil {
			return nil, err
		}
		installed = append(installed, "GeoLite2-Country.mmdb")
		source = "loyalsoldier"
	}

	dbPath := filepath.Join(s.confDir, "GeoLite2-Country.mmdb")
	var dbSize int64
	if st, err := os.Stat(dbPath); err == nil {
		dbSize = st.Size()
	}

	return &GeoIPInstallResult{
		Installed: installed,
		DBPath:    dbPath,
		DBSize:    dbSize,
		Source:    source,
	}, nil
}

func (s *Service) downloadMaxMindEdition(licenseKey, edition, filename string) error {
	url := fmt.Sprintf(
		"https://download.maxmind.com/app/geoip_download?edition_id=%s&license_key=%s&suffix=tar.gz",
		edition, licenseKey,
	)
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("owpanel-%s-%d.tar.gz", edition, time.Now().UnixNano()))
	defer os.Remove(tmp)
	if err := downloadGeoFile(url, tmp); err != nil {
		return err
	}
	return extractMMDB(tmp, filepath.Join(s.confDir, filename))
}

func (s *Service) downloadLoyalsoldierCountry(dest string) error {
	url, err := loyalsoldierCountryURL()
	if err != nil {
		return err
	}
	tmp := dest + ".tmp"
	if err := downloadGeoFile(url, tmp); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("download GeoLite2-Country: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("save GeoLite2-Country: %w", err)
	}
	return nil
}

func loyalsoldierCountryURL() (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/Loyalsoldier/geoip/releases/latest")
	if err != nil {
		return "", fmt.Errorf("fetch release info: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch release info: HTTP %d", resp.StatusCode)
	}
	var release struct {
		Assets []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("parse release info: %w", err)
	}
	for _, a := range release.Assets {
		if a.Name == "Country.mmdb" {
			return a.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("Country.mmdb not found in latest Loyalsoldier release")
}

func downloadGeoFile(url, dest string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		f, err := os.Create(dest)
		if err == nil {
			_, copyErr := io.Copy(f, resp.Body)
			f.Close()
			if copyErr == nil {
				return nil
			}
		}
	}
	out, err := exec.Command("curl", "-fsSL", "-o", dest, url).CombinedOutput()
	if err == nil {
		if st, statErr := os.Stat(dest); statErr == nil && st.Size() > 0 {
			return nil
		}
	}
	if err != nil {
		return fmt.Errorf("%s (%s)", strings.TrimSpace(string(out)), url)
	}
	return fmt.Errorf("download failed: %s", url)
}

func extractMMDB(tarGz, dest string) error {
	f, err := os.Open(tarGz)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if !strings.HasSuffix(hdr.Name, ".mmdb") {
			continue
		}
		out, err := os.Create(dest)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		out.Close()
		return nil
	}
	return fmt.Errorf("no .mmdb found in archive")
}
