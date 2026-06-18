package dbmigrate

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMigrateAppsKeyColumnDropsLegacyKey(t *testing.T) {
	dir := t.TempDir()
	dsn := filepath.Join(dir, "legacy.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	if err := db.Exec(`CREATE TABLE apps (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME,
		"key" TEXT NOT NULL,
		app_key TEXT DEFAULT '',
		name TEXT
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO apps ("key", name) VALUES ('nginx', 'Nginx')`).Error; err != nil {
		t.Fatal(err)
	}

	MigrateAppsKeyColumn(db)
	if AppsHasLegacyKeyColumn(db) {
		t.Fatal("legacy key column still present after migration")
	}

	var appKey string
	if err := db.Raw(`SELECT app_key FROM apps WHERE name = 'Nginx'`).Scan(&appKey).Error; err != nil {
		t.Fatal(err)
	}
	if appKey != "nginx" {
		t.Fatalf("app_key = %q, want nginx", appKey)
	}

	app := struct {
		Key  string `gorm:"column:app_key"`
		Name string
	}{Key: "redis", Name: "Redis"}
	if err := db.Table("apps").Create(&app).Error; err != nil {
		t.Fatalf("insert after migration failed: %v", err)
	}
}
