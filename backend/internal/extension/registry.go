package extension

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/luuuunet/owpanel/internal/models"
)

type loadedExtension struct {
	dir      string
	manifest Manifest
	enabled  bool
}

type Registry struct {
	dataDir string
	mu      sync.RWMutex
	items   []loadedExtension
}

func NewRegistry(dataDir string) *Registry {
	r := &Registry{dataDir: dataDir}
	r.Reload()
	return r
}

func (r *Registry) Reload() int {
	dir := filepath.Join(r.dataDir, "extensions")
	_ = os.MkdirAll(dir, 0755)
	r.removeSampleExtension(dir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("[extension] read dir: %v", err)
		return 0
	}

	var loaded []loadedExtension
	for _, ent := range entries {
		if !ent.IsDir() || ent.Name() == "_sample" {
			continue
		}
		extDir := filepath.Join(dir, ent.Name())
		manifestPath := filepath.Join(extDir, "manifest.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			log.Printf("[extension] invalid manifest %s: %v", manifestPath, err)
			continue
		}
		if m.ID == "" {
			m.ID = ent.Name()
		}
		loaded = append(loaded, loadedExtension{
			dir:      extDir,
			manifest: m,
			enabled:  m.isEnabled(),
		})
	}
	sort.Slice(loaded, func(i, j int) bool {
		return loaded[i].manifest.Name < loaded[j].manifest.Name
	})

	r.mu.Lock()
	r.items = loaded
	r.mu.Unlock()
	return len(loaded)
}

func (r *Registry) removeSampleExtension(dir string) {
	_ = os.RemoveAll(filepath.Join(dir, "_sample"))
}

func (r *Registry) Get(id string) (*ExtensionInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, ext := range r.items {
		if ext.manifest.ID == id {
			hookEvents := make([]string, 0, len(ext.manifest.Hooks))
			for ev := range ext.manifest.Hooks {
				hookEvents = append(hookEvents, ev)
			}
			sort.Strings(hookEvents)
			info := ExtensionInfo{
				ID:          ext.manifest.ID,
				Name:        ext.manifest.Name,
				Version:     ext.manifest.Version,
				Description: ext.manifest.Description,
				Author:      ext.manifest.Author,
				Enabled:     ext.enabled,
				Dir:         ext.dir,
				Menu:        ext.manifest.Menu,
				Hooks:       hookEvents,
				Catalog:     len(ext.manifest.Catalog),
			}
			return &info, true
		}
	}
	return nil, false
}

func (r *Registry) ResolveEmbedURL(id string) (embedURL, title string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, ext := range r.items {
		if ext.manifest.ID != id {
			continue
		}
		for _, m := range ext.manifest.Menu {
			if m.EmbedURL != "" {
				return m.EmbedURL, m.Title
			}
			if m.Path == "/ext/"+id || m.Path == "ext/"+id {
				title = m.Title
			}
		}
		if title == "" {
			title = ext.manifest.Name
		}
		return "", title
	}
	return "", ""
}

func (r *Registry) List() []ExtensionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ExtensionInfo, 0, len(r.items))
	for _, ext := range r.items {
		hookEvents := make([]string, 0, len(ext.manifest.Hooks))
		for ev := range ext.manifest.Hooks {
			hookEvents = append(hookEvents, ev)
		}
		sort.Strings(hookEvents)
		out = append(out, ExtensionInfo{
			ID:          ext.manifest.ID,
			Name:        ext.manifest.Name,
			Version:     ext.manifest.Version,
			Description: ext.manifest.Description,
			Author:      ext.manifest.Author,
			Enabled:     ext.enabled,
			Dir:         ext.dir,
			Menu:        ext.manifest.Menu,
			Hooks:       hookEvents,
			Catalog:     len(ext.manifest.Catalog),
		})
	}
	return out
}

func (r *Registry) MenuItems() []MenuItemView {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []MenuItemView
	groupTitles := map[string]string{
		"extensions": "扩展",
		"tools":      "工具",
		"website":    "网站",
		"security":   "安全",
		"docker":     "容器",
	}
	for _, ext := range r.items {
		if !ext.enabled {
			continue
		}
		for _, m := range ext.manifest.Menu {
			if m.Path == "" || m.Title == "" {
				continue
			}
			path := m.Path
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}
			group := m.Group
			if group == "" {
				group = "extensions"
			}
			out = append(out, MenuItemView{
				Path:       path,
				Title:      m.Title,
				Icon:       defaultIcon(m.Icon),
				Group:      group,
				GroupTitle: groupTitles[group],
				Admin:      m.Admin,
				Perm:       m.Perm,
				EmbedURL:   m.EmbedURL,
				External:   m.External,
				Extension:  ext.manifest.ID,
			})
		}
	}
	return out
}

func defaultIcon(icon string) string {
	if strings.TrimSpace(icon) == "" {
		return "Box"
	}
	return icon
}

func (r *Registry) CatalogApps() []models.App {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []models.App
	for _, ext := range r.items {
		if !ext.enabled {
			continue
		}
		for _, c := range ext.manifest.Catalog {
			if c.Key == "" || c.Name == "" {
				continue
			}
			ver := c.Version
			if ver == "" {
				ver = "latest"
			}
			versions := c.Versions
			if versions == "" {
				versions = ver
			}
			icon := c.Icon
			if icon == "" {
				icon = "Box"
			}
			out = append(out, models.App{
				Key:         c.Key,
				Name:        c.Name,
				Category:    c.Category,
				Version:     ver,
				Versions:    versions,
				Description: c.Description,
				Port:        c.Port,
				InstallPath: c.InstallPath,
				ConfigPath:  c.ConfigPath,
				Icon:        icon,
			})
		}
	}
	return out
}

func (r *Registry) SetEnabled(id string, enabled bool) error {
	r.mu.RLock()
	var target *loadedExtension
	for i := range r.items {
		if r.items[i].manifest.ID == id {
			target = &r.items[i]
			break
		}
	}
	r.mu.RUnlock()
	if target == nil {
		return os.ErrNotExist
	}
	manifestPath := filepath.Join(target.dir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	m.Enabled = &enabled
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(manifestPath, out, 0644); err != nil {
		return err
	}
	r.Reload()
	return nil
}

func (r *Registry) ExtensionsDir() string {
	return filepath.Join(r.dataDir, "extensions")
}
