package extension

// Manifest describes an OWPanel extension installed under dataDir/extensions/<id>/.
type Manifest struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Author      string            `json:"author"`
	Enabled     *bool             `json:"enabled"`
	Menu        []MenuItem        `json:"menu"`
	Hooks       map[string][]string `json:"hooks"`
	Catalog     []CatalogApp      `json:"catalog"`
	Settings    map[string]string `json:"settings"`
}

type MenuItem struct {
	Path     string `json:"path"`
	Title    string `json:"title"`
	Icon     string `json:"icon"`
	Group    string `json:"group"`
	Admin    bool   `json:"admin"`
	Perm     string `json:"perm"`
	EmbedURL string `json:"embed_url"`
	External string `json:"external_url"`
}

type CatalogApp struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Version     string `json:"version"`
	Versions    string `json:"versions"`
	Description string `json:"description"`
	Port        int    `json:"port"`
	InstallPath string `json:"install_path"`
	ConfigPath  string `json:"config_path"`
	Icon        string `json:"icon"`
}

type ExtensionInfo struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Version     string     `json:"version"`
	Description string     `json:"description"`
	Author      string     `json:"author"`
	Enabled     bool       `json:"enabled"`
	Dir         string     `json:"dir"`
	Menu        []MenuItem `json:"menu"`
	Hooks       []string   `json:"hooks"`
	Catalog     int        `json:"catalog_count"`
}

type MenuItemView struct {
	Path       string `json:"path"`
	Title      string `json:"title"`
	Icon       string `json:"icon"`
	Group      string `json:"group"`
	GroupTitle string `json:"group_title"`
	Admin      bool   `json:"admin"`
	Perm       string `json:"perm"`
	EmbedURL   string `json:"embed_url,omitempty"`
	External   string `json:"external_url,omitempty"`
	Extension  string `json:"extension_id"`
}

func (m *Manifest) isEnabled() bool {
	if m.Enabled == nil {
		return true
	}
	return *m.Enabled
}
