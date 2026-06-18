package aihub

import "github.com/luuuunet/owpanel/internal/services/modelcatalog"

type ModelCatalogEntry = modelcatalog.ModelCatalogEntry

func ModelCatalog() []ModelCatalogEntry {
	return modelcatalog.Catalog()
}

func DefaultModelIDs() []string {
	return modelcatalog.DefaultHFModelIDs()
}

func ResolveCatalogEntry(id string) *ModelCatalogEntry {
	return modelcatalog.ResolveEntry(id)
}

func HubTasks() []modelcatalog.HubTask {
	return modelcatalog.HubTasks()
}

func CatalogByModality(modality string) []ModelCatalogEntry {
	return modelcatalog.CatalogByModality(modality)
}
