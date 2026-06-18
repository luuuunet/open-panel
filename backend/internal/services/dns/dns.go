package dns

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"gorm.io/gorm"
)

type Service struct {
	db       *gorm.DB
	settings *settings.Service
}

func NewService(db *gorm.DB, settingsSvc *settings.Service) *Service {
	return &Service{db: db, settings: settingsSvc}
}

type ProviderAccountDTO struct {
	models.DNSProviderAccount
	HasToken  bool   `json:"has_token"`
	HasSecret bool   `json:"has_secret"`
	ProviderName string `json:"provider_name"`
}

func (s *Service) List(domain string) ([]models.DNSRecord, error) {
	var list []models.DNSRecord
	q := s.db.Order("id desc")
	if domain != "" {
		q = q.Where("domain = ?", domain)
	}
	return list, q.Find(&list).Error
}

func (s *Service) Get(id uint) (*models.DNSRecord, error) {
	var r models.DNSRecord
	if err := s.db.First(&r, id).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Service) Create(r *models.DNSRecord) error {
	return s.db.Create(r).Error
}

func (s *Service) Update(r *models.DNSRecord) error {
	return s.db.Save(r).Error
}

func (s *Service) Delete(id uint) error {
	rec, err := s.Get(id)
	if err != nil {
		return err
	}
	if rec.ProviderID > 0 && rec.ExternalID != "" {
		if err := s.deleteRemoteRecord(rec); err != nil {
			return err
		}
	}
	return s.db.Delete(&models.DNSRecord{}, id).Error
}

// DeleteByWebsiteID removes DNS records linked to a website (best-effort remote cleanup).
func (s *Service) DeleteByWebsiteID(websiteID uint) error {
	var recs []models.DNSRecord
	if err := s.db.Where("website_id = ?", websiteID).Find(&recs).Error; err != nil {
		return err
	}
	for _, rec := range recs {
		_ = s.Delete(rec.ID)
	}
	return nil
}

