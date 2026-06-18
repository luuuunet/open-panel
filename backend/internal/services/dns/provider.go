package dns

import (
	"context"
	"fmt"

	"github.com/luuuunet/owpanel/internal/models"
)

type ZoneInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	NameServers []string `json:"name_servers"`
}

type RemoteRecord struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Proxied  bool   `json:"proxied"`
	Priority int    `json:"priority"`
}

type ProviderClient interface {
	Test(ctx context.Context, account *models.DNSProviderAccount) error
	ListZones(ctx context.Context, account *models.DNSProviderAccount) ([]ZoneInfo, error)
	ListRecords(ctx context.Context, account *models.DNSProviderAccount, zoneID string) ([]RemoteRecord, error)
	CreateRecord(ctx context.Context, account *models.DNSProviderAccount, zoneID string, rec RemoteRecord) (string, error)
	UpdateRecord(ctx context.Context, account *models.DNSProviderAccount, zoneID string, rec RemoteRecord) error
	DeleteRecord(ctx context.Context, account *models.DNSProviderAccount, zoneID, recordID string) error
}

func providerClient(name string) (ProviderClient, error) {
	switch name {
	case "cloudflare":
		return &cloudflareClient{}, nil
	case "alidns":
		return &aliDNSClient{}, nil
	case "dnspod":
		return &dnsPodClient{}, nil
	default:
		return nil, fmt.Errorf("unsupported dns provider: %s", name)
	}
}

var SupportedProviders = []map[string]string{
	{"key": "cloudflare", "name": "Cloudflare", "auth": "api_token"},
	{"key": "alidns", "name": "阿里云 DNS", "auth": "access_key"},
	{"key": "dnspod", "name": "DNSPod (腾讯云)", "auth": "token"},
}
