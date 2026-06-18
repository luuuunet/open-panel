package dns

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

type cloudflareClient struct{}

func (c *cloudflareClient) headers(account *models.DNSProviderAccount) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + account.APIToken,
	}
}

func (c *cloudflareClient) Test(ctx context.Context, account *models.DNSProviderAccount) error {
	if strings.TrimSpace(account.APIToken) == "" {
		return fmt.Errorf("cloudflare api token required")
	}
	var out struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := doJSON(http.MethodGet, "https://api.cloudflare.com/client/v4/user/tokens/verify", c.headers(account), nil, &out); err != nil {
		return err
	}
	if !out.Success {
		if len(out.Errors) > 0 {
			return fmt.Errorf("%s", out.Errors[0].Message)
		}
		return fmt.Errorf("cloudflare token invalid")
	}
	return nil
}

func (c *cloudflareClient) ListZones(ctx context.Context, account *models.DNSProviderAccount) ([]ZoneInfo, error) {
	var zones []ZoneInfo
	page := 1
	for {
		u := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones?per_page=50&page=%d", page)
		var out struct {
			Success bool `json:"success"`
			Result  []struct {
				ID              string   `json:"id"`
				Name            string   `json:"name"`
				Status          string   `json:"status"`
				NameServers     []string `json:"name_servers"`
			} `json:"result"`
			ResultInfo struct {
				TotalPages int `json:"total_pages"`
			} `json:"result_info"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if err := doJSON(http.MethodGet, u, c.headers(account), nil, &out); err != nil {
			return nil, err
		}
		if !out.Success {
			if len(out.Errors) > 0 {
				return nil, fmt.Errorf(out.Errors[0].Message)
			}
			return nil, fmt.Errorf("cloudflare list zones failed")
		}
		for _, z := range out.Result {
			zones = append(zones, ZoneInfo{
				ID: z.ID, Name: z.Name, Status: z.Status, NameServers: z.NameServers,
			})
		}
		if page >= out.ResultInfo.TotalPages || out.ResultInfo.TotalPages == 0 {
			break
		}
		page++
	}
	return zones, nil
}

func (c *cloudflareClient) ListRecords(ctx context.Context, account *models.DNSProviderAccount, zoneID string) ([]RemoteRecord, error) {
	var records []RemoteRecord
	page := 1
	for {
		u := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?per_page=100&page=%d", zoneID, page)
		var out struct {
			Success bool `json:"success"`
			Result  []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Name     string `json:"name"`
				Content  string `json:"content"`
				TTL      int    `json:"ttl"`
				Proxied  bool   `json:"proxied"`
				Priority int    `json:"priority"`
			} `json:"result"`
			ResultInfo struct {
				TotalPages int `json:"total_pages"`
			} `json:"result_info"`
		}
		if err := doJSON(http.MethodGet, u, c.headers(account), nil, &out); err != nil {
			return nil, err
		}
		for _, r := range out.Result {
			records = append(records, RemoteRecord{
				ID: r.ID, Type: r.Type, Name: r.Name, Content: r.Content,
				TTL: r.TTL, Proxied: r.Proxied, Priority: r.Priority,
			})
		}
		if page >= out.ResultInfo.TotalPages || out.ResultInfo.TotalPages == 0 {
			break
		}
		page++
	}
	return records, nil
}

func (c *cloudflareClient) CreateRecord(ctx context.Context, account *models.DNSProviderAccount, zoneID string, rec RemoteRecord) (string, error) {
	body := map[string]any{
		"type":    rec.Type,
		"name":    rec.Name,
		"content": rec.Content,
		"ttl":     rec.TTL,
	}
	if rec.Type == "A" || rec.Type == "AAAA" || rec.Type == "CNAME" {
		body["proxied"] = rec.Proxied
	}
	if rec.Type == "MX" {
		body["priority"] = rec.Priority
	}
	u := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", zoneID)
	var out struct {
		Success bool `json:"success"`
		Result  struct {
			ID string `json:"id"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := doJSON(http.MethodPost, u, c.headers(account), body, &out); err != nil {
		return "", err
	}
	if !out.Success {
		if len(out.Errors) > 0 {
			return "", fmt.Errorf("%s", out.Errors[0].Message)
		}
		return "", fmt.Errorf("create record failed")
	}
	return out.Result.ID, nil
}

func (c *cloudflareClient) UpdateRecord(ctx context.Context, account *models.DNSProviderAccount, zoneID string, rec RemoteRecord) error {
	body := map[string]any{
		"type":    rec.Type,
		"name":    rec.Name,
		"content": rec.Content,
		"ttl":     rec.TTL,
	}
	if rec.Type == "A" || rec.Type == "AAAA" || rec.Type == "CNAME" {
		body["proxied"] = rec.Proxied
	}
	u := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", zoneID, url.PathEscape(rec.ID))
	var out struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := doJSON(http.MethodPut, u, c.headers(account), body, &out); err != nil {
		return err
	}
	if !out.Success && len(out.Errors) > 0 {
		return fmt.Errorf("%s", out.Errors[0].Message)
	}
	return nil
}

func (c *cloudflareClient) DeleteRecord(ctx context.Context, account *models.DNSProviderAccount, zoneID, recordID string) error {
	u := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", zoneID, recordID)
	return doJSON(http.MethodDelete, u, c.headers(account), nil, nil)
}
