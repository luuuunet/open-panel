package logs

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ClearResult struct {
	ClearedFiles int   `json:"cleared_files"`
	BytesFreed   int64 `json:"bytes_freed"`
	Skipped      int   `json:"skipped"`
}

type CleanResult struct {
	DeletedFiles int   `json:"deleted_files"`
	TrimmedFiles int   `json:"trimmed_files"`
	BytesFreed   int64 `json:"bytes_freed"`
}

func (s *Service) ClearAll() (ClearResult, error) {
	if !s.IsLoggingEnabled() {
		return ClearResult{}, ErrLoggingDisabled
	}
	var result ClearResult
	seen := map[string]bool{}
	for _, src := range s.DiscoverSources() {
		if src.Virtual || src.LogKind == "journal" || src.LogKind == "docker" ||
			strings.HasSuffix(src.ID, ".docker") || src.ID == "system.journal" {
			result.Skipped++
			continue
		}
		path := strings.TrimSpace(src.Path)
		if path == "" || path == "journalctl" {
			result.Skipped++
			continue
		}
		if !s.offerPath(path) {
			result.Skipped++
			continue
		}
		cleanPath := filepath.Clean(path)
		if seen[cleanPath] {
			continue
		}
		seen[cleanPath] = true

		st, err := os.Stat(cleanPath)
		if err != nil {
			result.Skipped++
			continue
		}
		if st.IsDir() {
			result.Skipped++
			continue
		}
		if st.Size() == 0 {
			continue
		}
		if err := os.Truncate(cleanPath, 0); err != nil {
			result.Skipped++
			continue
		}
		result.ClearedFiles++
		result.BytesFreed += st.Size()
	}
	return result, nil
}

func (s *Service) CleanOlderThan(days int) (CleanResult, error) {
	if !s.IsLoggingEnabled() {
		return CleanResult{}, ErrLoggingDisabled
	}
	if days <= 0 {
		return CleanResult{}, nil
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	var result CleanResult
	seen := map[string]bool{}

	deleteFile := func(path string) {
		cleanPath := filepath.Clean(path)
		if seen[cleanPath] {
			return
		}
		if !s.offerPath(cleanPath) {
			return
		}
		st, err := os.Stat(cleanPath)
		if err != nil || st.IsDir() {
			return
		}
		if st.ModTime().After(cutoff) {
			return
		}
		size := st.Size()
		if err := os.Remove(cleanPath); err != nil {
			return
		}
		seen[cleanPath] = true
		result.DeletedFiles++
		result.BytesFreed += size
	}

	s.walkLogFiles(func(path string, info os.FileInfo) {
		if info.IsDir() {
			return
		}
		deleteFile(path)
	})

	for _, src := range s.DiscoverSources() {
		if src.Virtual {
			continue
		}
		path := strings.TrimSpace(src.Path)
		if path == "" || path == "journalctl" || !s.offerPath(path) {
			continue
		}
		cleanPath := filepath.Clean(path)
		for _, candidate := range rotatedVariants(cleanPath) {
			deleteFile(candidate)
		}
	}

	for _, src := range s.DiscoverSources() {
		if src.Virtual {
			continue
		}
		path := strings.TrimSpace(src.Path)
		if path == "" || path == "journalctl" || !s.offerPath(path) {
			continue
		}
		cleanPath := filepath.Clean(path)
		if seen[cleanPath] {
			continue
		}
		st, err := os.Stat(cleanPath)
		if err != nil || st.IsDir() {
			continue
		}
		freed, trimmed, err := trimFileOlderThan(cleanPath, cutoff, st.Size())
		if err != nil || !trimmed {
			continue
		}
		result.TrimmedFiles++
		result.BytesFreed += freed
	}

	return result, nil
}

func (s *Service) walkLogFiles(fn func(path string, info os.FileInfo)) {
	logRoot := filepath.Join(s.dataDir, "logs")
	_ = filepath.Walk(logRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		fn(path, info)
		return nil
	})
}

func rotatedVariants(activePath string) []string {
	dir := filepath.Dir(activePath)
	base := filepath.Base(activePath)
	baseLower := strings.ToLower(base)
	var out []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if lower == baseLower {
			continue
		}
		if strings.HasSuffix(lower, ".gz") ||
			strings.HasPrefix(lower, baseLower+".") ||
			strings.HasPrefix(lower, strings.TrimSuffix(baseLower, ".log")+".log.") ||
			strings.Contains(lower, strings.TrimSuffix(baseLower, ".log")+".log-") {
			out = append(out, filepath.Join(dir, name))
		}
	}
	return out
}

func trimFileOlderThan(path string, cutoff time.Time, sizeBefore int64) (bytesFreed int64, trimmed bool, err error) {
	if sizeBefore < 64*1024 {
		return 0, false, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, false, err
	}
	defer f.Close()

	var kept []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	dropped := 0
	for sc.Scan() {
		line := sc.Text()
		if ts, ok := lineTimestamp(line); ok && ts.Before(cutoff) {
			dropped++
			continue
		}
		kept = append(kept, line)
	}
	if err := sc.Err(); err != nil {
		return 0, false, err
	}
	if dropped == 0 {
		return 0, false, nil
	}

	tmpPath := path + ".op-trim"
	out, err := os.Create(tmpPath)
	if err != nil {
		return 0, false, err
	}
	w := bufio.NewWriter(out)
	for i, line := range kept {
		if i > 0 {
			if err := w.WriteByte('\n'); err != nil {
				out.Close()
				os.Remove(tmpPath)
				return 0, false, err
			}
		}
		if _, err := w.WriteString(line); err != nil {
			out.Close()
			os.Remove(tmpPath)
			return 0, false, err
		}
	}
	if err := w.Flush(); err != nil {
		out.Close()
		os.Remove(tmpPath)
		return 0, false, err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmpPath)
		return 0, false, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return 0, false, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return int64(dropped), true, nil
	}
	return sizeBefore - st.Size(), true, nil
}

func lineTimestamp(line string) (time.Time, bool) {
	line = strings.TrimSpace(line)
	if len(line) >= 10 {
		if t, err := time.Parse("2006-01-02", line[:10]); err == nil {
			return t, true
		}
		if t, err := time.Parse(time.RFC3339, line[:min(len(line), 25)]); err == nil {
			return t, true
		}
	}
	if idx := strings.Index(line, "["); idx >= 0 {
		rest := line[idx+1:]
		if len(rest) >= 20 {
			if t, err := time.Parse("02/Jan/2006:15:04:05", rest[:20]); err == nil {
				return t, true
			}
		}
		if len(rest) >= 11 {
			if t, err := time.Parse("02/Jan/2006:15:04:05", rest[:11]); err == nil {
				return t, true
			}
		}
	}
	fields := strings.Fields(line)
	if len(fields) >= 3 {
		layouts := []string{
			"Jan 2 15:04:05",
			"Jan 02 15:04:05",
			"2006 Jan 2 15:04:05",
		}
		for _, layout := range layouts {
			var chunk string
			switch layout {
			case "2006 Jan 2 15:04:05":
				if len(fields) >= 4 {
					chunk = strings.Join(fields[:4], " ")
				}
			default:
				chunk = strings.Join(fields[:3], " ")
			}
			if chunk == "" {
				continue
			}
			if t, err := time.Parse(layout, chunk); err == nil {
				if t.Year() == 0 {
					now := time.Now()
					t = time.Date(now.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, t.Location())
				}
				return t, true
			}
		}
	}
	return time.Time{}, false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
