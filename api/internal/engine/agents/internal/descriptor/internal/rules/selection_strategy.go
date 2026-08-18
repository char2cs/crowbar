package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

type selectionStrategy struct{}

func (selectionStrategy) Name() string { return "selection_strategy" }

// Check admits exactly one strategy on the model:/effort: blocks — restart_tui,
// the strategy that says a changed choice takes effect on the next process.
//
// Nothing else is accepted, and in particular no live-switching strategy is,
// because neither CLI offers a channel that could change a running process's
// model and Crowbar therefore has no way to test one. A descriptor is
// user-overridable, so an unrecognised value must fail loudly here rather than
// be ignored into a chat whose picker silently does nothing.
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
