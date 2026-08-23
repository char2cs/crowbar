package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

type selectionStrategy struct{}

func (selectionStrategy) Name() string { return "selection_strategy" }

func (selectionStrategy) Check(d *spec.Descriptor) error {
	if d.Model != nil && d.Model.Strategy != spec.DeliveryRestartTUI {
		return invalid(d.ID, "model.strategy must be %q, got %q",
			spec.DeliveryRestartTUI, d.Model.Strategy)
	}
	if d.Effort != nil && d.Effort.Strategy != spec.DeliveryRestartTUI {
		return invalid(d.ID, "effort.strategy must be %q, got %q",
			spec.DeliveryRestartTUI, d.Effort.Strategy)
	}
	return nil
}
