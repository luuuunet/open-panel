package cache

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

type PurgePathRequest struct {
	Paths []string `json:"paths"`
}

// PurgePaths removes cached entries for specific URL paths on a site.
func (s *Service) PurgePaths(domain string, paths []string) (*PurgeResult, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return nil, fmt.Errorf("domain required")
	}
	var site models.Website
	if err := s.db.Where("domain = ?", domain).First(&site).Error; err != nil {
		return nil, fmt.Errorf("site not found: %s", domain)
	}

	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}

	normalized := normalizePaths(paths)
	if len(normalized) == 0 {
		return s.PurgeSite(domain)
	}

	pxDir := s.SiteProxyCacheDir(&site)
	fcDir := s.SiteFastCGICacheDir(&site)
	var cleared int64
	methods := []string{"GET", "HEAD"}
	schemes := []string{"http", "https"}

	for _, rawPath := range normalized {
		for _, scheme := range schemes {
			for _, method := range methods {
				for _, cacheKey := range cacheKeyVariants(&site, cfg, scheme, method, domain, rawPath) {
					for _, dir := range []string{pxDir, fcDir} {
						cleared += deleteCacheKeyFile(dir, cacheKey)
					}
				}
			}
		}
		// Prefix purge: walk cache tree and remove entries whose key file might match path segment
		cleared += purgePathPrefix(pxDir, rawPath)
		cleared += purgePathPrefix(fcDir, rawPath)
	}

	msg := fmt.Sprintf("已按路径清理站点 %s 的 CDN 缓存（%d 个条目）", domain, len(normalized))
	if cleared == 0 {
		msg = fmt.Sprintf("站点 %s 未找到匹配路径的缓存文件，可能已过期或未缓存", domain)
	}
	return &PurgeResult{
		ClearedBytes: cleared,
		Message:      msg,
		Domain:       domain,
	}, nil
}

func normalizePaths(paths []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		if idx := strings.Index(p, "?"); idx >= 0 {
			p = p[:idx]
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func cacheKeyVariants(site *models.Website, cfg *models.CacheConfig, scheme, method, host, uri string) []string {
	withQuery := cfg != nil && cfg.CacheQueryString
	base := fmt.Sprintf(`%s%s%s%s`, scheme, method, host, uri)
	keys := []string{fmt.Sprintf(`"%s"`, base)}
	if !withQuery {
		keys = append(keys, fmt.Sprintf(`"%s%s%s%s"`, scheme, method, host, uri))
	}
	_ = site
	return keys
}

func deleteCacheKeyFile(cacheDir, cacheKey string) int64 {
	path := cacheFilePath(cacheDir, cacheKey)
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if err := os.Remove(path); err != nil {
		return 0
	}
	// Nginx also stores metadata alongside; remove .key suffix variant
	_ = os.Remove(path + ".key")
	return info.Size()
}

// cacheFilePath maps an nginx cache key to on-disk path (levels=1:2).
func cacheFilePath(cacheDir, cacheKey string) string {
	sum := md5.Sum([]byte(cacheKey))
	hex := fmt.Sprintf("%x", sum)
	if len(hex) < 3 {
		return filepath.Join(cacheDir, hex)
	}
	return filepath.Join(cacheDir, hex[len(hex)-1:], hex[len(hex)-3:len(hex)-1], hex)
}

func purgePathPrefix(cacheDir, uriPath string) int64 {
	if cacheDir == "" || uriPath == "" {
		return 0
	}
	// Conservative fallback: if exact key miss, purge files under dirs whose names contain path hash fragments
	segment := strings.Trim(uriPath, "/")
	if segment == "" {
		return 0
	}
	parts := strings.Split(segment, "/")
	target := parts[len(parts)-1]
	if len(target) < 2 {
		return 0
	}
	var cleared int64
	_ = filepath.WalkDir(cacheDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.Contains(strings.ToLower(d.Name()), strings.ToLower(target)) {
			if info, err := d.Info(); err == nil {
				cleared += info.Size()
			}
			_ = os.Remove(path)
		}
		return nil
	})
	return cleared
}
