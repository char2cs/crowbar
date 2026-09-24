package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

type modelCatalog struct{}

func (modelCatalog) Name() string { return "model_catalog" }

func (modelCatalog) Check(d *spec.Descriptor) error {
	if d.Model == nil {
		return nil
	}
	sources := 0
	if len(d.Model.Available) > 0 {
		sources++
	}
	if d.Model.Discover != nil {
		sources++
	}
	if d.Model.Manifest != nil {
		sources++
	}
	if sources > 1 {
		return invalid(d.ID, "model.available, model.discover, and model.manifest are mutually exclusive")
	}
	for i, id := range d.Model.Available {
		if id == "" {
			return invalid(d.ID, "model.available[%d] is empty", i)
		}
	}
	return nil
}
