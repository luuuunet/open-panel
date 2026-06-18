package appstore

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

func closeDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	if err != nil {
		return
	}
	if err := sqlDB.Close(); err != nil {
		t.Logf("close db: %v", err)
	}
}

func openTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := filepath.Join(dir, name)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeDB(t, db) })
	return db
}

func TestMergedCatalogNotEmpty(t *testing.T) {
	items := mergedCatalog()
	if len(items) < 50 {
		t.Fatalf("mergedCatalog() returned %d items, want >= 50", len(items))
	}
}

func TestListSeedsAndReturnsApps(t *testing.T) {
	db := openTestDB(t, "test.db")
	if err := db.AutoMigrate(&models.App{}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	svc := NewService(db, dir)
	apps, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) == 0 {
		t.Fatal("List() returned empty after NewService seed")
	}
	t.Logf("List() returned %d apps", len(apps))
}

func TestListAfterPurgeRemovedStoreApps(t *testing.T) {
	db := openTestDB(t, "test.db")
	if err := db.AutoMigrate(&models.App{}); err != nil {
		t.Fatal(err)
	}

	stale := models.App{Key: "external-legacy-app", Name: "Legacy", Category: "工具", Version: "1.0", Versions: "1.0"}
	if err := db.Create(&stale).Error; err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	svc := NewService(db, dir)
	apps, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) == 0 {
		t.Fatal("List() returned empty")
	}
	for _, app := range apps {
		if app.Key == "external-legacy-app" {
			t.Fatal("stale app should have been purged")
		}
	}
}

func TestFilterCatalogApps(t *testing.T) {
	allowed := catalogOnlyKeys()
	if len(allowed) == 0 {
		t.Fatal("catalogOnlyKeys empty")
	}
	apps := []models.App{
		{Key: "nginx", Name: "Nginx"},
		{Key: "unknown-app", Name: "Unknown"},
	}
	filtered := filterCatalogApps(apps)
	if len(filtered) != 1 || filtered[0].Key != "nginx" {
		t.Fatalf("filterCatalogApps = %+v", filtered)
	}
}

func TestListRecoversFromLegacyKeyColumn(t *testing.T) {
	dir := t.TempDir()
	dsn := filepath.Join(dir, "legacy.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeDB(t, db) })

	if err := db.Exec(`CREATE TABLE apps (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME,
		"key" TEXT NOT NULL,
		app_key TEXT DEFAULT '',
		name TEXT,
		category TEXT,
		version TEXT,
		versions TEXT,
		description TEXT,
		installed INTEGER DEFAULT 0,
		status TEXT DEFAULT 'stopped',
		install_error TEXT,
		port INTEGER DEFAULT 0,
		install_path TEXT,
		config_path TEXT,
		config TEXT,
		auto_start INTEGER DEFAULT 1,
		watch_enabled INTEGER DEFAULT 0,
		auto_restart INTEGER DEFAULT 0,
		icon TEXT,
		icon_url TEXT,
		bind_domain TEXT,
		meta TEXT
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO apps ("key", name, category, version, versions, description, port, install_path, config_path, icon)
		VALUES ('nginx', 'Nginx', 'Web服务器', '1.26', '1.26', 'test', 80, 'server/nginx', '/etc/nginx/nginx.conf', 'SetUp')`).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewService(db, dir)
	apps, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) < 50 {
		t.Fatalf("List() returned %d apps after legacy migration, want full catalog", len(apps))
	}
	foundNginx := false
	for _, app := range apps {
		if app.Key == "nginx" {
			foundNginx = true
		}
	}
	if !foundNginx {
		t.Fatal("nginx not in store list after legacy recovery")
	}
}
