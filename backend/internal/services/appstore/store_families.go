package appstore

import (
	"fmt"
	"sort"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

// StoreVersionEntry is one installable version within a grouped store card.
type StoreVersionEntry struct {
	Key         string `json:"key"`
	Version     string `json:"version"`
	Installed   bool   `json:"installed"`
	Status      string `json:"status"`
	Port        int    `json:"port"`
	InstallPath string `json:"install_path,omitempty"`
}

// StoreListingItem is returned by the software store API (flat or grouped).
type StoreListingItem struct {
	models.App
	Grouped        bool                `json:"grouped"`
	FamilyKey      string              `json:"family_key,omitempty"`
	DescriptionEN  string              `json:"description_en,omitempty"`
	VersionEntries []StoreVersionEntry `json:"version_entries,omitempty"`
	DockerApp      bool                `json:"docker_app"`
	AccessURL      string              `json:"access_url,omitempty"`
}

type storeFamilyDef struct {
	Key           string
	Name          string
	NameEN        string
	Description   string
	DescriptionEN string
	Category      string
	Icon          string
	match         func(key string) bool
	sortVersion   func(a, b StoreVersionEntry) bool
}

func versionNumDesc(a, b string) bool {
	return parseVersionParts(a) > parseVersionParts(b)
}

func parseVersionParts(v string) float64 {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	parts := strings.Split(v, ".")
	var major, minor float64
	fmt.Sscanf(parts[0], "%f", &major)
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%f", &minor)
	}
	return major + minor/100
}

var storeFamilies = []storeFamilyDef{
	{
		Key: "php", Name: "PHP", NameEN: "PHP",
		Description:   "PHP 运行环境，可同时安装多个版本（独立 php-fpm 服务与端口）",
		DescriptionEN: "PHP runtime — install multiple versions side by side (separate php-fpm services and ports)",
		Category: "运行环境", Icon: "Coffee",
		match: func(key string) bool {
			return strings.HasPrefix(key, "php") && key != "phpmyadmin"
		},
		sortVersion: func(a, b StoreVersionEntry) bool { return versionNumDesc(a.Version, b.Version) },
	},
	{
		Key: "mysql", Name: "MySQL", NameEN: "MySQL",
		Description:   "MySQL 关系型数据库，支持多版本并存（独立数据目录与端口）",
		DescriptionEN: "MySQL relational database — multiple versions with separate data dirs and ports",
		Category: "数据库", Icon: "Coin",
		match: func(key string) bool {
			return key == "mysql" || strings.HasPrefix(key, "mysql") && key != "mysql" && len(key) > 5
		},
		sortVersion: func(a, b StoreVersionEntry) bool { return versionNumDesc(a.Version, b.Version) },
	},
	{
		Key: "java", Name: "OpenJDK", NameEN: "OpenJDK",
		Description:   "Java 运行环境（OpenJDK LTS），可同时安装多个版本",
		DescriptionEN: "Java runtime (OpenJDK LTS) — multiple versions side by side",
		Category: "运行环境", Icon: "CoffeeCup",
		match:       func(key string) bool { return strings.HasPrefix(key, "java") },
		sortVersion: func(a, b StoreVersionEntry) bool { return versionNumDesc(a.Version, b.Version) },
	},
	{
		Key: "nodejs", Name: "Node.js", NameEN: "Node.js",
		Description:   "Node.js 运行时（npm），可同时安装多个 LTS 版本",
		DescriptionEN: "Node.js runtime (npm) — multiple LTS versions side by side",
		Category: "运行环境", Icon: "Platform",
		match:       func(key string) bool { return strings.HasPrefix(key, "nodejs") },
		sortVersion: func(a, b StoreVersionEntry) bool { return versionNumDesc(a.Version, b.Version) },
	},
	{
		Key: "dotnet", Name: ".NET", NameEN: ".NET",
		Description:   ".NET 运行时（ASP.NET Core）",
		DescriptionEN: ".NET runtime (ASP.NET Core)",
		Category: "运行环境", Icon: "Platform",
		match:       func(key string) bool { return strings.HasPrefix(key, "dotnet") },
		sortVersion: func(a, b StoreVersionEntry) bool { return versionNumDesc(a.Version, b.Version) },
	},
	{
		Key: "tomcat", Name: "Tomcat", NameEN: "Tomcat",
		Description:   "Apache Tomcat Java Web 容器",
		DescriptionEN: "Apache Tomcat Java web container",
		Category: "运行环境", Icon: "CoffeeCup",
		match:       func(key string) bool { return strings.HasPrefix(key, "tomcat") },
		sortVersion: func(a, b StoreVersionEntry) bool { return versionNumDesc(a.Version, b.Version) },
	},
}