func (s *Service) ListProviders() ([]ProviderAccountDTO, error) {
	var list []models.DNSProviderAccount
	if err := s.db.Order("is_default desc, id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	out := make([]ProviderAccountDTO, len(list))
	for i, a := range list {
		out[i] = ProviderAccountDTO{
			DNSProviderAccount: a,
			HasToken:           a.APIToken != "",
			HasSecret:          a.SecretKey != "",
			ProviderName:       providerDisplayName(a.Provider),
		}
		out[i].APIToken = ""
		out[i].SecretKey = ""
	}
	return out, nil
}

func providerDisplayName(key string) string {
	for _, p := range SupportedProviders {
		if p["key"] == key {
			return p["name"]
		}
	}
	return key
}

type CreateProviderRequest struct {
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	APIToken  string `json:"api_token"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Extra     string `json:"extra"`
	IsDefault bool   `json:"is_default"`
}

func (s *Service) CreateProvider(req CreateProviderRequest) (*models.DNSProviderAccount, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name required")
	}
	if _, err := providerClient(req.Provider); err != nil {
		return nil, err
	}
	acc := &models.DNSProviderAccount{
		Name: req.Name, Provider: req.Provider,
		APIToken: req.APIToken, AccessKey: req.AccessKey, SecretKey: req.SecretKey,
		Extra: req.Extra, Enabled: true, IsDefault: req.IsDefault,
	}
	if req.IsDefault {
		s.db.Model(&models.DNSProviderAccount{}).Where("1=1").Update("is_default", false)
	}
	if err := s.db.Create(acc).Error; err != nil {
		return nil, err
	}
	return acc, nil
}

type UpdateProviderRequest struct {
	Name      string `json:"name"`
	APIToken  string `json:"api_token"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Extra     string `json:"extra"`
	Enabled   *bool  `json:"enabled"`
	IsDefault *bool  `json:"is_default"`
}

func (s *Service) UpdateProvider(id uint, req UpdateProviderRequest) error {
	var acc models.DNSProviderAccount
	if err := s.db.First(&acc, id).Error; err != nil {
		return err
	}
	if req.Name != "" {
		acc.Name = req.Name
	}
	if req.APIToken != "" {
		acc.APIToken = req.APIToken
	}
	if req.AccessKey != "" {
		acc.AccessKey = req.AccessKey
	}
	if req.SecretKey != "" {
		acc.SecretKey = req.SecretKey
	}
	if req.Extra != "" {
		acc.Extra = req.Extra
	}
	if req.Enabled != nil {
		acc.Enabled = *req.Enabled
	}
	if req.IsDefault != nil && *req.IsDefault {
		s.db.Model(&models.DNSProviderAccount{}).Where("id != ?", id).Update("is_default", false)
		acc.IsDefault = true
	}
	return s.db.Save(&acc).Error
}

func (s *Service) DeleteProvider(id uint) error {
	return s.db.Delete(&models.DNSProviderAccount{}, id).Error
}

func (s *Service) getAccount(id uint) (*models.DNSProviderAccount, error) {
	var acc models.DNSProviderAccount
	if err := s.db.First(&acc, id).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

func (s *Service) defaultAccount() (*models.DNSProviderAccount, error) {
	var acc models.DNSProviderAccount
	err := s.db.Where("enabled = ? AND is_default = ?", true, true).First(&acc).Error
	if err == nil {
		return &acc, nil
	}
	err = s.db.Where("enabled = ?", true).Order("id asc").First(&acc).Error
	if err != nil {
		return nil, fmt.Errorf("no dns provider configured")
	}
	return &acc, nil
}

func (s *Service) TestProvider(id uint) error {
	acc, err := s.getAccount(id)
	if err != nil {
		return err
	}
	client, err := providerClient(acc.Provider)
	if err != nil {
		return err
	}
	return client.Test(context.Background(), acc)
}

func (s *Service) SyncProviderZones(id uint) (int, error) {
	acc, err := s.getAccount(id)
	if err != nil {
		return 0, err
	}
	client, err := providerClient(acc.Provider)
	if err != nil {
		return 0, err
	}
	zones, err := client.ListZones(context.Background(), acc)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, z := range zones {
		ns, _ := json.Marshal(z.NameServers)
		row := models.DNSZone{
			ProviderID: acc.ID, ZoneID: z.ID, Name: z.Name,
			Status: z.Status, NameServers: string(ns),
		}
		var existing models.DNSZone
		err := s.db.Where("provider_id = ? AND zone_id = ?", acc.ID, z.ID).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			if err := s.db.Create(&row).Error; err != nil {
				return count, err
			}
		} else if err == nil {
			existing.Name = z.Name
			existing.Status = z.Status
			existing.NameServers = string(ns)
			s.db.Save(&existing)
		}
		count++
	}
	return count, nil
}

func (s *Service) ListZones(providerID uint) ([]models.DNSZone, error) {
	var list []models.DNSZone
	q := s.db.Order("name asc")
	if providerID > 0 {
		q = q.Where("provider_id = ?", providerID)
	}
	return list, q.Find(&list).Error
}

func (s *Service) PullZoneRecords(providerID uint, zoneName string) (int, error) {
	acc, err := s.getAccount(providerID)
	if err != nil {
		return 0, err
	}
	var zone models.DNSZone
	if err := s.db.Where("provider_id = ? AND name = ?", providerID, zoneName).First(&zone).Error; err != nil {
		return 0, fmt.Errorf("zone not found, sync zones first")
	}
	client, err := providerClient(acc.Provider)
	if err != nil {
		return 0, err
	}
	zoneKey := zone.ZoneID
	if acc.Provider == "alidns" {
		zoneKey = zone.Name
	}
	remote, err := client.ListRecords(context.Background(), acc, zoneKey)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, r := range remote {
		name := r.Name
		if strings.HasSuffix(name, "."+zone.Name) {
			name = strings.TrimSuffix(name, "."+zone.Name)
		}
		if name == zone.Name {
			name = "@"
		}
		var local models.DNSRecord
		err := s.db.Where("provider_id = ? AND external_id = ?", acc.ID, r.ID).First(&local).Error
		rec := models.DNSRecord{
			Domain: zone.Name, Type: r.Type, Name: name, Value: r.Content,
			TTL: r.TTL, ProviderID: acc.ID, ZoneID: zone.ZoneID,
			ExternalID: r.ID, Proxied: r.Proxied, SyncStatus: "synced",
		}
		if err == gorm.ErrRecordNotFound {
			if err := s.db.Create(&rec).Error; err != nil {
				return count, err
			}
		} else if err == nil {
			local.Type = rec.Type
			local.Name = rec.Name
			local.Value = rec.Value
			local.TTL = rec.TTL
			local.Proxied = rec.Proxied
			local.SyncStatus = "synced"
			local.SyncError = ""
			s.db.Save(&local)
		}
		count++
	}
	return count, nil
}

func (s *Service) ServerIP() string {
	if s.settings != nil {
		all, _ := s.settings.GetAll()
		if ip := strings.TrimSpace(all["server_public_ip"]); ip != "" {
			return ip
		}
	}
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			return ip.String()
		}
	}
	resp, err := http.Get("https://api.ipify.org?format=text")
	if err == nil {
		defer resp.Body.Close()
		buf := make([]byte, 64)
		n, _ := resp.Body.Read(buf)
		if ip := strings.TrimSpace(string(buf[:n])); ip != "" {
			return ip
		}
	}
	return "127.0.0.1"
}

