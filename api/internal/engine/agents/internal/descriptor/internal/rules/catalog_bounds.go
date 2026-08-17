package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

type catalogBounds struct{}

func (catalogBounds) Name() string { return "catalog_bounds" }

// Check holds every declared bound to the engine's ceiling. The ceilings are the
// point: a descriptor is user-overridable, so it may ask for LESS exposure to a
// provider command and never for more.
func (catalogBounds) Check(d *spec.Descriptor) error {
	sc := d.Presentation.SlashCatalog
	if sc == nil {
		return nil
	}
	switch sc.Completeness {
	case spec.CatalogCompletenessComplete,
		spec.CatalogCompletenessModelVisible,
		spec.CatalogCompletenessPluginOnly:
	default:
		return invalid(d.ID,
			"presentation.slash_catalog has unsupported completeness %q", sc.Completeness)
	}
	bounds := []struct {
		name  string
		value int
		max   int
	}{
		{"timeout_ms", sc.TimeoutMS, spec.MaxCatalogTimeoutMS},
		{"max_stdout_bytes", sc.MaxStdoutBytes, spec.MaxCatalogStdoutBytes},
		{"max_stderr_bytes", sc.MaxStderrBytes, spec.MaxCatalogStderrBytes},
		{"max_items", sc.MaxItems, spec.MaxCatalogItems},
		{"pipeline.detail_concurrency", sc.Pipeline.DetailConcurrency, spec.MaxCatalogDetailConcurrency},
	}
	for _, b := range bounds {
		if b.value < 0 || b.value > b.max {
			return invalid(d.ID,
				"presentation.slash_catalog.%s must be between 0 and %d", b.name, b.max)
		}
	}
	return nil
}
