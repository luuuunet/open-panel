package logs

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type RotateResult struct {
	RotatedFiles   int   `json:"rotated_files"`
	CompressedFiles int  `json:"compressed_files"`
	BytesFreed     int64 `json:"bytes_freed"`
}

func (s *Service) RotateOversizedLogs() (RotateResult, error) {
	if !s.IsLoggingEnabled() {
		return RotateResult{}, ErrLoggingDisabled
	}
	cfg := s.loadConfig()
	maxMB := cfg.MaxSizeMB
	if maxMB <= 0 {
		maxMB = 50
	}
	maxFiles := cfg.MaxRotatedFiles
	if maxFiles <= 0 {
		maxFiles = 5
	}
	maxBytes := int64(maxMB) * 1024 * 1024
	var result RotateResult

	for _, src := range s.DiscoverSources() {
		if src.Virtual || src.LogKind == "journal" || src.LogKind == "docker" {
			continue
		}
		path := strings.TrimSpace(src.Path)
		if path == "" || path == "journalctl" || !s.offerPath(path) {
			continue
		}
		path = filepath.Clean(path)
		st, err := os.Stat(path)
		if err != nil || st.IsDir() || st.Size() < maxBytes {
			continue
		}
		if err := rotateLogFile(path, maxFiles, compressRotated(cfg.CompressRotated), &result); err != nil {
			continue
		}
		result.RotatedFiles++
	}
	return result, nil
}

func rotateLogFile(activePath string, maxFiles int, compress bool, result *RotateResult) error {
	dir := filepath.Dir(activePath)
	base := filepath.Base(activePath)

	for i := maxFiles; i >= 1; i-- {
		src := rotatedName(dir, base, i-1)
		dst := rotatedName(dir, base, i)
		if i == maxFiles {
			if st, err := os.Stat(src); err == nil && !st.IsDir() {
				if compress {
					gz := src + ".gz"
					if err := gzipFile(src, gz); err == nil {
						_ = os.Remove(src)
						result.CompressedFiles++
						result.BytesFreed += st.Size()
					}
				} else {
					_ = os.Remove(src)
					result.BytesFreed += st.Size()
				}
			}
			continue
		}
		if _, err := os.Stat(src); err == nil {
			_ = os.Rename(src, dst)
		}
	}
	if err := os.Rename(activePath, rotatedName(dir, base, 1)); err != nil {
		return err
	}
	f, err := os.Create(activePath)
	if err != nil {
		return err
	}
	return f.Close()
}

func rotatedName(dir, base string, n int) string {
	if n <= 0 {
		return filepath.Join(dir, base)
	}
	return filepath.Join(dir, fmt.Sprintf("%s.%d", base, n))
}

func gzipFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	gz := gzip.NewWriter(out)
	if _, err := io.Copy(gz, in); err != nil {
		gz.Close()
		return err
	}
	return gz.Close()
}
