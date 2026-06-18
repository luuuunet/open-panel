package edgeworker

import (
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

// AvailableDomain is a hostname that can be bound to an edge worker.
type AvailableDomain struct {
	Domain    string `json:"domain"`
	WebsiteID uint   `json:"website_id"`
	IsPrimary bool   `json:"is_primary"`
}

// WorkerDTO extends EdgeWorker with a parsed domains_list for API responses.
type WorkerDTO struct {
	models.EdgeWorker
	DomainsList []string `json:"domains_list"`
}

// ParseDomains splits a worker's comma-separated Domains field into hostnames.
func ParseDomains(w *models.EdgeWorker) []string {
	if w == nil {
		return nil
	}
	raw := strings.TrimSpace(w.Domains)
	if raw == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == ','
	}) {
		d := normalizeHostname(part)
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

func normalizeHostname(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimPrefix(raw, "https://")
	if idx := strings.Index(raw, "/"); idx >= 0 {
		raw = raw[:idx]
	}
	if raw == "" {
		return ""
	}
	// Strip port if present (host:port)
	if strings.Count(raw, ":") == 1 {
		if host, _, ok := strings.Cut(raw, ":"); ok {
			raw = host
		}
	}
	return raw
}

// IsAllDomains reports whether the worker applies to every hostname.
func IsAllDomains(domains []string) bool {
	for _, d := range domains {
		if d == "*" {
			return true
		}
	}
	return len(domains) == 0 && false // empty is not wildcard; resolved separately
}

// DomainsForWebsite returns primary domain plus aliases for a site.
func DomainsForWebsite(db *gorm.DB, siteID uint) ([]string, error) {
	var site models.Website
	if err := db.Preload("Aliases").First(&site, siteID).Error; err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	if d := normalizeHostname(site.Domain); d != "" && !seen[d] {
		seen[d] = true
		out = append(out, d)
	}
	for _, a := range site.Aliases {
		if d := normalizeHostname(a.Domain); d != "" && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out, nil
}

// ResolveSiteIDs maps a worker's bound domains to website IDs (primary + aliases).
func ResolveSiteIDs(db *gorm.DB, w *models.EdgeWorker) []uint {
	domains := ParseDomains(w)
	if IsAllDomains(domains) {
		return nil
	}
	if len(domains) == 0 && w.WebsiteID > 0 {
		return []uint{w.WebsiteID}
	}
	hostToSite := buildHostIndex(db)
	seen := map[uint]bool{}
	var ids []uint
	for _, d := range domains {
		if id, ok := hostToSite[d]; ok && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if w.WebsiteID > 0 && !seen[w.WebsiteID] {
		ids = append(ids, w.WebsiteID)
	}
	return ids
}

func buildHostIndex(db *gorm.DB) map[string]uint {
	idx := map[string]uint{}
	var sites []models.Website
	_ = db.Preload("Aliases").Find(&sites).Error
	for _, site := range sites {
		if d := normalizeHostname(site.Domain); d != "" {
			idx[d] = site.ID
		}
		for _, a := range site.Aliases {
			if d := normalizeHostname(a.Domain); d != "" {
				idx[d] = site.ID
			}
		}
	}
	return idx
}

// ListAvailableDomains returns all bindable hostnames from websites.
func ListAvailableDomains(db *gorm.DB) ([]AvailableDomain, error) {
	var sites []models.Website
	if err := db.Preload("Aliases").Order("id asc").Find(&sites).Error; err != nil {
		return nil, err
	}
	var out []AvailableDomain
	for _, site := range sites {
		if d := normalizeHostname(site.Domain); d != "" {
			out = append(out, AvailableDomain{Domain: d, WebsiteID: site.ID, IsPrimary: true})
		}
		for _, a := range site.Aliases {
			if d := normalizeHostname(a.Domain); d != "" {
				out = append(out, AvailableDomain{Domain: d, WebsiteID: site.ID, IsPrimary: false})
			}
		}
	}
	return out, nil
}

// WorkersForSite returns enabled workers that apply to the given site hostnames.
func WorkersForSite(workers []models.EdgeWorker, siteID uint, siteDomain string, aliasDomains []string) []models.EdgeWorker {
	siteHosts := map[string]bool{}
	if d := normalizeHostname(siteDomain); d != "" {
		siteHosts[d] = true
	}
	for _, a := range aliasDomains {
		if d := normalizeHostname(a); d != "" {
			siteHosts[d] = true
		}
	}

	var out []models.EdgeWorker
	for _, w := range workers {
		if !w.Enabled {
			continue
		}
		domains := ParseDomains(&w)
		if IsAllDomains(domains) {
			out = append(out, w)
			continue
		}
		if w.WebsiteID > 0 && w.WebsiteID == siteID && len(domains) == 0 {
			out = append(out, w)
			continue
		}
		for _, d := range domains {
			if siteHosts[d] {
				out = append(out, w)
				break
			}
		}
	}
	return out
}

func (s *Service) EnrichWorker(w *models.EdgeWorker) WorkerDTO {
	return WorkerDTO{
		EdgeWorker:  *w,
		DomainsList: ParseDomains(w),
	}
}

func (s *Service) EnrichWorkers(list []models.EdgeWorker) []WorkerDTO {
	out := make([]WorkerDTO, len(list))
	for i := range list {
		out[i] = s.EnrichWorker(&list[i])
	}
	return out
}

func (s *Service) ListAvailableDomains() ([]AvailableDomain, error) {
	return ListAvailableDomains(s.db)
}

func (s *Service) prepareDomains(w *models.EdgeWorker) error {
	domains := ParseDomains(w)
	if len(domains) == 0 && w.WebsiteID > 0 {
		auto, err := DomainsForWebsite(s.db, w.WebsiteID)
		if err != nil {
			return err
		}
		if len(auto) == 0 {
			return fmt.Errorf("website has no domains")
		}
		w.Domains = strings.Join(auto, ",")
		domains = auto
	}
	if len(domains) == 0 {
		return fmt.Errorf("at least one domain is required (or select a website to auto-fill)")
	}
	w.Domains = strings.Join(domains, ",")
	return nil
}

// buildSiteWorkersMap groups enabled workers into per-site snippets and global wildcard workers.
func (s *Service) buildSiteWorkersMap(workers []models.EdgeWorker) (map[uint][]models.EdgeWorker, []models.EdgeWorker) {
	siteMap := map[uint][]models.EdgeWorker{}
	var global []models.EdgeWorker

	for _, w := range workers {
		domains := ParseDomains(&w)
		if IsAllDomains(domains) {
			global = append(global, w)
			continue
		}
		siteIDs := ResolveSiteIDs(s.db, &w)
		if len(siteIDs) == 0 {
			continue
		}
		for _, id := range siteIDs {
			siteMap[id] = append(siteMap[id], w)
		}
	}
	return siteMap, global
}

func hostGuardLua(domains []string, edgeAlreadyRequired bool) string {
	if len(domains) == 0 || IsAllDomains(domains) {
		return ""
	}
	var quoted []string
	for _, d := range domains {
		quoted = append(quoted, fmt.Sprintf("%q", d))
	}
	prelude := ""
	if !edgeAlreadyRequired {
		prelude = "local edge = require \"edge_runtime\"\n"
	}
	return prelude + fmt.Sprintf("if not edge.match_host(ngx.var.host, {%s}) then return end\n", strings.Join(quoted, ", "))
}
