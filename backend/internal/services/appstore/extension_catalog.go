package appstore

import (
	"github.com/luuuunet/owpanel/internal/models"
)

var extensionCatalogLoader func() []models.App

// SetExtensionCatalogLoader registers a callback that returns apps from panel extensions.
func SetExtensionCatalogLoader(fn func() []models.App) {
	extensionCatalogLoader = fn
}

func extensionCatalogItems() []catalogItem {
	if extensionCatalogLoader == nil {
		return nil
	}
	apps := extensionCatalogLoader()
	if len(apps) == 0 {
		return nil
	}
	static := catalogKeySet()
	var out []catalogItem
	for _, app := range apps {
		if _, ok := static[app.Key]; ok {
			continue
		}
		out = append(out, catalogItem{
			App:           app,
			defaultConfig: map[string]interface{}{"source": "extension"},
		})
	}
	return out
}

func appendExtensionCatalog(out []catalogItem) []catalogItem {
	items := extensionCatalogItems()
	if len(items) == 0 {
		return out
	}
	seen := make(map[string]struct{}, len(out))
	for _, item := range out {
		seen[item.Key] = struct{}{}
	}
	for _, item := range items {
		if _, ok := seen[item.Key]; ok {
			continue
		}
		out = append(out, item)
	}
	return out
}