type DetectItem struct {
	Host         string `json:"host"`
	Source       string `json:"source"`
	ZoneFound    bool   `json:"zone_found"`
	ZoneName     string `json:"zone_name,omitempty"`
	Provider     string `json:"provider,omitempty"`
	ProviderID   uint   `json:"provider_id,omitempty"`
	CurrentValue string `json:"current_value,omitempty"`
	ExpectedIP   string `json:"expected_ip"`
	NeedsFix     bool   `json:"needs_fix"`
	RecordType   string `json:"record_type,omitempty"`
}

func (s *Service) DetectDomains() ([]DetectItem, error) {
	expected := s.ServerIP()
	var zones []models.DNSZone
	s.db.Find(&zones)
	providerNames := map[uint]string{}
	var providers []models.DNSProviderAccount
	s.db.Find(&providers)
	for _, p := range providers {
		providerNames[p.ID] = p.Provider
	}

	type hostSrc struct{ host, source string }
	var hosts []hostSrc

	var websites []models.Website
	s.db.Find(&websites)
	for _, w := range websites {
		hosts = append(hosts, hostSrc{w.Domain, "website"})
	}
	var aliases []models.WebsiteAlias
	s.db.Find(&aliases)
	for _, a := range aliases {
		hosts = append(hosts, hostSrc{a.Domain, "alias"})
	}
	var wpDomains []models.WordPressDomain
	s.db.Find(&wpDomains)
	for _, d := range wpDomains {
		hosts = append(hosts, hostSrc{d.Domain, "wordpress"})
	}
	var mailDomains []models.MailDomain
	s.db.Find(&mailDomains)
	for _, m := range mailDomains {
		hosts = append(hosts, hostSrc{m.Domain, "mail"})
	}

	seen := map[string]bool{}
	var result []DetectItem
	for _, h := range hosts {
		host := normalizeHost(h.host)
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		item := DetectItem{Host: host, Source: h.source, ExpectedIP: expected}
		if zone := findZoneForHost(zones, host); zone != nil {
			item.ZoneFound = true
			item.ZoneName = zone.Name
			item.ProviderID = zone.ProviderID
			item.Provider = providerNames[zone.ProviderID]
			recName := recordNameForProvider(item.Provider, zone.Name, host)
			var local models.DNSRecord
			err := s.db.Where("provider_id = ? AND domain = ? AND type = ? AND name = ?",
				zone.ProviderID, zone.Name, "A", recName).First(&local).Error
			if err == nil {
				item.CurrentValue = local.Value
				item.RecordType = "A"
			} else {
				item.CurrentValue = lookupA(host)
				item.RecordType = "A"
			}
			item.NeedsFix = item.CurrentValue != expected
		} else {
			item.CurrentValue = lookupA(host)
			item.RecordType = "A"
			item.NeedsFix = item.CurrentValue != "" && item.CurrentValue != expected
		}
		result = append(result, item)
	}
	return result, nil
}

func lookupA(host string) string {
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return ""
	}
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			return v4.String()
		}
	}
	return ips[0].String()
}

