package stackscripts

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed all:stack
var embedded embed.FS

// HasEmbedded reports whether stack install scripts were baked into the binary.
func HasEmbedded() bool {
	entries, err := fs.ReadDir(embedded, "stack")
	return err == nil && len(entries) > 0
}

// ExtractTo writes embedded stack scripts to dest (idempotent; skips if fallback.sh exists).
func ExtractTo(dest string) error {
	if !HasEmbedded() {
		return fs.ErrNotExist
	}
	dest = filepath.Clean(dest)
	if st, err := os.Stat(filepath.Join(dest, "fallback.sh")); err == nil && !st.IsDir() {
		return nil
	}
	return fs.WalkDir(embedded, "stack", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, "stack/")
		if rel == "" || rel == "." {
			return nil
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := embedded.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		mode := os.FileMode(0644)
		if strings.HasSuffix(rel, ".sh") {
			mode = 0755
		}
		return os.WriteFile(target, data, mode)
	})
}
