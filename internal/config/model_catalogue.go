package config

import "slices"

// ModelCatalogueVersion changes whenever a reviewed entry changes. Catalogue
// metadata describes the documented provider API, not a proxy's serving model.
const ModelCatalogueVersion = "2026-09-09.1"

// CatalogueMetadata uses exact provider/model matches on the default endpoint.
// Custom endpoints and aggregator aliases need their own discovery/overrides.
func CatalogueMetadata(p ModelProfile) ProfileMetadata {
	if p.Provider != "openai" || p.Endpoint != "" {
		return ProfileMetadata{}
	}
	if !slices.Contains([]string{"gpt-4o", "gpt-4o-2024-08-06", "gpt-4o-2024-11-20"}, p.Model) {
		return ProfileMetadata{}
	}
	return ProfileMetadata{
		Verified:          true,
		Source:            "catalogue:" + ModelCatalogueVersion + ":https://developers.openai.com/api/docs/models/gpt-4o",
		AdvertisedContext: 128000,
		MaxOutput:         16384,
		InputTypes:        []string{"text", "image"},
	}
}
