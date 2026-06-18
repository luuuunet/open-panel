package appstore

import "github.com/luuuunet/owpanel/internal/services/modelcatalog"

func resolveCatalogEntry(id string) *modelcatalog.ModelCatalogEntry {
	return modelcatalog.ResolveEntry(id)
}
