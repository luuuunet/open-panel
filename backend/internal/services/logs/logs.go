package logs

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

type Entry struct {
	Source   string `json:"source"`
	SourceID string `json:"source_id,omitempty"`
	Category string `json:"category,omitempty"`
	Content  string `json:"content"`
	Path     string `json:"path,omitempty"`
}

type TailResult struct {
	SourceID string `json:"source_id"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	Exists   bool   `json:"exists"`
	Size     int64  `json:"size"`
	Lines    int    `json:"lines"`
}

type Service struct {
	dataDir string
	db      *gorm.DB
}

func NewService(dataDir string, db *gorm.DB) *Service {
	return &Service{dataDir: dataDir, db: db}
}

func (s *Service) ListSources() ([]Source, []CategoryInfo) {
	sources := s.DiscoverSources()
	return sources, s.Categories(sources)
}

func (s *Service) SetEnabled(updates map[string]bool) error {
	cfg := s.loadConfig()
	if cfg.Enabled == nil {
		cfg.Enabled = map[string]bool{}
	}
	for id, enabled := range updates {
		cfg.Enabled[id] = enabled
	}
	return s.saveConfig(cfg)
}

func (s *Service) Tail(sourceID string, maxLines int) (*TailResult, error) {
	if !s.IsLoggingEnabled() {
		return nil, ErrLoggingDisabled
	}
	if maxLines <= 0 {
		maxLines = 200
	}
	if maxLines > 2000 {
		maxLines = 2000
	}
	sources := s.DiscoverSources()
	var src *Source
	for i := range sources {
		if sources[i].ID == sourceID {
			src = &sources[i]
			break
		}
	}
	if src == nil {
		return nil, os.ErrNotExist
	}
	return s.tailSource(src, maxLines)
}

func (s *Service) TailPath(path string, maxLines int) (*TailResult, error) {
	if maxLines <= 0 {
		maxLines = 200
	}
	if maxLines > 2000 {
		maxLines = 2000
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, os.ErrNotExist
	}
	return s.tailSource(&Source{ID: "path", Path: path}, maxLines)
}

func (s *Service) tailSource(src *Source, maxLines int) (*TailResult, error) {
	content, size, exists := s.readSource(src, maxLines)
	return &TailResult{
		SourceID: src.ID,
		Path:     src.Path,
		Content:  content,
		Exists:   exists,
		Size:     size,
		Lines:    countLines(content),
	}, nil
}

func (s *Service) Combined(maxLines int) ([]Entry, error) {
	if !s.IsLoggingEnabled() {
		return nil, nil
	}
	if maxLines <= 0 {
		maxLines = 200
	}
	perSource := maxLines / 4
	if perSource < 50 {
		perSource = 50
	}
	if perSource > 300 {
		perSource = 300
	}

	var entries []Entry
	for _, src := range s.DiscoverSources() {
		if !src.Enabled {
			continue
		}
		content, _, exists := s.readSource(&src, perSource)
		if !exists || strings.TrimSpace(content) == "" {
			continue
		}
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimRight(line, "\r")
			if line == "" {
				continue
			}
			entries = append(entries, Entry{
				Source:   src.Name,
				SourceID: src.ID,
				Category: src.Category,
				Content:  line,
				Path:     src.Path,
			})
		}
	}
	if len(entries) > maxLines {
		entries = entries[len(entries)-maxLines:]
	}
	return entries, nil
}

// List keeps backward compatibility with the old audit API.
func (s *Service) List(limit int) ([]Entry, error) {
	return s.Combined(limit)
}

func (s *Service) readSource(src *Source, maxLines int) (content string, size int64, exists bool) {
	if src.Virtual {
		if src.LogKind == "journal" || src.ID == "system.journal" {
			return readJournal(maxLines)
		}
		if src.LogKind == "docker" || strings.HasSuffix(src.ID, ".docker") {
			return readDockerLogs(src.Path, maxLines)
		}
	}
	st, err := os.Stat(src.Path)
	if err != nil || st.IsDir() {
		return "", 0, false
	}
	size = st.Size()
	return tailFile(src.Path, maxLines), size, true
}

func readDockerLogs(container string, maxLines int) (string, int64, bool) {
	if container == "" {
		return "", 0, false
	}
	out, err := exec.Command("docker", "logs", "--tail", strconv.Itoa(maxLines), container).CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), 0, false
	}
	text := strings.TrimRight(string(out), "\n")
	return text, int64(len(out)), true
}

func readJournal(maxLines int) (string, int64, bool) {
	if runtime.GOOS != "linux" {
		return "", 0, false
	}
	out, err := exec.Command("journalctl", "-n", strconv.Itoa(maxLines), "--no-pager", "-o", "short-iso").CombinedOutput()
	if err != nil {
		return "", 0, false
	}
	return strings.TrimRight(string(out), "\n"), int64(len(out)), true
}

func tailFile(path string, maxLines int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	var ring []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if len(ring) >= maxLines {
			ring = ring[1:]
		}
		ring = append(ring, line)
	}
	return strings.Join(ring, "\n")
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Split(s, "\n"))
}
