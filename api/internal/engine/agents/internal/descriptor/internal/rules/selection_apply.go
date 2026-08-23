package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

type selectionApply struct{}

func (selectionApply) Name() string { return "selection_apply" }

func (selectionApply) Check(d *spec.Descriptor) error {
	if d.Model != nil && len(d.Model.Apply) == 0 {
		return invalid(d.ID, "model.apply is empty")
	}
	if d.Effort != nil && len(d.Effort.Apply) == 0 {
		return invalid(d.ID, "effort.apply is empty")
	}
	return nil
}
