package appstore

import "github.com/luuuunet/owpanel/internal/models"

// MailStackActions wires the panel mail module into the software store mail-server app.
type MailStackActions interface {
	InstallStack() error
	UninstallStack() error
	EnsureConfigured() error
}

func (s *Service) SetMailStackActions(a MailStackActions) {
	s.mailStack = a
}

func (s *Service) SyncMailStackRecords(installed bool) {
	s.syncMailStackRecords(installed)
}

func (s *Service) syncMailStackRecords(installed bool) {
	app, err := s.Get("mail-server")
	if err != nil {
		return
	}
	if !installed {
		_ = s.db.Model(app).Updates(map[string]interface{}{
			"installed":     false,
			"status":        "stopped",
			"install_error": "",
		}).Error
		s.InvalidateLiveStatus("mail-server")
		return
	}
	postfix := s.detectAppStatus("postfix")
	dovecot := s.detectAppStatus("dovecot")
	status := "stopped"
	if postfix == "running" && dovecot == "running" {
		status = "running"
	} else if postfix != "stopped" || dovecot != "stopped" {
		status = "stopped"
	}
	_ = s.db.Model(app).Updates(map[string]interface{}{
		"installed":     true,
		"status":        status,
		"install_error": "",
	}).Error
	s.InvalidateLiveStatus("mail-server")
}

func catalogOnlyKeys() map[string]struct{} {
	keys := make(map[string]struct{})
	for _, item := range mergedCatalog() {
		keys[item.Key] = struct{}{}
	}
	return keys
}

func filterCatalogApps(apps []models.App) []models.App {
	allowed := catalogOnlyKeys()
	out := make([]models.App, 0, len(apps))
	for _, app := range apps {
		if _, ok := allowed[app.Key]; ok {
			out = append(out, app)
		}
	}
	return out
}
