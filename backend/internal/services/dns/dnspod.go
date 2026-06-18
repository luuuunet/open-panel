package dns

import (
	"context"
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

type dnsPodClient struct{}

func (c *dnsPodClient) token(account *models.DNSProviderAccount) string {
	if account.APIToken != "" {
		return account.APIToken
	}
	if account.AccessKey != "" && account.SecretKey != "" {
		return account.AccessKey + "," + account.SecretKey
	}
	return ""
}

func (c *dnsPodClient) call(account *models.DNSProviderAccount, action string, extra map[string]string, out any) error {
	token := c.token(account)
	if token == "" {
		return fmt.Errorf("dnspod token required (format: id,token)")
	}
	form := map[string]string{
		"login_token": token,
		"format":      "json",
		"lang":        "cn",
		"error_on_empty": "no",
	}
	for k, v := range extra {
		form[k] = v
	}
	var resp struct {
		Status struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"status"`
	}
	if err := doFormPOST("https://dnsapi.cn/"+action, form, &resp); err != nil {
		return err
	}
	if resp.Status.Code != "1" {
		return fmt.Errorf("dnspod: %s", resp.Status.Message)
	}
	return doFormPOST("https://dnsapi.cn/"+action, form, out)
}

func (c *dnsPodClient) Test(ctx context.Context, account *models.DNSProviderAccount) error {
	var out struct {
		Status struct {
			Code string `json:"code"`
			Message string `json:"message"`
		} `json:"status"`
	}
	token := c.token(account)
	if token == "" {
		return fmt.Errorf("dnspod token required")
	}
	form := map[string]string{
		"login_token": token,
		"format":      "json",
	}
	if err := doFormPOST("https://dnsapi.cn/User.Detail", form, &out); err != nil {
		return err
	}
	if out.Status.Code != "1" {
		return fmt.Errorf("dnspod: %s", out.Status.Message)
	}
	return nil
}

func (c *dnsPodClient) ListZones(ctx context.Context, account *models.DNSProviderAccount) ([]ZoneInfo, error) {
	var out struct {
		Domains []struct {
			ID     int    `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
			Grade  string `json:"grade"`
		} `json:"domains"`
	}
	if err := c.call(account, "Domain.List", map[string]string{"type": "all", "offset": "0", "length": "1000"}, &out); err != nil {
		return nil, err
	}
	zones := make([]ZoneInfo, 0, len(out.Domains))
	for _, d := range out.Domains {
		zones = append(zones, ZoneInfo{
			ID: fmt.Sprintf("%d", d.ID), Name: d.Name, Status: d.Status,
		})
	}
	return zones, nil
}

func (c *dnsPodClient) ListRecords(ctx context.Context, account *models.DNSProviderAccount, zoneID string) ([]RemoteRecord, error) {
	var zoneName string
	var zones []ZoneInfo
	var err error
	if strings.Contains(zoneID, ".") {
		zoneName = zoneID
	} else {
		zones, err = c.ListZones(ctx, account)
		if err != nil {
			return nil, err
		}
		for _, z := range zones {
			if z.ID == zoneID {
				zoneName = z.Name
				break
			}
		}
	}
	if zoneName == "" {
		zoneName = zoneID
	}
	var out struct {
		Records []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Name     string `json:"name"`
			Value    string `json:"value"`
			TTL      string `json:"ttl"`
			MX       int    `json:"mx"`
			Enabled  string `json:"enabled"`
		} `json:"records"`
	}
	if err := c.call(account, "Record.List", map[string]string{
		"domain": zoneName,
		"offset": "0",
		"length": "3000",
	}, &out); err != nil {
		return nil, err
	}
	records := make([]RemoteRecord, 0, len(out.Records))
	for _, r := range out.Records {
		name := strings.TrimSuffix(r.Name, "."+zoneName)
		if name == zoneName {
			name = "@"
		}
		ttl := 600
		fmt.Sscanf(r.TTL, "%d", &ttl)
		records = append(records, RemoteRecord{
			ID: r.ID, Type: r.Type, Name: name, Content: r.Value,
			TTL: ttl, Priority: r.MX,
		})
	}
	return records, nil
}

func (c *dnsPodClient) CreateRecord(ctx context.Context, account *models.DNSProviderAccount, zoneID string, rec RemoteRecord) (string, error) {
	zoneName := zoneID
	if !strings.Contains(zoneID, ".") {
		zones, err := c.ListZones(ctx, account)
		if err != nil {
			return "", err
		}
		for _, z := range zones {
			if z.ID == zoneID {
				zoneName = z.Name
				break
			}
		}
	}
	sub := rec.Name
	if sub == "@" {
		sub = "@"
	}
	params := map[string]string{
		"domain": zoneName,
		"sub_domain": sub,
		"record_type": rec.Type,
		"record_line": "默认",
		"value":       rec.Content,
		"ttl":         fmt.Sprintf("%d", rec.TTL),
	}
	if rec.Type == "MX" {
		params["mx"] = fmt.Sprintf("%d", rec.Priority)
	}
	var out struct {
		Record struct {
			ID string `json:"id"`
		} `json:"record"`
	}
	if err := c.call(account, "Record.Create", params, &out); err != nil {
		return "", err
	}
	return out.Record.ID, nil
}

func (c *dnsPodClient) UpdateRecord(ctx context.Context, account *models.DNSProviderAccount, zoneID string, rec RemoteRecord) error {
	zoneName := zoneID
	if !strings.Contains(zoneID, ".") {
		zones, err := c.ListZones(ctx, account)
		if err != nil {
			return err
		}
		for _, z := range zones {
			if z.ID == zoneID {
				zoneName = z.Name
				break
			}
		}
	}
	sub := rec.Name
	params := map[string]string{
		"domain":      zoneName,
		"record_id":   rec.ID,
		"sub_domain":  sub,
		"record_type": rec.Type,
		"record_line": "默认",
		"value":       rec.Content,
		"ttl":         fmt.Sprintf("%d", rec.TTL),
	}
	var out struct {
		Status struct {
			Code string `json:"code"`
		} `json:"status"`
	}
	return c.call(account, "Record.Modify", params, &out)
}

func (c *dnsPodClient) DeleteRecord(ctx context.Context, account *models.DNSProviderAccount, zoneID, recordID string) error {
	zoneName := zoneID
	if !strings.Contains(zoneID, ".") {
		zones, err := c.ListZones(ctx, account)
		if err != nil {
			return err
		}
		for _, z := range zones {
			if z.ID == zoneID {
				zoneName = z.Name
				break
			}
		}
	}
	var out struct{}
	return c.call(account, "Record.Remove", map[string]string{
		"domain":    zoneName,
		"record_id": recordID,
	}, &out)
}
