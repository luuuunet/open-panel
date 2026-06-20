package database

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"errors"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/dbmigrate"
	"github.com/luuuunet/owpanel/internal/services/cilium"
	"github.com/luuuunet/owpanel/internal/services/kafkaaccel"
	"github.com/luuuunet/owpanel/internal/secrets"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func Init(dataDir string) (*gorm.DB, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "panel.db")
	dsn := dbPath + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := db.AutoMigrate(
		&models.User{},
		&models.Website{},
		&models.WebsiteSubdir{},
		&models.WebsiteAlias{},
		&models.SiteCategory{},
		&models.DatabaseInstance{},
		&models.DatabaseBackup{},
		&models.CronJob{},
		&models.FirewallRule{},
		&models.SSLCertificate{},
		&models.App{},
		&models.FTPAccount{},
		&models.BackupTask{},
		&models.WebsiteBackup{},
		&models.BackupRemote{},
		&models.SSHKey{},
		&models.PanelSetting{},
		&models.ComposeApp{},
		&models.DockerContainerBinding{},
		&models.WAFRule{},
		&models.SecurityConfig{},
		&models.CacheConfig{},
		&models.CacheRule{},
		&models.IPBlacklist{},
		&models.IPWhitelist{},
		&models.MailDomain{},
		&models.MailBox{},
		&models.MailBackup{},
		&models.MailSendProvider{},
		&models.MailBulkCampaign{},
		&models.MailBulkRecipient{},
		&models.DNSRecord{},
		&models.DNSProviderAccount{},
		&models.DNSZone{},
		&models.WordPressSite{},
		&models.WordPressBackup{},
		&models.WordPressDomain{},
		&models.NodeProject{},
		&models.JavaProject{},
		&models.RuntimeProject{},
		&models.TrafficHit{},
		&models.WebsiteGeoPolicy{},
		&models.BotCrawlerRule{},
		&models.MetricSnapshot{},
		&models.AutoOpsEvent{},
		&models.ClusterNode{},
		&models.LoadBalancer{},
		&models.LoadBalancerBackend{},
		&models.ClusterWorkflow{},
		&models.OSSStorage{},
		&models.OSSSyncTask{},
		&models.PanelBackupRecord{},
		&models.PanelUpdateRecord{},
		&models.LocalCleanupRule{},
		&models.OSSLifecycleRule{},
		&models.OSSArchiveRule{},
		&models.UptimeMonitor{},
		&models.CacheSnapshot{},
		&models.SiteDeployConfig{},
		&models.SiteDeployJob{},
		&models.AISiteBootstrapJob{},
		&models.EdgeWorker{},
		&models.EdgeKVNamespace{},
		&models.EdgeKVEntry{},
		&models.EdgeD1Database{},
		&models.EdgeWorkerBinding{},
		&kafkaaccel.KafkaAccelConfig{},
		&kafkaaccel.KafkaAccelRule{},
		&cilium.CiliumConfig{},
		&models.CommandSnippet{},
		&models.LoginEvent{},
		&models.PanelAuditEvent{},
		&models.BastionAssetGroup{},
		&models.BastionAsset{},
		&models.BastionAccount{},
		&models.BastionAccountRotationLog{},
		&models.BastionAccessRequest{},
		&models.BastionKnownHost{},
		&models.BastionPermission{},
		&models.BastionSession{},
		&models.BastionCommandAudit{},
		&models.OpsTemplate{},
		&models.OpsJob{},
		&models.OpsJobRun{},
		&models.OpsJobResult{},
	); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	dbmigrate.MigrateAppsKeyColumn(db)

	if err := seedAdmin(db, dataDir); err != nil {
		return nil, err
	}
	migrateAdminSecurity(db)

	TuneSQLite(db)
	EnsureTrafficIndexes(db)
	StartPeriodicMaintenance(db, time.Hour)

	return db, nil
}

func seedAdmin(db *gorm.DB, dataDir string) error {
	var count int64
	db.Model(&models.User{}).Count(&count)
	if count > 0 {
		return nil
	}

	pass, err := secrets.GeneratePassword(16)
	if err != nil {
		return fmt.Errorf("generate admin password: %w", err)
	}

	admin := &models.User{
		Username:           "admin",
		Role:               "admin",
		MustChangePassword: true,
	}
	if err := admin.SetPassword(pass); err != nil {
		return fmt.Errorf("seed admin password: %w", err)
	}
	if err := db.Create(admin).Error; err != nil {
		return err
	}

	credPath, err := secrets.WriteInitialAdminCredentials(dataDir, admin.Username, pass)
	if err != nil {
		log.Printf("warning: could not write initial credentials file: %v", err)
	} else {
		log.Printf("OWPanel: initial admin credentials saved to %s", credPath)
	}
	log.Printf("OWPanel: first login — username: admin  password: %s  (change after login)", pass)
	return nil
}

func migrateAdminSecurity(db *gorm.DB) {
	var admin models.User
	if db.Where("username = ?", "admin").First(&admin).Error != nil {
		return
	}
	if admin.CheckPassword("admin") {
		db.Model(&admin).Update("must_change_password", true)
	}
}

// ResetAdminPassword sets the admin user's password (creates admin if missing).
func ResetAdminPassword(db *gorm.DB, username, password string) error {
	if username == "" {
		username = "admin"
	}
	var user models.User
	err := db.Where("username = ?", username).First(&user).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		user = models.User{Username: username, Role: "admin"}
		if err := user.SetPassword(password); err != nil {
			return err
		}
		return db.Create(&user).Error
	}
	if err := user.SetPassword(password); err != nil {
		return err
	}
	user.MustChangePassword = false
	return db.Save(&user).Error
}
