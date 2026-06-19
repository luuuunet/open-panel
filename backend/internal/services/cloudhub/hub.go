package cloudhub

import (
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/autops"
	"github.com/luuuunet/owpanel/internal/services/backup"
	"github.com/luuuunet/owpanel/internal/services/ossstorage"
	"github.com/luuuunet/owpanel/internal/services/uptime"
	"gorm.io/gorm"
)

type Service struct {
	db      *gorm.DB
	dataDir string
	autops  *autops.Service
	backup  *backup.Service
	uptime  *uptime.Service
	oss     *ossstorage.Service
}

func NewService(db *gorm.DB, dataDir string, autopsSvc *autops.Service, backupSvc *backup.Service, uptimeSvc *uptime.Service, ossSvc *ossstorage.Service) *Service {
	return &Service{
		db:      db,
		dataDir: dataDir,
		autops:  autopsSvc,
		backup:  backupSvc,
		uptime:  uptimeSvc,
		oss:     ossSvc,
	}
}

type FeatureStatus struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Route       string `json:"route"`
	Configured  bool   `json:"configured"`
	Count       int    `json:"count"`
	Description string `json:"description,omitempty"`
}

type StorageBrief struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Bucket   string `json:"bucket"`
}

type VendorHub struct {
	Key         string          `json:"key"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	OSSCount    int             `json:"oss_count"`
	DNSReady    bool            `json:"dns_ready"`
	MailReady   bool            `json:"mail_ready"`
	Storages    []StorageBrief  `json:"storages"`
	Features    []FeatureStatus `json:"features"`
}

type HubSummary struct {
	OSSStorages    int  `json:"oss_storages"`
	DNSProviders   int  `json:"dns_providers"`
	BackupTasks    int  `json:"backup_tasks"`
	BackupWithOSS  int  `json:"backup_with_oss"`
	UptimeMonitors int  `json:"uptime_monitors"`
	AutoOpsEnabled bool `json:"autops_enabled"`
	OSSSyncTasks   int  `json:"oss_sync_tasks"`
	ClusterNodes   int  `json:"cluster_nodes"`
}

type HubResponse struct {
	Summary      HubSummary      `json:"summary"`
	Integrations []FeatureStatus `json:"integrations"`
	Vendors      []VendorHub     `json:"vendors"`
}

type CloudPresetRequest struct {
	OSSStorageID *uint `json:"oss_storage_id"`
	IncludeOps   bool  `json:"include_ops"`
	LinkBackup   bool  `json:"link_backup"`
	CreateSync   bool  `json:"create_sync"`
}

type CloudPresetResult struct {
	Vendor          string                   `json:"vendor"`
	Autops          *autops.SitePresetResult `json:"autops,omitempty"`
	Uptime          any                      `json:"uptime,omitempty"`
	BackupWebsites  *backup.PresetResult     `json:"backup_websites,omitempty"`
	BackupDatabases *backup.PresetResult     `json:"backup_databases,omitempty"`
	OpsApplied      bool                     `json:"ops_applied"`
	BackupLinked    int                      `json:"backup_linked"`
	OSSSyncCreated  bool                     `json:"oss_sync_created"`
	Todos           []string                 `json:"todos"`
}

type vendorSpec struct {
	key          string
	name         string
	description  string
	ossProviders []string
	dnsProviders []string
	mailTypes    []string
	todos        []string
}

var vendorSpecs = []vendorSpec{
	{
		key: "aliyun", name: "阿里云", description: "轻量/ECS + OSS + 云解析 DNS + 邮件推送",
		ossProviders: []string{"aliyun"}, dnsProviders: []string{"alidns"}, mailTypes: []string{"aliyun_dm"},
		todos: []string{"在 OSS 页添加阿里云 Bucket 凭证", "在 DNS 页接入阿里云 AccessKey", "可选：邮件页配置阿里云 DirectMail"},
	},
	{
		key: "tencent", name: "腾讯云", description: "CVM + COS + DNSPod",
		ossProviders: []string{"tencent"}, dnsProviders: []string{"dnspod"},
		todos: []string{"在 OSS 页添加腾讯云 COS", "在 DNS 页接入 DNSPod Token"},
	},
	{
		key: "aws", name: "AWS", description: "Lightsail/EC2 + S3 + SES",
		ossProviders: []string{"aws"}, mailTypes: []string{"amazon_ses"},
		todos: []string{"在 OSS 页添加 S3 Bucket", "可选：邮件页配置 Amazon SES"},
	},
	{
		key: "gcp", name: "Google Cloud", description: "Compute + Cloud Storage",
		ossProviders: []string{"google"},
		todos: []string{"在 OSS 页添加 GCS 存储", "建议在 Google Search Console 验证站点"},
	},
	{
		key: "ibm", name: "IBM Cloud", description: "VSI + Cloud Object Storage",
		ossProviders: []string{"ibm"},
		todos: []string{"在 OSS 页添加 IBM COS 端点"},
	},
}

func (s *Service) GetHub() (*HubResponse, error) {
	var ossList []models.OSSStorage
	_ = s.db.Where("enabled = ?", true).Find(&ossList).Error
	var dnsList []models.DNSProviderAccount
	_ = s.db.Where("enabled = ?", true).Find(&dnsList).Error
	var mailList []models.MailSendProvider
	_ = s.db.Where("enabled = ?", true).Find(&mailList).Error
	var backupTasks int64
	_ = s.db.Model(&models.BackupTask{}).Count(&backupTasks).Error
	var backupOSS int64
	_ = s.db.Model(&models.BackupTask{}).Where("oss_storage_id IS NOT NULL AND oss_storage_id > 0").Count(&backupOSS).Error
	var uptimeCount int64
	_ = s.db.Model(&models.UptimeMonitor{}).Count(&uptimeCount).Error
	var syncCount int64
	_ = s.db.Model(&models.OSSSyncTask{}).Count(&syncCount).Error
	var clusterNodes int64
	_ = s.db.Model(&models.ClusterNode{}).Count(&clusterNodes).Error

	autopsEnabled := false
	if st, err := s.autops.GetStatus(); err == nil && st.Config.Enabled {
		autopsEnabled = true
	}

	summary := HubSummary{
		OSSStorages:    len(ossList),
		DNSProviders:   len(dnsList),
		BackupTasks:    int(backupTasks),
		BackupWithOSS:  int(backupOSS),
		UptimeMonitors: int(uptimeCount),
		AutoOpsEnabled: autopsEnabled,
		OSSSyncTasks:   int(syncCount),
		ClusterNodes:   int(clusterNodes),
	}

	resp := &HubResponse{
		Summary:      summary,
		Integrations: buildIntegrations(len(ossList), len(dnsList), len(mailList), int(backupTasks), int(uptimeCount), autopsEnabled, int(syncCount), int(clusterNodes)),
	}
	for _, spec := range vendorSpecs {
		resp.Vendors = append(resp.Vendors, buildVendorHub(spec, summary, ossList, dnsList, mailList))
	}
	return resp, nil
}

func buildIntegrations(ossN, dnsN, mailN, backups, uptime int, autopsOn bool, syncTasks, clusterNodes int) []FeatureStatus {
	return []FeatureStatus{
		{Key: "oss", Label: "oss", Route: "/oss", Configured: ossN > 0, Count: ossN, Description: "oss"},
		{Key: "dns", Label: "dns", Route: "/dns", Configured: dnsN > 0, Count: dnsN, Description: "dns"},
		{Key: "backup", Label: "backup", Route: "/backup", Configured: backups > 0, Count: backups, Description: "backup"},
		{Key: "uptime", Label: "uptime", Route: "/uptime", Configured: uptime > 0, Count: uptime, Description: "uptime"},
		{Key: "autops", Label: "autops", Route: "/auto-ops", Configured: autopsOn, Count: boolCount(autopsOn), Description: "autops"},
		{Key: "sync", Label: "sync", Route: "/oss", Configured: syncTasks > 0, Count: syncTasks, Description: "sync"},
		{Key: "mail", Label: "mail", Route: "/mail", Configured: mailN > 0, Count: mailN, Description: "mail"},
		{Key: "cluster", Label: "cluster", Route: "/cluster", Configured: clusterNodes > 0, Count: clusterNodes, Description: "cluster"},
	}
}

func buildVendorHub(spec vendorSpec, summary HubSummary, ossList []models.OSSStorage, dnsList []models.DNSProviderAccount, mailList []models.MailSendProvider) VendorHub {
	hub := VendorHub{
		Key:         spec.key,
		Name:        spec.name,
		Description: spec.description,
	}
	for _, o := range ossList {
		if matchProvider(o.Provider, spec.ossProviders) {
			hub.OSSCount++
			hub.Storages = append(hub.Storages, StorageBrief{ID: o.ID, Name: o.Name, Provider: o.Provider, Bucket: o.Bucket})
		}
	}
	for _, d := range dnsList {
		if matchProvider(d.Provider, spec.dnsProviders) {
			hub.DNSReady = true
		}
	}
	for _, m := range mailList {
		if matchProvider(m.ProviderType, spec.mailTypes) {
			hub.MailReady = true
		}
	}
	hub.Features = []FeatureStatus{
		{Key: "oss", Label: "oss", Route: "/oss", Configured: hub.OSSCount > 0, Count: hub.OSSCount},
		{Key: "dns", Label: "dns", Route: "/dns", Configured: hub.DNSReady, Count: boolCount(hub.DNSReady)},
		{Key: "backup", Label: "backup", Route: "/backup", Configured: summary.BackupWithOSS > 0, Count: summary.BackupWithOSS},
		{Key: "monitor", Label: "autops", Route: "/auto-ops", Configured: summary.AutoOpsEnabled, Count: boolCount(summary.AutoOpsEnabled)},
		{Key: "uptime", Label: "uptime", Route: "/uptime", Configured: summary.UptimeMonitors > 0, Count: summary.UptimeMonitors},
	}
	return hub
}

func boolCount(v bool) int {
	if v {
		return 1
	}
	return 0
}

func matchProvider(provider string, allowed []string) bool {
	if len(allowed) == 0 {
		return false
	}
	p := strings.ToLower(strings.TrimSpace(provider))
	for _, a := range allowed {
		if p == strings.ToLower(a) {
			return true
		}
	}
	return false
}

func vendorByKey(key string) (vendorSpec, bool) {
	k := strings.ToLower(strings.TrimSpace(key))
	for _, v := range vendorSpecs {
		if v.key == k {
			return v, true
		}
	}
	return vendorSpec{}, false
}

func (s *Service) ApplyCloudPreset(vendorKey string, req *CloudPresetRequest) (*CloudPresetResult, error) {
	spec, ok := vendorByKey(vendorKey)
	if !ok {
		return nil, fmt.Errorf("unknown cloud vendor: %s", vendorKey)
	}
	if req == nil {
		req = &CloudPresetRequest{IncludeOps: true, LinkBackup: true, CreateSync: true}
	}

	out := &CloudPresetResult{Vendor: spec.key, Todos: append([]string{}, spec.todos...)}

	autopsRes, err := s.autops.ApplySitePreset()
	if err != nil {
		return nil, err
	}
	out.Autops = autopsRes

	uptimeRes, err := s.uptime.ImportFromWebsites(300, nil)
	if err != nil {
		return nil, err
	}
	out.Uptime = uptimeRes

	webBackup, err := s.backup.ApplyPreset("websites", "0 2 * * *", nil, req.OSSStorageID)
	if err != nil {
		return nil, err
	}
	out.BackupWebsites = webBackup

	dbBackup, err := s.backup.ApplyPreset("databases", "0 3 * * *", nil, req.OSSStorageID)
	if err != nil {
		return nil, err
	}
	out.BackupDatabases = dbBackup

	if req.IncludeOps {
		if err := s.autops.ApplyOpsPreset(); err != nil {
			return nil, err
		}
		out.OpsApplied = true
	}

	storageID := req.OSSStorageID
	if storageID == nil && req.LinkBackup {
		storageID = s.firstVendorStorage(spec.ossProviders)
	}

	if storageID != nil && *storageID > 0 && req.LinkBackup {
		linked, err := s.backup.LinkOSSToTasks(*storageID)
		if err != nil {
			return nil, err
		}
		out.BackupLinked = linked
	}

	if storageID != nil && *storageID > 0 && req.CreateSync {
		created, err := s.ensureBackupSyncTask(*storageID, spec.name)
		if err != nil {
			return nil, err
		}
		out.OSSSyncCreated = created
	}

	if storageID == nil || *storageID == 0 {
		out.Todos = append([]string{fmt.Sprintf("在 OSS 页添加 %s 存储后，再次点击一键迁移包并选择存储", spec.name)}, out.Todos...)
	}
	if specHasDNS(spec) && !s.vendorDNSReady(spec) {
		out.Todos = append(out.Todos, "在 DNS 页接入对应云解析服务商")
	}
	out.Todos = append(out.Todos, "在自动化运维 → 策略设置 填写 Webhook 接收告警")

	return out, nil
}

func specHasDNS(spec vendorSpec) bool {
	return len(spec.dnsProviders) > 0
}

func (s *Service) vendorDNSReady(spec vendorSpec) bool {
	var dnsList []models.DNSProviderAccount
	if s.db.Where("enabled = ?", true).Find(&dnsList).Error != nil {
		return false
	}
	for _, d := range dnsList {
		if matchProvider(d.Provider, spec.dnsProviders) {
			return true
		}
	}
	return false
}

func (s *Service) firstVendorStorage(providers []string) *uint {
	var list []models.OSSStorage
	if s.db.Where("enabled = ?", true).Order("id asc").Find(&list).Error != nil {
		return nil
	}
	for _, st := range list {
		if matchProvider(st.Provider, providers) {
			id := st.ID
			return &id
		}
	}
	return nil
}

func (s *Service) ensureBackupSyncTask(storageID uint, vendorName string) (bool, error) {
	name := fmt.Sprintf("云备份同步-%s", vendorName)
	var count int64
	s.db.Model(&models.OSSSyncTask{}).Where("name = ?", name).Count(&count)
	if count > 0 {
		return false, nil
	}
	var st models.OSSStorage
	if s.db.First(&st, storageID).Error != nil {
		return false, fmt.Errorf("oss storage not found")
	}
	_, err := s.oss.CreateSyncTask(&ossstorage.SyncTaskRequest{
		Name:            name,
		Mode:            "upload",
		TargetStorageID: &storageID,
		LocalPath:       "wwwroot",
		TargetPath:      "owpanel-backups/wwwroot",
		Schedule:        "0 4 * * *",
		Enabled:         true,
	})
	return err == nil, err
}
