package cache

import (
	"fmt"

	"github.com/luuuunet/owpanel/internal/models"
)

type PresetResult struct {
	Name         string `json:"name"`
	ConfigPatch  any    `json:"config_patch,omitempty"`
	RulesCreated int    `json:"rules_created"`
	Message      string `json:"message"`
}

type presetSpec struct {
	name   string
	config models.CacheConfig
	rules  []models.CacheRule
}

func (s *Service) ApplyPreset(name string) (*PresetResult, error) {
	spec, ok := presetByName(name)
	if !ok {
		return nil, fmt.Errorf("unknown preset: %s", name)
	}

	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	patch := spec.config
	patch.ID = cfg.ID
	patch.Scope = cfg.Scope
	if _, err := s.UpdateConfig(&patch); err != nil {
		return nil, err
	}

	created := 0
	for i := range spec.rules {
		rule := spec.rules[i]
		if _, err := s.CreateRule(&rule); err == nil {
			created++
		}
	}

	applyRes, err := s.Apply()
	if err != nil {
		return nil, err
	}

	return &PresetResult{
		Name:         spec.name,
		ConfigPatch:  patch,
		RulesCreated: created,
		Message:      applyRes.Message,
	}, nil
}

func presetByName(name string) (presetSpec, bool) {
	switch name {
	case "wordpress":
		return presetWordPress(), true
	case "laravel":
		return presetLaravel(), true
	case "static":
		return presetStatic(), true
	case "ecommerce":
		return presetEcommerce(), true
	default:
		return presetSpec{}, false
	}
}

func presetWordPress() presetSpec {
	return presetSpec{
		name: "WordPress",
		config: models.CacheConfig{
			Enabled:          true,
			HtmlTTLMinutes:   10,
			StaticTTLHours:   720,
			BypassCookies:    "wordpress_logged_in|wordpress_sec|wp-postpass|comment_author|PHPSESSID|woocommerce_items_in_cart|woocommerce_cart_hash",
			BypassPaths:      "/wp-admin|/wp-login|/xmlrpc.php|/wp-json/|/cart|/checkout|/my-account",
			StaleEnabled:     true,
			HonorOrigin:      false,
			CacheQueryString: false,
		},
		rules: []models.CacheRule{
			{Name: "WP Admin", Pattern: "/wp-admin|/wp-login", Action: "bypass", Priority: 10, Enabled: true},
			{Name: "WP REST API", Pattern: "^/wp-json/", Action: "bypass", Priority: 20, Enabled: true},
			{Name: "WP Static long cache", Pattern: "\\.(css|js|woff2?|jpg|jpeg|png|gif|webp|svg|ico)$", Action: "cache", TTLMinutes: 10080, Priority: 100, Enabled: true},
		},
	}
}

func presetLaravel() presetSpec {
	return presetSpec{
		name: "Laravel",
		config: models.CacheConfig{
			Enabled:          true,
			HtmlTTLMinutes:   5,
			StaticTTLHours:   168,
			BypassCookies:    "laravel_session|XSRF-TOKEN|PHPSESSID|session",
			BypassPaths:      "/admin|/login|/register|/api/|/horizon|/telescope",
			StaleEnabled:     true,
			HonorOrigin:      true,
			CacheQueryString: false,
		},
		rules: []models.CacheRule{
			{Name: "Laravel API", Pattern: "^/api/", Action: "bypass", Priority: 10, Enabled: true},
			{Name: "Laravel Auth", Pattern: "/login|/register|/password", Action: "bypass", Priority: 20, Enabled: true},
			{Name: "Laravel Admin", Pattern: "^/admin", Action: "bypass", Priority: 30, Enabled: true},
			{Name: "Mix/Vite assets", Pattern: "^/build/|\\.css$|\\.js$", Action: "cache", TTLMinutes: 10080, Priority: 100, Enabled: true},
		},
	}
}

func presetStatic() presetSpec {
	return presetSpec{
		name: "Static site",
		config: models.CacheConfig{
			Enabled:          true,
			HtmlTTLMinutes:   60,
			StaticTTLHours:   720,
			BypassCookies:    "session|auth",
			BypassPaths:      "/admin|/api/",
			StaleEnabled:     true,
			HonorOrigin:      false,
			CacheQueryString: false,
		},
		rules: []models.CacheRule{
			{Name: "Static assets", Pattern: "\\.(css|js|woff2?|jpg|jpeg|png|gif|webp|svg|ico|avif|mp4|pdf)$", Action: "cache", TTLMinutes: 43200, Priority: 50, Enabled: true},
		},
	}
}

func presetEcommerce() presetSpec {
	return presetSpec{
		name: "E-commerce",
		config: models.CacheConfig{
			Enabled:          true,
			HtmlTTLMinutes:   5,
			StaticTTLHours:   168,
			BypassCookies:    "session|cart|woocommerce_items_in_cart|woocommerce_cart_hash|PHPSESSID",
			BypassPaths:      "/cart|/checkout|/order|/payment|/account|/login|/register|/api/",
			StaleEnabled:     true,
			HonorOrigin:      false,
			CacheQueryString: true,
		},
		rules: []models.CacheRule{
			{Name: "Cart & checkout", Pattern: "/cart|/checkout|/payment|/order", Action: "bypass", Priority: 10, Enabled: true},
			{Name: "Account area", Pattern: "/account|/my-account|/profile", Action: "bypass", Priority: 20, Enabled: true},
			{Name: "Product images", Pattern: "\\.(jpg|jpeg|png|webp|gif)$", Action: "cache", TTLMinutes: 1440, Priority: 100, Enabled: true},
		},
	}
}