type ApplyDNSRequest struct {
	Hosts      []string `json:"hosts"`
	IP         string   `json:"ip"`
	ProviderID uint     `json:"provider_id"`
	Proxied    bool     `json:"proxied"`
	WebsiteID  uint     `json:"website_id"`
}

func (s *Service) ApplyRecords(req ApplyDNSRequest) ([]models.DNSRecord, error) {
	ip := req.IP
	if ip == "" {
		ip = s.ServerIP()
	}
	var acc *models.DNSProviderAccount
	var err error
	if req.ProviderID > 0 {
		acc, err = s.getAccount(req.ProviderID)
	} else {
		acc, err = s.defaultAccount()
	}
	if err != nil {
		return s.applyLocalOnly(req.Hosts, ip, req.WebsiteID)
	}

	var zones []models.DNSZone
	s.db.Where("provider_id = ?", acc.ID).Find(&zones)
	client, err := providerClient(acc.Provider)
	if err != nil {
		return nil, err
	}

	var created []models.DNSRecord
	for _, host := range req.Hosts {
		host = normalizeHost(host)
		if host == "" {
			continue
		}
		zone := findZoneForHost(zones, host)
		if zone == nil {
			_, _ = s.SyncProviderZones(acc.ID)
			s.db.Where("provider_id = ?", acc.ID).Find(&zones)
			zone = findZoneForHost(zones, host)
		}
		if zone == nil {
			name, domain := splitHostZone(host)
			rec := models.DNSRecord{
				Domain: domain, Type: "A",
				Name: name, Value: ip, TTL: 600,
				SyncStatus: "local", WebsiteID: req.WebsiteID,
				Comment: "zone not found in provider",
			}
			_ = s.db.Create(&rec)
			created = append(created, rec)
			continue
		}

		recName := recordNameForProvider(acc.Provider, zone.Name, host)
		fqdn := fqdnForProvider(acc.Provider, zone.Name, recName)
		remote := RemoteRecord{
			Type: "A", Name: fqdn, Content: ip, TTL: 600, Proxied: req.Proxied,
		}
		zoneKey := zone.ZoneID
		if acc.Provider == "alidns" {
			zoneKey = zone.Name
			remote.Name = recName
		} else if acc.Provider == "dnspod" {
			remote.Name = recName
		}

		var local models.DNSRecord
		err := s.db.Where("provider_id = ? AND domain = ? AND type = ? AND name = ?",
			acc.ID, zone.Name, "A", recName).First(&local).Error
		if err == gorm.ErrRecordNotFound && req.WebsiteID > 0 {
			err = s.db.Where("domain = ? AND type = ? AND name = ? AND website_id = ?",
				zone.Name, "A", recName, req.WebsiteID).First(&local).Error
		}

		var extID string
		if err == gorm.ErrRecordNotFound {
			extID, err = client.CreateRecord(context.Background(), acc, zoneKey, remote)
		} else if err == nil && local.ExternalID != "" {
			remote.ID = local.ExternalID
			err = client.UpdateRecord(context.Background(), acc, zoneKey, remote)
			extID = local.ExternalID
		} else if err == nil {
			extID, err = client.CreateRecord(context.Background(), acc, zoneKey, remote)
		}
		rec := models.DNSRecord{
			Domain: zone.Name, Type: "A", Name: recName, Value: ip, TTL: 600,
			ProviderID: acc.ID, ZoneID: zone.ZoneID, ExternalID: extID,
			Proxied: req.Proxied, WebsiteID: req.WebsiteID,
		}
		if err != nil {
			rec.SyncStatus = "error"
			rec.SyncError = err.Error()
		} else {
			rec.SyncStatus = "synced"
		}
		if local.ID > 0 {
			local.Value = ip
			local.ExternalID = extID
			local.ProviderID = acc.ID
			local.ZoneID = zone.ZoneID
			local.SyncStatus = rec.SyncStatus
			local.SyncError = rec.SyncError
			local.Proxied = req.Proxied
			local.WebsiteID = req.WebsiteID
			s.db.Save(&local)
			created = append(created, local)
		} else {
			s.db.Create(&rec)
			created = append(created, rec)
		}
	}
	return created, nil
}

