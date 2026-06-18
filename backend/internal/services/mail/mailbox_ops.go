package mail

import (
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

type MailboxExportRow struct {
	Address  string `json:"address"`
	Domain   string `json:"domain"`
	Password string `json:"password"`
	Quota    int    `json:"quota"`
	Maildir  string `json:"maildir,omitempty"`
}

type BatchMailboxRequest struct {
	Domain           string `json:"domain"`
	Lines            string `json:"lines"`
	DefaultPassword  string `json:"default_password"`
	GeneratePassword bool   `json:"generate_password"`
	Quota            int    `json:"quota"`
}

type BatchMailboxItem struct {
	Address  string `json:"address"`
	Password string `json:"password,omitempty"`
	Error    string `json:"error,omitempty"`
}

type BatchMailboxResult struct {
	Created int                `json:"created"`
	Failed  int                `json:"failed"`
	Items   []BatchMailboxItem `json:"items"`
}

type ImportMailboxRequest struct {
	Accounts    []MailboxExportRow `json:"accounts"`
	SkipExisting bool              `json:"skip_existing"`
	UpdatePassword bool            `json:"update_password"`
}

type ImportMailboxResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

func (s *Service) ExportMailboxes(domain, format string) ([]byte, string, error) {
	list, err := s.ListMailboxes(domain)
	if err != nil {
		return nil, "", err
	}
	rows := make([]MailboxExportRow, 0, len(list))
	for _, m := range list {
		rows = append(rows, MailboxExportRow{
			Address:  m.Address,
			Domain:   m.Domain,
			Password: s.readPassSecret(m.Address),
			Quota:    m.Quota,
			Maildir:  m.Maildir,
		})
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "json" {
		data, err := json.MarshalIndent(rows, "", "  ")
		return data, "mailboxes.json", err
	}
	var buf strings.Builder
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"address", "password", "domain", "quota"})
	for _, r := range rows {
		_ = w.Write([]string{r.Address, r.Password, r.Domain, fmt.Sprintf("%d", r.Quota)})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, "", err
	}
	return []byte(buf.String()), "mailboxes.csv", nil
}

func (s *Service) BatchCreateMailboxes(req *BatchMailboxRequest) (*BatchMailboxResult, error) {
	domain := strings.TrimSpace(strings.ToLower(req.Domain))
	if domain == "" {
		return nil, fmt.Errorf("域名不能为空")
	}
	lines := parseBatchLines(req.Lines)
	if len(lines) == 0 {
		return nil, fmt.Errorf("请填写至少一个邮箱账号")
	}
	quota := req.Quota
	if quota <= 0 {
		quota = 1024
	}
	res := &BatchMailboxResult{Items: make([]BatchMailboxItem, 0, len(lines))}
	for _, line := range lines {
		user, pass := splitUserPass(line)
		user = strings.TrimSpace(strings.ToLower(user))
		if user == "" {
			continue
		}
		address := user
		if !strings.Contains(address, "@") {
			address = user + "@" + domain
		}
		if pass == "" {
			if req.GeneratePassword {
				pass = randomPassword(12)
			} else {
				pass = strings.TrimSpace(req.DefaultPassword)
			}
		}
		if pass == "" {
			res.Failed++
			res.Items = append(res.Items, BatchMailboxItem{Address: address, Error: "密码为空"})
			continue
		}
		m := &models.MailBox{Domain: domain, Address: address, Quota: quota}
		if err := s.CreateMailbox(m, pass); err != nil {
			res.Failed++
			res.Items = append(res.Items, BatchMailboxItem{Address: address, Error: err.Error()})
			continue
		}
		res.Created++
		res.Items = append(res.Items, BatchMailboxItem{Address: address, Password: pass})
	}
	if res.Created > 0 {
		_ = s.syncMaps()
	}
	return res, nil
}

func (s *Service) ImportMailboxes(req *ImportMailboxRequest) (*ImportMailboxResult, error) {
	if len(req.Accounts) == 0 {
		return nil, fmt.Errorf("导入列表为空")
	}
	res := &ImportMailboxResult{}
	for _, acc := range req.Accounts {
		address := strings.TrimSpace(strings.ToLower(acc.Address))
		domain := strings.TrimSpace(strings.ToLower(acc.Domain))
		pass := strings.TrimSpace(acc.Password)
		if address == "" {
			res.Failed++
			res.Errors = append(res.Errors, "缺少邮箱地址")
			continue
		}
		if !strings.Contains(address, "@") && domain != "" {
			address = address + "@" + domain
		}
		if domain == "" && strings.Contains(address, "@") {
			domain = strings.SplitN(address, "@", 2)[1]
		}
		if pass == "" {
			res.Failed++
			res.Errors = append(res.Errors, address+": 密码为空")
			continue
		}
		quota := acc.Quota
		if quota <= 0 {
			quota = 1024
		}
		var existing models.MailBox
		err := s.db.Where("address = ?", address).First(&existing).Error
		if err == nil {
			if req.SkipExisting && !req.UpdatePassword {
				res.Skipped++
				continue
			}
			if req.UpdatePassword {
				if err := s.UpdateMailboxPassword(existing.ID, pass); err != nil {
					res.Failed++
					res.Errors = append(res.Errors, address+": "+err.Error())
					continue
				}
				res.Updated++
				continue
			}
			res.Skipped++
			continue
		}
		m := &models.MailBox{Domain: domain, Address: address, Quota: quota}
		if err := s.CreateMailbox(m, pass); err != nil {
			res.Failed++
			res.Errors = append(res.Errors, address+": "+err.Error())
			continue
		}
		res.Created++
	}
	if res.Created > 0 || res.Updated > 0 {
		_ = s.syncMaps()
	}
	return res, nil
}

func (s *Service) ParseImportData(format string, r io.Reader) ([]MailboxExportRow, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil, fmt.Errorf("文件为空")
	}
	if format == "json" || strings.HasPrefix(text, "[") || strings.HasPrefix(text, "{") {
		var rows []MailboxExportRow
		if err := json.Unmarshal(data, &rows); err != nil {
			var wrap struct {
				Accounts []MailboxExportRow `json:"accounts"`
			}
			if err2 := json.Unmarshal(data, &wrap); err2 != nil {
				return nil, fmt.Errorf("JSON 解析失败")
			}
			rows = wrap.Accounts
		}
		return rows, nil
	}
	cr := csv.NewReader(strings.NewReader(text))
	cr.TrimLeadingSpace = true
	records, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("CSV 解析失败: %w", err)
	}
	var rows []MailboxExportRow
	start := 0
	if len(records) > 0 && strings.EqualFold(records[0][0], "address") {
		start = 1
	}
	for _, rec := range records[start:] {
		if len(rec) == 0 || strings.TrimSpace(rec[0]) == "" {
			continue
		}
		row := MailboxExportRow{Address: strings.TrimSpace(rec[0])}
		if len(rec) > 1 {
			row.Password = strings.TrimSpace(rec[1])
		}
		if len(rec) > 2 {
			row.Domain = strings.TrimSpace(rec[2])
		}
		if len(rec) > 3 {
			fmt.Sscanf(strings.TrimSpace(rec[3]), "%d", &row.Quota)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func parseBatchLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func splitUserPass(line string) (user, pass string) {
	if i := strings.Index(line, ":"); i > 0 {
		return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:])
	}
	if i := strings.Index(line, ","); i > 0 && !strings.Contains(line, "@") {
		return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:])
	}
	return line, ""
}

func randomPassword(n int) string {
	if n < 8 {
		n = 8
	}
	b := make([]byte, (n+1)/2)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}
