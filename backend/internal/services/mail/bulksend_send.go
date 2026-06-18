package mail

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

func (s *Service) sendOutbound(provider *models.MailSendProvider, msg outboundMessage) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	cfg := parseBulkConfig(provider.ConfigJSON)
	switch provider.ProviderType {
	case "local":
		return s.sendLocal(msg)
	case "custom_smtp", "amazon_ses", "aliyun_dm", "tencent_ses":
		return sendViaSMTP(provider.ProviderType, cfg, msg)
	case "sendgrid":
		return sendSendGrid(cfg, msg)
	case "mailgun":
		return sendMailgun(cfg, msg)
	case "postmark":
		return sendPostmark(cfg, msg)
	case "brevo":
		return sendBrevo(cfg, msg)
	case "mailjet":
		return sendMailjet(cfg, msg)
	case "sparkpost":
		return sendSparkPost(cfg, msg)
	case "resend":
		return sendResend(cfg, msg)
	case "smtp2go":
		return sendSMTP2GO(cfg, msg)
	case "elastic_email":
		return sendElasticEmail(cfg, msg)
	default:
		return fmt.Errorf("unsupported provider: %s", provider.ProviderType)
	}
}

func (s *Service) sendLocal(msg outboundMessage) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("本机 Postfix 发送需在 Linux 上运行")
	}
	from := strings.TrimSpace(msg.From)
	to := strings.TrimSpace(msg.To)
	if from == "" || to == "" {
		return fmt.Errorf("发件人或收件人为空")
	}
	subject := msg.Subject
	if subject == "" {
		subject = "(no subject)"
	}
	body := msg.BodyText
	if body == "" {
		body = stripHTML(msg.BodyHTML)
	}
	var buf bytes.Buffer
	buf.WriteString("From: " + msg.fromHeader() + "\r\n")
	buf.WriteString("To: " + to + "\r\n")
	if rt := strings.TrimSpace(msg.ReplyTo); rt != "" {
		buf.WriteString("Reply-To: " + rt + "\r\n")
	}
	buf.WriteString("Subject: " + subject + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	if strings.TrimSpace(msg.BodyHTML) != "" {
		boundary := "owpanel-" + strconv.FormatInt(time.Now().UnixNano(), 10)
		buf.WriteString("Content-Type: multipart/alternative; boundary=" + boundary + "\r\n\r\n")
		buf.WriteString("--" + boundary + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n")
		buf.WriteString(body + "\r\n")
		buf.WriteString("--" + boundary + "\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n")
		buf.WriteString(msg.BodyHTML + "\r\n")
		buf.WriteString("--" + boundary + "--\r\n")
	} else {
		buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		buf.WriteString(body + "\r\n")
	}
	cmd := exec.Command("sendmail", "-f", from, to)
	cmd.Stdin = &buf
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sendmail: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func sendViaSMTP(providerType string, cfg bulkConfig, msg outboundMessage) error {
	host := cfg.get("smtp_host")
	port := cfg.get("smtp_port")
	user := cfg.get("smtp_user", "access_key", "secret_id")
	pass := cfg.get("smtp_password", "secret_key", "api_secret")
	useTLS := cfg.get("smtp_tls") != "0" && cfg.get("smtp_tls") != "false"

	switch providerType {
	case "amazon_ses":
		if host == "" {
			region := cfg.get("region")
			if region == "" {
				region = "us-east-1"
			}
			host = "email-smtp." + region + ".amazonaws.com"
		}
		if port == "" {
			port = "587"
		}
		if user == "" {
			user = cfg.get("access_key")
		}
		if pass == "" {
			pass = cfg.get("secret_key")
		}
	case "aliyun_dm":
		if host == "" {
			host = "smtpdm.aliyun.com"
		}
		if port == "" {
			port = "465"
		}
		useTLS = true
	case "tencent_ses":
		if host == "" {
			region := cfg.get("region")
			if region == "" {
				region = "ap-guangzhou"
			}
			host = "smtp." + region + ".tencentcloud.com"
		}
		if port == "" {
			port = "587"
		}
	}

	if host == "" || port == "" {
		return fmt.Errorf("SMTP 主机或端口未配置")
	}
	return smtpSend(host, port, user, pass, useTLS, msg)
}

func smtpSend(host, port, user, pass string, useTLS bool, msg outboundMessage) error {
	addr := net.JoinHostPort(host, port)
	body := buildMIMEBody(msg)
	auth := smtp.PlainAuth("", user, pass, host)
	if useTLS || port == "465" {
		return smtpSendTLS(addr, host, auth, msg.From, msg.To, body)
	}
	return smtp.SendMail(addr, auth, msg.From, []string{msg.To}, body)
}

func smtpSendTLS(addr, host string, auth smtp.Auth, from, to string, body []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if auth != nil && auth != (smtp.Auth)(nil) {
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return err
			}
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func buildMIMEBody(msg outboundMessage) []byte {
	subject := msg.Subject
	if subject == "" {
		subject = "(no subject)"
	}
	text := msg.BodyText
	if text == "" {
		text = stripHTML(msg.BodyHTML)
	}
	var buf bytes.Buffer
	buf.WriteString("From: " + msg.fromHeader() + "\r\n")
	buf.WriteString("To: " + msg.To + "\r\n")
	if rt := strings.TrimSpace(msg.ReplyTo); rt != "" {
		buf.WriteString("Reply-To: " + rt + "\r\n")
	}
	buf.WriteString("Subject: " + subject + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	if strings.TrimSpace(msg.BodyHTML) != "" {
		b := "opb-" + strconv.FormatInt(time.Now().UnixNano(), 10)
		buf.WriteString("Content-Type: multipart/alternative; boundary=" + b + "\r\n\r\n")
		buf.WriteString("--" + b + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + text + "\r\n")
		buf.WriteString("--" + b + "\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n" + msg.BodyHTML + "\r\n")
		buf.WriteString("--" + b + "--\r\n")
	} else {
		buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n" + text + "\r\n")
	}
	return buf.Bytes()
}

func stripHTML(s string) string {
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")
	var out strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			out.WriteRune(r)
		}
	}
	return strings.TrimSpace(out.String())
}

func httpPostJSON(url string, headers map[string]string, payload interface{}) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

func sendSendGrid(cfg bulkConfig, msg outboundMessage) error {
	key := cfg.get("api_key")
	if key == "" {
		return fmt.Errorf("SendGrid API Key 未配置")
	}
	payload := map[string]interface{}{
		"personalizations": []map[string]interface{}{{"to": []map[string]string{{"email": msg.To}}}},
		"from":             map[string]string{"email": msg.From, "name": msg.FromName},
		"subject":          msg.Subject,
		"content":          buildContentParts(msg),
	}
	if rt := strings.TrimSpace(msg.ReplyTo); rt != "" {
		payload["reply_to"] = map[string]string{"email": rt}
	}
	return httpPostJSON("https://api.sendgrid.com/v3/mail/send", map[string]string{"Authorization": "Bearer " + key}, payload)
}

func buildContentParts(msg outboundMessage) []map[string]string {
	var parts []map[string]string
	if t := strings.TrimSpace(msg.BodyText); t != "" {
		parts = append(parts, map[string]string{"type": "text/plain", "value": t})
	} else if msg.BodyHTML != "" {
		parts = append(parts, map[string]string{"type": "text/plain", "value": stripHTML(msg.BodyHTML)})
	}
	if h := strings.TrimSpace(msg.BodyHTML); h != "" {
		parts = append(parts, map[string]string{"type": "text/html", "value": h})
	}
	if len(parts) == 0 {
		parts = append(parts, map[string]string{"type": "text/plain", "value": " "})
	}
	return parts
}

func sendMailgun(cfg bulkConfig, msg outboundMessage) error {
	key := cfg.get("api_key")
	domain := cfg.get("domain")
	if key == "" || domain == "" {
		return fmt.Errorf("Mailgun API Key 或域名未配置")
	}
	base := "https://api.mailgun.net/v3"
	if strings.EqualFold(cfg.get("region"), "eu") {
		base = "https://api.eu.mailgun.net/v3"
	}
	form := urlValues(msg)
	req, _ := http.NewRequest(http.MethodPost, base+"/"+domain+"/messages", strings.NewReader(form))
	req.SetBasicAuth("api", key)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("Mailgun HTTP %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

func urlValues(msg outboundMessage) string {
	v := map[string]string{
		"from":    msg.fromHeader(),
		"to":      msg.To,
		"subject": msg.Subject,
	}
	if msg.BodyHTML != "" {
		v["html"] = msg.BodyHTML
	}
	if msg.BodyText != "" {
		v["text"] = msg.BodyText
	} else if msg.BodyHTML != "" {
		v["text"] = stripHTML(msg.BodyHTML)
	}
	if msg.ReplyTo != "" {
		v["h:Reply-To"] = msg.ReplyTo
	}
	var parts []string
	for k, val := range v {
		parts = append(parts, k+"="+strings.ReplaceAll(val, "&", "%26"))
	}
	return strings.Join(parts, "&")
}

func sendPostmark(cfg bulkConfig, msg outboundMessage) error {
	token := cfg.get("server_token")
	if token == "" {
		return fmt.Errorf("Postmark Server Token 未配置")
	}
	payload := map[string]interface{}{
		"From": msg.fromHeader(), "To": msg.To, "Subject": msg.Subject,
		"TextBody": msg.BodyText, "HtmlBody": msg.BodyHTML,
	}
	if msg.BodyText == "" && msg.BodyHTML != "" {
		payload["TextBody"] = stripHTML(msg.BodyHTML)
	}
	if msg.ReplyTo != "" {
		payload["ReplyTo"] = msg.ReplyTo
	}
	return httpPostJSON("https://api.postmarkapp.com/email", map[string]string{"X-Postmark-Server-Token": token}, payload)
}

func sendBrevo(cfg bulkConfig, msg outboundMessage) error {
	key := cfg.get("api_key")
	if key == "" {
		return fmt.Errorf("Brevo API Key 未配置")
	}
	payload := map[string]interface{}{
		"sender":      map[string]string{"email": msg.From, "name": msg.FromName},
		"to":          []map[string]string{{"email": msg.To}},
		"subject":     msg.Subject,
		"htmlContent": msg.BodyHTML,
		"textContent": msg.BodyText,
	}
	if msg.BodyText == "" && msg.BodyHTML != "" {
		payload["textContent"] = stripHTML(msg.BodyHTML)
	}
	if msg.ReplyTo != "" {
		payload["replyTo"] = map[string]string{"email": msg.ReplyTo}
	}
	return httpPostJSON("https://api.brevo.com/v3/smtp/email", map[string]string{"api-key": key}, payload)
}

func sendMailjet(cfg bulkConfig, msg outboundMessage) error {
	key := cfg.get("api_key")
	secret := cfg.get("api_secret")
	if key == "" || secret == "" {
		return fmt.Errorf("Mailjet API Key/Secret 未配置")
	}
	payload := map[string]interface{}{
		"Messages": []map[string]interface{}{{
			"From":     map[string]string{"Email": msg.From, "Name": msg.FromName},
			"To":       []map[string]string{{"Email": msg.To}},
			"Subject":  msg.Subject,
			"TextPart": msg.BodyText,
			"HTMLPart": msg.BodyHTML,
		}},
	}
	if msg.BodyText == "" && msg.BodyHTML != "" {
		payload["Messages"].([]map[string]interface{})[0]["TextPart"] = stripHTML(msg.BodyHTML)
	}
	auth := base64.StdEncoding.EncodeToString([]byte(key + ":" + secret))
	return httpPostJSON("https://api.mailjet.com/v3/send", map[string]string{"Authorization": "Basic " + auth}, payload)
}

func sendSparkPost(cfg bulkConfig, msg outboundMessage) error {
	key := cfg.get("api_key")
	if key == "" {
		return fmt.Errorf("SparkPost API Key 未配置")
	}
	content := map[string]string{"from": msg.fromHeader(), "subject": msg.Subject}
	if msg.BodyHTML != "" {
		content["html"] = msg.BodyHTML
	}
	if msg.BodyText != "" {
		content["text"] = msg.BodyText
	} else if msg.BodyHTML != "" {
		content["text"] = stripHTML(msg.BodyHTML)
	}
	payload := map[string]interface{}{
		"recipients": []map[string]string{{"address": msg.To}},
		"content":    content,
	}
	if msg.ReplyTo != "" {
		content["reply_to"] = msg.ReplyTo
	}
	return httpPostJSON("https://api.sparkpost.com/api/v1/transmissions", map[string]string{"Authorization": key}, payload)
}

func sendResend(cfg bulkConfig, msg outboundMessage) error {
	key := cfg.get("api_key")
	if key == "" {
		return fmt.Errorf("Resend API Key 未配置")
	}
	payload := map[string]interface{}{
		"from": msg.fromHeader(), "to": []string{msg.To}, "subject": msg.Subject,
		"html": msg.BodyHTML, "text": msg.BodyText,
	}
	if msg.BodyText == "" && msg.BodyHTML != "" {
		payload["text"] = stripHTML(msg.BodyHTML)
	}
	if msg.ReplyTo != "" {
		payload["reply_to"] = msg.ReplyTo
	}
	return httpPostJSON("https://api.resend.com/emails", map[string]string{"Authorization": "Bearer " + key}, payload)
}

func sendSMTP2GO(cfg bulkConfig, msg outboundMessage) error {
	key := cfg.get("api_key")
	if key == "" {
		return fmt.Errorf("SMTP2GO API Key 未配置")
	}
	payload := map[string]interface{}{
		"api_key": key,
		"sender":  msg.fromHeader(),
		"to":      []string{msg.To},
		"subject": msg.Subject,
	}
	if msg.BodyHTML != "" {
		payload["html_body"] = msg.BodyHTML
	}
	if msg.BodyText != "" {
		payload["text_body"] = msg.BodyText
	} else if msg.BodyHTML != "" {
		payload["text_body"] = stripHTML(msg.BodyHTML)
	}
	return httpPostJSON("https://api.smtp2go.com/v3/email/send", nil, payload)
}

func sendElasticEmail(cfg bulkConfig, msg outboundMessage) error {
	key := cfg.get("api_key")
	if key == "" {
		return fmt.Errorf("Elastic Email API Key 未配置")
	}
	form := urlValues(msg)
	form += "&apikey=" + key
	req, _ := http.NewRequest(http.MethodPost, "https://api.elasticemail.com/v2/email/send", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("Elastic Email HTTP %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}
