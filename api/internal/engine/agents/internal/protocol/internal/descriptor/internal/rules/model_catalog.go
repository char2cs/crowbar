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
	if split := d.Model.IDSplit; split != nil &&
		(split.Sep == "" || len(split.Names) != 2 || split.Names[0] == split.Names[1]) {
		return invalid(d.ID, "model.id_split needs a sep and two distinct names")
	}
	for i, id := range d.Model.Available {
		if id == "" {
			return invalid(d.ID, "model.available[%d] is empty", i)
		}
	}
	return nil
}
