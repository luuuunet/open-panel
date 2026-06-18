package appstore

import (
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

var mysqlVersionDefs = []struct {
	Key     string
	Version string
	Port    int
}{
	{Key: "mysql84", Version: "8.4", Port: 3306},
	{Key: "mysql80", Version: "8.0", Port: 3307},
	{Key: "mysql57", Version: "5.7", Port: 3308},
	{Key: "mysql56", Version: "5.6", Port: 3309},
	{Key: "mysql55", Version: "5.5", Port: 3310},
}

func appendMySQLVersionCatalog(out []catalogItem) []catalogItem {
	seen := make(map[string]struct{}, len(out))
	for _, item := range out {
		seen[item.Key] = struct{}{}
	}
	for _, def := range mysqlVersionDefs {
		if _, ok := seen[def.Key]; ok {
			continue
		}
		verPath := strings.ReplaceAll(def.Version, ".", "")
		out = append(out, catalogItem{
			App: models.App{
				Key: def.Key, Name: fmt.Sprintf("MySQL %s", def.Version), Category: "数据库",
				Versions: def.Version, Version: def.Version,
				Description: fmt.Sprintf("MySQL %s 关系型数据库", def.Version),
				Port: def.Port, InstallPath: fmt.Sprintf("server/mysql/%s", verPath),
				ConfigPath: fmt.Sprintf("server/mysql/%s/my.cnf", verPath), Icon: "Coin",
			},
			defaultConfig: map[string]interface{}{
				"max_connections": 500, "innodb_buffer_pool_size": "256M",
				"bind_address": "127.0.0.1", "port": def.Port,
			},
		})
		seen[def.Key] = struct{}{}
	}
	return out
}

// MySQLVersionFromKey resolves catalog/install version from a store key (mysql80 → 8.0).
func MySQLVersionFromKey(key string) (string, bool) {
	if key == "mysql" {
		return "8.0", true
	}
	for _, def := range mysqlVersionDefs {
		if def.Key == key {
			return def.Version, true
		}
	}
	if strings.HasPrefix(key, "mysql") && len(key) > 5 {
		suffix := strings.TrimPrefix(key, "mysql")
		if len(suffix) == 2 {
			return suffix[:1] + "." + suffix[1:], true
		}
	}
	return "", false
}

// MySQLKeyFromVersion returns the store key for a MySQL version string.
func MySQLKeyFromVersion(version string) string {
	for _, def := range mysqlVersionDefs {
		if def.Version == version {
			return def.Key
		}
	}
	suffix := strings.ReplaceAll(version, ".", "")
	if suffix != "" {
		return "mysql" + suffix
	}
	return "mysql"
}
