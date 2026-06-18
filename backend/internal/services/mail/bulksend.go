package mail

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

var bulkSendMu sync.Mutex
var bulkSending = map[uint]bool{}

type BulkProviderView struct {
	models.MailSendProvider
	Config map[string]string `json:"config,omitempty"`
}

func (s *Service) BulkProviderCatalog() []BulkProviderMeta {
	return BulkProviderCatalog
}

func (s *Service) ListBulkProviders() ([]BulkProviderView, error) {
	var list []models.MailSendProvider
	if err := s.db.Order("id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	out := make([]BulkProviderView, len(list))
	for i, p := range list {
		out[i] = BulkProviderView{MailSendProvider: p, Config: maskBulkConfig(p.ConfigJSON)}
	}
	return out, nil
}

type BulkProviderRequest struct {
	Name            string            `json:"name"`
	ProviderType    string            `json:"provider_type"`
	Enabled         *bool             `json:"enabled"`
	IsDefault       *bool             `json:"is_default"`
	DefaultFrom     string            `json:"default_from"`
	DefaultFromName string            `json:"default_from_name"`
	Config          map[string]string `json:"config"`
}

func (s *Service) CreateBulkProvider(req BulkProviderRequest) (*BulkProviderView, error) {
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.ProviderType) == "" {
		return nil, fmt.Errorf("名称与类型不能为空")
	}
	p := models.MailSendProvider{
		Name:            strings.TrimSpace(req.Name),
		ProviderType:    strings.TrimSpace(req.ProviderType),
		Enabled:         true,
		DefaultFrom:     strings.TrimSpace(req.DefaultFrom),
		DefaultFromName: strings.TrimSpace(req.DefaultFromName),
		ConfigJSON:      bulkConfigJSON(mergeBulkConfig("", req.Config)),
	}
	if req.Enabled != nil {
		p.Enabled = *req.Enabled
	}
	if req.IsDefault != nil && *req.IsDefault {
		_ = s.db.Model(&models.MailSendProvider{}).Where("1=1").Update("is_default", false).Error
		p.IsDefault = true
	}
	if err := s.db.Create(&p).Error; err != nil {
		return nil, err
	}
	v := BulkProviderView{MailSendProvider: p, Config: maskBulkConfig(p.ConfigJSON)}
	return &v, nil
}

func (s *Service) UpdateBulkProvider(id uint, req BulkProviderRequest) (*BulkProviderView, error) {
	var p models.MailSendProvider
	if err := s.db.First(&p, id).Error; err != nil {
		return nil, err
	}
	if req.Name != "" {
		p.Name = strings.TrimSpace(req.Name)
	}
	if req.ProviderType != "" {
		p.ProviderType = strings.TrimSpace(req.ProviderType)
	}
	if req.Enabled != nil {
		p.Enabled = *req.Enabled
	}
	if req.DefaultFrom != "" {
		p.DefaultFrom = strings.TrimSpace(req.DefaultFrom)
	}
	if req.DefaultFromName != "" {
		p.DefaultFromName = strings.TrimSpace(req.DefaultFromName)
	}
	if req.Config != nil {
		p.ConfigJSON = bulkConfigJSON(mergeBulkConfig(p.ConfigJSON, req.Config))
	}
	if req.IsDefault != nil && *req.IsDefault {
		_ = s.db.Model(&models.MailSendProvider{}).Where("id != ?", id).Update("is_default", false).Error
		p.IsDefault = true
	}
	if err := s.db.Save(&p).Error; err != nil {
		return nil, err
	}
	v := BulkProviderView{MailSendProvider: p, Config: maskBulkConfig(p.ConfigJSON)}
	return &v, nil
}

func (s *Service) DeleteBulkProvider(id uint) error {
	return s.db.Delete(&models.MailSendProvider{}, id).Error
}

