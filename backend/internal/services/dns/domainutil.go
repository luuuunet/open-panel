package dns

import (
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

func normalizeHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimSuffix(host, ".")
	return host
}

func splitHostZone(host string) (name, zone string) {
	host = normalizeHost(host)
	parts := strings.Split(host, ".")
	if len(parts) <= 2 {
		return "@", host
	}
	return parts[0], strings.Join(parts[1:], ".")
}

func recordNameForProvider(provider, zoneName, host string) string {
	host = normalizeHost(host)
	zoneName = normalizeHost(zoneName)
	if host == zoneName {
		return "@"
	}
	suffix := "." + zoneName
	if strings.HasSuffix(host, suffix) {
		sub := strings.TrimSuffix(host, suffix)
		if sub == "" {
			return "@"
		}
		return sub
	}
	name, _ := splitHostZone(host)
	return name
}

func fqdnForProvider(provider, zoneName, recordName string) string {
	zoneName = normalizeHost(zoneName)
	if recordName == "@" || recordName == "" {
		return zoneName
	}
	if provider == "cloudflare" {
		return recordName + "." + zoneName
	}
	return recordName
}

func findZoneForHost(zones []models.DNSZone, host string) *models.DNSZone {
	host = normalizeHost(host)
	var best *models.DNSZone
	bestLen := -1
	for i := range zones {
		z := &zones[i]
		zn := normalizeHost(z.Name)
		if host == zn || strings.HasSuffix(host, "."+zn) {
			if len(zn) > bestLen {
				best = z
				bestLen = len(zn)
			}
		}
	}
	return best
}
