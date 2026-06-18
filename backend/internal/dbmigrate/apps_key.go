package dbmigrate

import (
	"log"

	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

func appsHasLegacyKeyColumn(db *gorm.DB) bool {
	if db == nil || !db.Migrator().HasTable(&models.App{}) {
		return false
	}
	var count int64
	if err := db.Raw(`SELECT COUNT(*) FROM pragma_table_info('apps') WHERE name = 'key'`).Scan(&count).Error; err != nil {
		return false
	}
	return count > 0
}

// AppsHasLegacyKeyColumn reports whether apps still has the pre-v0.1.6 `key` column.
func AppsHasLegacyKeyColumn(db *gorm.DB) bool {
	return appsHasLegacyKeyColumn(db)
}

// MigrateAppsKeyColumn copies legacy `key` values into `app_key`, removes orphan rows,
// and drops the legacy column so catalog seed inserts are not blocked by NOT NULL on `key`.
func MigrateAppsKeyColumn(db *gorm.DB) {
	if !appsHasLegacyKeyColumn(db) {
		return
	}
	res := db.Exec(`UPDATE apps SET app_key = "key" WHERE (app_key IS NULL OR app_key = '') AND "key" IS NOT NULL AND "key" != ''`)
	if res.Error != nil {
		log.Printf("[dbmigrate] migrate apps.key -> app_key: %v", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		log.Printf("[dbmigrate] migrated %d app rows from legacy key column to app_key", res.RowsAffected)
	}
	if err := db.Exec(`DELETE FROM apps WHERE app_key IS NULL OR app_key = ''`).Error; err != nil {
		log.Printf("[dbmigrate] delete empty app_key rows: %v", err)
		return
	}
	if err := db.Exec(`ALTER TABLE apps DROP COLUMN "key"`).Error; err != nil {
		log.Printf("[dbmigrate] drop apps.key column: %v", err)
		return
	}
	log.Printf("[dbmigrate] dropped legacy apps.key column")
}
