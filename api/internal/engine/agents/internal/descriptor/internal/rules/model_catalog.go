package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

type modelCatalog struct{}

func (modelCatalog) Name() string { return "model_catalog" }

// Check holds every declared model id to being a real id.
//
// An empty entry would render as a blank row in the picker and, if chosen, as an
// empty argv value behind the model flag — where the next token silently becomes
// the flag's value. The catalogue itself may legitimately be empty (a descriptor
// declaring the block with nothing in it yet); an empty ENTRY never can be.
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
