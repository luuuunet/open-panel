package dns

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

type aliDNSClient struct{}

func aliSign(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var pairs []string
	for _, k := range keys {
		pairs = append(pairs, specialURLEncode(k)+"="+specialURLEncode(params[k]))
	}
	stringToSign := "GET&%2F&" + specialURLEncode(strings.Join(pairs, "&"))
	mac := hmac.New(sha1.New, []byte(secret+"&"))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func specialURLEncode(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

func (c *aliDNSClient) call(account *models.DNSProviderAccount, action string, extra map[string]string, out any) error {
	if account.AccessKey == "" || account.SecretKey == "" {
		return fmt.Errorf("alidns access key and secret required")
	}
	params := map[string]string{
		"Format":           "JSON",
		"Version":          "2015-01-09",
		"AccessKeyId":      account.AccessKey,
		"SignatureMethod":  "HMAC-SHA1",
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"SignatureVersion": "1.0",
		"SignatureNonce":   fmt.Sprintf("%d", time.Now().UnixNano()),
		"Action":           action,
	}
	for k, v := range extra {
		params[k] = v
	}
	params["Signature"] = aliSign(params, account.SecretKey)
	u, _ := url.Parse("https://alidns.aliyuncs.com/")
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	resp, err := httpClient.Get(u.String())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("alidns HTTP %d: %s", resp.StatusCode, string(data))
	}
	return json.Unmarshal(data, out)
}

func (c *aliDNSClient) Test(ctx context.Context, account *models.DNSProviderAccount) error {
	var out struct {
		TotalCount int `json:"TotalCount"`
		Code       string `json:"Code"`
		Message    string `json:"Message"`
	}
	if err := c.call(account, "DescribeDomains", map[string]string{"PageSize": "1"}, &out); err != nil {
		return err
	}
	if out.Code != "" {
		return fmt.Errorf("%s: %s", out.Code, out.Message)
	}
	return nil
}

func (c *aliDNSClient) ListZones(ctx context.Context, account *models.DNSProviderAccount) ([]ZoneInfo, error) {
	var zones []ZoneInfo
	page := 1
	for {
		var out struct {
			Domains struct {
				Domain []struct {
					DomainId string `json:"DomainId"`
					DomainName string `json:"DomainName"`
					DomainNameServers struct {
						DomainNameServer []string `json:"DomainNameServer"`
					} `json:"DomainNameServers"`
				} `json:"Domain"`
			} `json:"Domains"`
			TotalCount int `json:"TotalCount"`
			Code       string `json:"Code"`
			Message    string `json:"Message"`
		}
		if err := c.call(account, "DescribeDomains", map[string]string{
			"PageNumber": fmt.Sprintf("%d", page),
			"PageSize":   "50",
		}, &out); err != nil {
			return nil, err
		}
		if out.Code != "" {
			return nil, fmt.Errorf("%s: %s", out.Code, out.Message)
		}
		for _, d := range out.Domains.Domain {
			zones = append(zones, ZoneInfo{
				ID: d.DomainId, Name: d.DomainName, Status: "active",
				NameServers: d.DomainNameServers.DomainNameServer,
			})
		}
		if len(out.Domains.Domain) == 0 || len(zones) >= out.TotalCount {
			break
		}
		page++
	}
	return zones, nil
}

func (c *aliDNSClient) ListRecords(ctx context.Context, account *models.DNSProviderAccount, zoneID string) ([]RemoteRecord, error) {
	var records []RemoteRecord
	page := 1
	for {
		var out struct {
			DomainRecords struct {
				Record []struct {
					RecordId string `json:"RecordId"`
					Type     string `json:"Type"`
					RR       string `json:"RR"`
					Value    string `json:"Value"`
					TTL      int    `json:"TTL"`
					Priority int    `json:"Priority"`
				} `json:"Record"`
			} `json:"DomainRecords"`
			TotalCount int `json:"TotalCount"`
			Code       string `json:"Code"`
			Message    string `json:"Message"`
		}
		if err := c.call(account, "DescribeDomainRecords", map[string]string{
			"DomainName": zoneID,
			"PageNumber": fmt.Sprintf("%d", page),
			"PageSize":   "100",
		}, &out); err != nil {
			return nil, err
		}
		if out.Code != "" {
			return nil, fmt.Errorf("%s: %s", out.Code, out.Message)
		}
		for _, r := range out.DomainRecords.Record {
			name := r.RR
			if name == "" {
				name = "@"
			}
			records = append(records, RemoteRecord{
				ID: r.RecordId, Type: r.Type, Name: name, Content: r.Value,
				TTL: r.TTL, Priority: r.Priority,
			})
		}
		if len(out.DomainRecords.Record) == 0 || len(records) >= out.TotalCount {
			break
		}
		page++
	}
	return records, nil
}

func (c *aliDNSClient) CreateRecord(ctx context.Context, account *models.DNSProviderAccount, zoneID string, rec RemoteRecord) (string, error) {
	rr := rec.Name
	if rr == "@" {
		rr = ""
	}
	params := map[string]string{
		"DomainName": zoneID,
		"RR":         rr,
		"Type":       rec.Type,
		"Value":      rec.Content,
		"TTL":        fmt.Sprintf("%d", rec.TTL),
	}
	if rec.Type == "MX" {
		params["Priority"] = fmt.Sprintf("%d", rec.Priority)
	}
	var out struct {
		RecordId string `json:"RecordId"`
		Code     string `json:"Code"`
		Message  string `json:"Message"`
	}
	if err := c.call(account, "AddDomainRecord", params, &out); err != nil {
		return "", err
	}
	if out.Code != "" {
		return "", fmt.Errorf("%s: %s", out.Code, out.Message)
	}
	return out.RecordId, nil
}

func (c *aliDNSClient) UpdateRecord(ctx context.Context, account *models.DNSProviderAccount, zoneID string, rec RemoteRecord) error {
	rr := rec.Name
	if rr == "@" {
		rr = ""
	}
	params := map[string]string{
		"RecordId": rec.ID,
		"RR":       rr,
		"Type":     rec.Type,
		"Value":    rec.Content,
		"TTL":      fmt.Sprintf("%d", rec.TTL),
	}
	var out struct {
		Code    string `json:"Code"`
		Message string `json:"Message"`
	}
	if err := c.call(account, "UpdateDomainRecord", params, &out); err != nil {
		return err
	}
	if out.Code != "" {
		return fmt.Errorf("%s: %s", out.Code, out.Message)
	}
	return nil
}

func (c *aliDNSClient) DeleteRecord(ctx context.Context, account *models.DNSProviderAccount, zoneID, recordID string) error {
	var out struct {
		Code    string `json:"Code"`
		Message string `json:"Message"`
	}
	if err := c.call(account, "DeleteDomainRecord", map[string]string{"RecordId": recordID}, &out); err != nil {
		return err
	}
	if out.Code != "" {
		return fmt.Errorf("%s: %s", out.Code, out.Message)
	}
	return nil
}
