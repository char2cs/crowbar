package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

type modelCatalog struct{}

func (modelCatalog) Name() string { return "model_catalog" }

func (modelCatalog) Check(d *spec.Descriptor) error {
	if d.Model == nil {
		return nil
	}
	for i, id := range d.Model.Available {
		if id == "" {
			return invalid(d.ID, "model.available[%d] is empty", i)
		}
	}
	return nil
}