func (s *Service) TestBulkProvider(id uint, to string) error {
	var p models.MailSendProvider
	if err := s.db.First(&p, id).Error; err != nil {
		return err
	}
	to = strings.TrimSpace(to)
	if to == "" {
		return fmt.Errorf("测试收件人不能为空")
	}
	from := p.DefaultFrom
	if from == "" {
		from = "noreply@example.com"
	}
	return s.sendOutbound(&p, outboundMessage{
		From: from, FromName: p.DefaultFromName, To: to,
		Subject: "OWPanel 邮件通道测试",
		BodyText: "这是一封来自 OWPanel 的测试邮件。This is a test message from OWPanel mail provider.",
	})
}

type BulkCampaignRequest struct {
	Name          string `json:"name"`
	ProviderID    uint   `json:"provider_id"`
	FromEmail     string `json:"from_email"`
	FromName      string `json:"from_name"`
	ReplyTo       string `json:"reply_to"`
	Subject       string `json:"subject"`
	BodyHTML      string `json:"body_html"`
	BodyText      string `json:"body_text"`
	Recipients    string `json:"recipients"`
	RatePerMinute int    `json:"rate_per_minute"`
}

func parseRecipientLines(raw string) []struct{ Email, Name string } {
	var out []struct{ Email, Name string }
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.ReplaceAll(line, ";", ",")
		parts := strings.Split(line, ",")
		email := strings.TrimSpace(parts[0])
		if email == "" || !strings.Contains(email, "@") {
			continue
		}
		name := ""
		if len(parts) > 1 {
			name = strings.TrimSpace(parts[1])
		}
		out = append(out, struct{ Email, Name string }{email, name})
	}
	return out
}

func (s *Service) CreateBulkCampaign(req BulkCampaignRequest) (*models.MailBulkCampaign, error) {
	if req.ProviderID == 0 {
		return nil, fmt.Errorf("请选择发送通道")
	}
	if strings.TrimSpace(req.Subject) == "" {
		return nil, fmt.Errorf("主题不能为空")
	}
	recipients := parseRecipientLines(req.Recipients)
	if len(recipients) == 0 {
		return nil, fmt.Errorf("请至少添加一个收件人")
	}
	rate := req.RatePerMinute
	if rate <= 0 {
		rate = 60
	}
	if rate > 600 {
		rate = 600
	}
	c := models.MailBulkCampaign{
		Name: strings.TrimSpace(req.Name), ProviderID: req.ProviderID,
		FromEmail: strings.TrimSpace(req.FromEmail), FromName: strings.TrimSpace(req.FromName),
		ReplyTo: strings.TrimSpace(req.ReplyTo), Subject: strings.TrimSpace(req.Subject),
		BodyHTML: req.BodyHTML, BodyText: req.BodyText,
		Status: "draft", TotalRecipients: len(recipients), RatePerMinute: rate,
	}
	if c.Name == "" {
		c.Name = c.Subject
	}
	if err := s.db.Create(&c).Error; err != nil {
		return nil, err
	}
	for _, r := range recipients {
		_ = s.db.Create(&models.MailBulkRecipient{
			CampaignID: c.ID, Email: r.Email, Name: r.Name, Status: "pending",
		}).Error
	}
	return &c, nil
}

func (s *Service) ListBulkCampaigns() ([]models.MailBulkCampaign, error) {
	var list []models.MailBulkCampaign
	return list, s.db.Order("id desc").Limit(100).Find(&list).Error
}

func (s *Service) GetBulkCampaign(id uint) (*models.MailBulkCampaign, []models.MailBulkRecipient, error) {
	var c models.MailBulkCampaign
	if err := s.db.First(&c, id).Error; err != nil {
		return nil, nil, err
	}
	var rec []models.MailBulkRecipient
	_ = s.db.Where("campaign_id = ?", id).Order("id asc").Find(&rec).Error
	return &c, rec, nil
}

