package provider

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed provider_catalog.json
var embeddedRemoteCatalogData []byte

var (
	embeddedRemoteCatalogOnce   sync.Once
	embeddedRemoteCatalogModels []ModelInfo
)

func registerGeneratedCatalog(c *ModelCatalog) {
	for _, model := range loadEmbeddedRemoteCatalogModels() {
		c.Register(model)
	}
}

func loadEmbeddedRemoteCatalogModels() []ModelInfo {
	embeddedRemoteCatalogOnce.Do(func() {
		if len(embeddedRemoteCatalogData) == 0 {
			embeddedRemoteCatalogModels = []ModelInfo{}
			return
		}

		var models []ModelInfo
		if err := json.Unmarshal(embeddedRemoteCatalogData, &models); err != nil {
			panic("provider: decode embedded provider_catalog.json: " + err.Error())
		}
		embeddedRemoteCatalogModels = models
	})
	return embeddedRemoteCatalogModels
}
