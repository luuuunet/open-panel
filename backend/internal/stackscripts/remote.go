package stackscripts

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/version"
)

const (
	githubRepo       = "luuuunet/owpanel"
	rawBaseMain      = "https://raw.githubusercontent.com/luuuunet/owpanel/main/scripts/stack"
	releaseAssetName = "owpanel-stack-scripts.tar.gz"
)

// RemoteBase returns the GitHub raw URL prefix for stack scripts (tag > main).
func RemoteBase() string {
	if tag := strings.TrimSpace(version.Version); tag != "" && tag != "dev" && strings.HasPrefix(tag, "v") {
		return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/scripts/stack", githubRepo, tag)
	}
	if commit := strings.TrimSpace(version.GitCommit); commit != "" && commit != "unknown" {
		return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/scripts/stack", githubRepo, commit)
	}
	return rawBaseMain
}

// DownloadTo tries release tarball, then raw GitHub files, then embedded extract.
func DownloadTo(dest string) error {
	dest = filepath.Clean(dest)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	if st, err := os.Stat(filepath.Join(dest, "fallback.sh")); err == nil && !st.IsDir() {
		return nil
	}
	var lastErr error
	for _, url := range releaseTarballURLs() {
		if err := downloadReleaseTarball(url, dest); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if err := downloadRawScripts(dest); err == nil {
		return nil
	} else if lastErr == nil {
		lastErr = err
	}
	if err := ExtractTo(dest); err == nil {
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("无法获取 stack 安装脚本（GitHub / 内置均失败）: %w", lastErr)
	}
	return fmt.Errorf("无法获取 stack 安装脚本")
}

func releaseTarballURLs() []string {
	tag := strings.TrimSpace(version.Version)
	var urls []string
	if tag != "" && tag != "dev" && strings.HasPrefix(tag, "v") {
		urls = append(urls,
			fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", githubRepo, tag, releaseAssetName),
		)
	}
	urls = append(urls,
		fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", githubRepo, releaseAssetName),
	)
	return urls
}

func downloadReleaseTarball(url, dest string) error {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	gz, err := gzip.NewReader(resp.Body)
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
		name := strings.TrimPrefix(hdr.Name, "./")
		if name == "" || strings.Contains(name, "..") {
			continue
		}
		target := filepath.Join(dest, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "fallback.sh")); err != nil {
		return fmt.Errorf("tarball missing fallback.sh")
	}
	return nil
}

var rawScriptFiles = []string{
	"common.sh", "fallback.sh",
	"install-nginx.sh", "install-mariadb.sh", "install-redis.sh",
	"install-postgresql.sh", "install-mongodb.sh", "install-docker.sh",
	"install-apache.sh", "install-openresty.sh", "install-certbot.sh",
	"install-php.sh", "install-generic.sh", "manifest.json",
}

func downloadRawScripts(dest string) error {
	base := RemoteBase()
	client := &http.Client{Timeout: 60 * time.Second}
	for _, name := range rawScriptFiles {
		url := base + "/" + name
		resp, err := client.Get(url)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
		}
		mode := os.FileMode(0644)
		if strings.HasSuffix(name, ".sh") {
			mode = 0755
		}
		if err := os.WriteFile(filepath.Join(dest, name), body, mode); err != nil {
			return err
		}
	}
	return nil
}