func (s *Service) StartBulkCampaign(id uint) error {
	bulkSendMu.Lock()
	if bulkSending[id] {
		bulkSendMu.Unlock()
		return fmt.Errorf("任务正在发送中")
	}
	bulkSending[id] = true
	bulkSendMu.Unlock()

	var c models.MailBulkCampaign
	if err := s.db.First(&c, id).Error; err != nil {
		s.clearBulkSending(id)
		return err
	}
	if c.Status == "sending" {
		s.clearBulkSending(id)
		return fmt.Errorf("任务已在发送中")
	}
	if c.Status == "completed" {
		s.clearBulkSending(id)
		return fmt.Errorf("任务已完成")
	}
	var provider models.MailSendProvider
	if err := s.db.First(&provider, c.ProviderID).Error; err != nil {
		s.clearBulkSending(id)
		return fmt.Errorf("发送通道不存在")
	}
	if !provider.Enabled {
		s.clearBulkSending(id)
		return fmt.Errorf("发送通道已禁用")
	}
	now := time.Now()
	c.Status = "sending"
	c.StartedAt = &now
	c.SentCount = 0
	c.FailedCount = 0
	c.LastError = ""
	_ = s.db.Save(&c).Error

	go s.runBulkCampaign(id)
	return nil
}

func (s *Service) CancelBulkCampaign(id uint) error {
	var c models.MailBulkCampaign
	if err := s.db.First(&c, id).Error; err != nil {
		return err
	}
	if c.Status != "sending" && c.Status != "draft" {
		return fmt.Errorf("当前状态无法取消")
	}
	c.Status = "cancelled"
	now := time.Now()
	c.FinishedAt = &now
	return s.db.Save(&c).Error
}

func (s *Service) DeleteBulkCampaign(id uint) error {
	var c models.MailBulkCampaign
	if err := s.db.First(&c, id).Error; err != nil {
		return err
	}
	if c.Status == "sending" {
		return fmt.Errorf("发送中无法删除")
	}
	_ = s.db.Where("campaign_id = ?", id).Delete(&models.MailBulkRecipient{}).Error
	return s.db.Delete(&c).Error
}

func (s *Service) clearBulkSending(id uint) {
	bulkSendMu.Lock()
	delete(bulkSending, id)
	bulkSendMu.Unlock()
}

func (s *Service) runBulkCampaign(id uint) {
	defer s.clearBulkSending(id)

	var c models.MailBulkCampaign
	if err := s.db.First(&c, id).Error; err != nil {
		return
	}
	var provider models.MailSendProvider
	if err := s.db.First(&provider, c.ProviderID).Error; err != nil {
		return
	}

	delay := time.Minute / time.Duration(max(c.RatePerMinute, 1))
	from := c.FromEmail
	if from == "" {
		from = provider.DefaultFrom
	}
	fromName := c.FromName
	if fromName == "" {
		fromName = provider.DefaultFromName
	}

	var pending []models.MailBulkRecipient
	_ = s.db.Where("campaign_id = ? AND status = ?", id, "pending").Find(&pending).Error

	for _, r := range pending {
		var fresh models.MailBulkCampaign
		if s.db.First(&fresh, id).Error != nil || fresh.Status == "cancelled" {
			break
		}
		err := s.sendOutbound(&provider, outboundMessage{
			From: from, FromName: fromName, ReplyTo: c.ReplyTo,
			To: r.Email, ToName: r.Name, Subject: c.Subject,
			BodyHTML: c.BodyHTML, BodyText: c.BodyText,
		})
		now := time.Now()
		if err != nil {
			r.Status = "failed"
			r.Error = err.Error()
			c.FailedCount++
			c.LastError = err.Error()
		} else {
			r.Status = "sent"
			r.SentAt = &now
			c.SentCount++
		}
		_ = s.db.Save(&r).Error
		_ = s.db.Model(&c).Updates(map[string]interface{}{
			"sent_count": c.SentCount, "failed_count": c.FailedCount, "last_error": c.LastError,
		}).Error
		time.Sleep(delay)
	}

	var final models.MailBulkCampaign
	if s.db.First(&final, id).Error != nil {
		return
	}
	if final.Status == "cancelled" {
		return
	}
	var remain int64
	s.db.Model(&models.MailBulkRecipient{}).Where("campaign_id = ? AND status = ?", id, "pending").Count(&remain)
	now := time.Now()
	final.FinishedAt = &now
	if remain > 0 && final.Status != "cancelled" {
		final.Status = "completed"
	} else if final.FailedCount > 0 && final.SentCount == 0 {
		final.Status = "failed"
	} else {
		final.Status = "completed"
	}
	_ = s.db.Save(&final).Error
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
