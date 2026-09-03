package rules

import (
	"sort"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type effortCatalog struct{}

func (effortCatalog) Name() string { return "effort_catalog" }

func (effortCatalog) Check(d *spec.Descriptor) error {
	if d.Effort == nil {
		return nil
	}
	if len(d.Effort.Available) == 0 {
		return invalid(d.ID, "effort.available declares no models")
	}
	keys := make([]string, 0, len(d.Effort.Available))
	for model := range d.Effort.Available {
		keys = append(keys, model)
	}
	sort.Strings(keys)
	for _, model := range keys {
		if err := checkEffortLevels(d.ID, model, d.Effort.Available[model]); err != nil {
			return err
		}
	}
	return nil
}

func checkEffortLevels(id, model string, levels []string) error {
	if len(levels) == 0 {
		return invalid(id, "effort.available[%q] is empty", model)
	}
	for i, level := range levels {
		if level == "" {
			return invalid(id, "effort.available[%q][%d] is empty", model, i)
		}
	}
	return nil
}