func familyForKey(key string) *storeFamilyDef {
	for i := range storeFamilies {
		if storeFamilies[i].match(key) {
			return &storeFamilies[i]
		}
	}
	return nil
}

// GroupStoreListing merges versioned apps (PHP, MySQL, JDK, …) into single store cards.
func GroupStoreListing(apps []models.App, enrich func(models.App) (dockerApp bool, accessURL string)) []StoreListingItem {
	byKey := make(map[string]models.App, len(apps))
	for _, app := range apps {
		byKey[app.Key] = app
	}

	consumed := make(map[string]struct{})
	var out []StoreListingItem

	for i := range storeFamilies {
		fam := &storeFamilies[i]
		var entries []StoreVersionEntry
		for _, app := range apps {
			if !fam.match(app.Key) {
				continue
			}
			consumed[app.Key] = struct{}{}
			entries = append(entries, StoreVersionEntry{
				Key: app.Key, Version: app.Version, Installed: app.Installed,
				Status: app.Status, Port: app.Port, InstallPath: app.InstallPath,
			})
		}
		if len(entries) == 0 {
			continue
		}
		if fam.sortVersion != nil {
			sort.Slice(entries, func(i, j int) bool { return fam.sortVersion(entries[i], entries[j]) })
		}

		versions := make([]string, 0, len(entries))
		for _, e := range entries {
			versions = append(versions, e.Version)
		}

		primary := byKey[entries[0].Key]
		for _, e := range entries {
			if e.Installed {
				primary = byKey[e.Key]
				break
			}
		}

		grouped := models.App{
			Key: primary.Key, Name: fam.Name, Category: fam.Category,
			Description: fam.Description, Version: entries[0].Version,
			Versions: strings.Join(versions, ","), Icon: fam.Icon,
			Port: primary.Port, InstallPath: primary.InstallPath,
		}
		grouped.Installed = false
		for _, e := range entries {
			if e.Installed {
				grouped.Installed = true
				grouped.Version = e.Version
				grouped.Status = e.Status
				break
			}
		}

		dockerApp, accessURL := enrich(primary)
		out = append(out, StoreListingItem{
			App: grouped, Grouped: true, FamilyKey: fam.Key,
			DescriptionEN: fam.DescriptionEN, VersionEntries: entries,
			DockerApp: dockerApp, AccessURL: accessURL,
		})
	}

	for _, app := range apps {
		if _, ok := consumed[app.Key]; ok {
			continue
		}
		dockerApp, accessURL := enrich(app)
		descEN := catalogDescriptionEN(app.Key)
		out = append(out, StoreListingItem{
			App: app, Grouped: false, DescriptionEN: descEN,
			DockerApp: dockerApp, AccessURL: accessURL,
		})
	}

	SortStoreListing(out)
	return out
}

func SortStoreListing(items []StoreListingItem) {
	rank := categoryRank()
	sort.Slice(items, func(i, j int) bool {
		ci := rank[NormalizeCategory(items[i].Category)]
		cj := rank[NormalizeCategory(items[j].Category)]
		if ci != cj {
			return ci < cj
		}
		return items[i].Name < items[j].Name
	})
}

// catalogDescriptionEN returns English description for common store apps.
func catalogDescriptionEN(key string) string {
	if d, ok := catalogDescriptionsEN[key]; ok {
		return d
	}
	for _, item := range mergedCatalog() {
		if item.Key == key {
			if d, ok := catalogDescriptionsEN[key]; ok {
				return d
			}
			break
		}
	}
	return ""
}