func (s *Service) applyLocalOnly(hosts []string, ip string, websiteID uint) ([]models.DNSRecord, error) {
	var created []models.DNSRecord
	for _, host := range hosts {
		name, domain := splitHostZone(host)
		var existing models.DNSRecord
		err := s.db.Where("domain = ? AND type = ? AND name = ? AND website_id = ?", domain, "A", name, websiteID).First(&existing).Error
		if err == nil {
			existing.Value = ip
			s.db.Save(&existing)
			created = append(created, existing)
			continue
		}
		rec := models.DNSRecord{
			Domain: domain, Type: "A", Name: name, Value: ip, TTL: 600,
			SyncStatus: "local", WebsiteID: websiteID,
		}
		s.db.Create(&rec)
		created = append(created, rec)
	}
	return created, nil
}

func (s *Service) AutoDNSForWebsite(primary string, aliases []string, websiteID uint) error {
	ip := s.ServerIP()
	hosts := []string{primary}
	hosts = append(hosts, aliases...)
	_, err := s.ApplyRecords(ApplyDNSRequest{Hosts: hosts, IP: ip, WebsiteID: websiteID})
	return err
}

func (s *Service) CreateAndSync(r *models.DNSRecord) error {
	if err := s.db.Create(r).Error; err != nil {
		return err
	}
	if r.ProviderID == 0 {
		acc, err := s.defaultAccount()
		if err != nil {
			r.SyncStatus = "local"
			return s.db.Save(r).Error
		}
		r.ProviderID = acc.ID
	}
	acc, err := s.getAccount(r.ProviderID)
	if err != nil {
		r.SyncStatus = "local"
		return s.db.Save(r).Error
	}
	var zone models.DNSZone
	if err := s.db.Where("provider_id = ? AND name = ?", acc.ID, r.Domain).First(&zone).Error; err != nil {
		r.SyncStatus = "local"
		return s.db.Save(r).Error
	}
	client, _ := providerClient(acc.Provider)
	zoneKey := zone.ZoneID
	recName := r.Name
	fqdn := fqdnForProvider(acc.Provider, zone.Name, recName)
	remote := RemoteRecord{
		Type: r.Type, Name: fqdn, Content: r.Value, TTL: r.TTL, Proxied: r.Proxied,
	}
	if acc.Provider == "alidns" {
		zoneKey = zone.Name
		remote.Name = recName
	} else if acc.Provider == "dnspod" {
		remote.Name = recName
	}
	extID, err := client.CreateRecord(context.Background(), acc, zoneKey, remote)
	if err != nil {
		r.SyncStatus = "error"
		r.SyncError = err.Error()
	} else {
		r.ExternalID = extID
		r.ZoneID = zone.ZoneID
		r.SyncStatus = "synced"
	}
	return s.db.Save(r).Error
}

func (s *Service) UpdateAndSync(r *models.DNSRecord) error {
	if r.ProviderID > 0 && r.ExternalID != "" {
		acc, err := s.getAccount(r.ProviderID)
		if err == nil {
			client, _ := providerClient(acc.Provider)
			zoneKey := r.ZoneID
			if acc.Provider == "alidns" {
				zoneKey = r.Domain
			}
			recName := r.Name
			fqdn := fqdnForProvider(acc.Provider, r.Domain, recName)
			remote := RemoteRecord{
				ID: r.ExternalID, Type: r.Type, Name: fqdn, Content: r.Value,
				TTL: r.TTL, Proxied: r.Proxied,
			}
			if acc.Provider != "cloudflare" {
				remote.Name = recName
			}
			if err := client.UpdateRecord(context.Background(), acc, zoneKey, remote); err != nil {
				r.SyncStatus = "error"
				r.SyncError = err.Error()
			} else {
				r.SyncStatus = "synced"
				r.SyncError = ""
			}
		}
	}
	return s.db.Save(r).Error
}

func (s *Service) deleteRemoteRecord(rec *models.DNSRecord) error {
	acc, err := s.getAccount(rec.ProviderID)
	if err != nil {
		return nil
	}
	client, err := providerClient(acc.Provider)
	if err != nil {
		return err
	}
	zoneKey := rec.ZoneID
	if acc.Provider == "alidns" {
		zoneKey = rec.Domain
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return client.DeleteRecord(ctx, acc, zoneKey, rec.ExternalID)
}
