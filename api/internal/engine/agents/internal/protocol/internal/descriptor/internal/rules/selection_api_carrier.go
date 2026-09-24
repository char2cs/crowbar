package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

type selectionAPICarrier struct{}

func (selectionAPICarrier) Name() string { return "selection_api_carrier" }

// Check refuses an api-transport descriptor whose model/effort choice is
// declared on the argv carrier alone.
//
// An api-transport spawn whose connection comes up forks NO process, so
// apply: renders into a plan nobody runs — the choice is dropped and the
// runner row records it as honoured, which makes the revert permanent
// (selectionRequiresRestart then compares equal forever). api_apply: is the
// same choice on the serve process's own config channel. Caught at load
// rather than at the third chat quietly running the wrong model.
func (selectionAPICarrier) Check(d *spec.Descriptor) error {
	if d.Runtime.Transport != "api" {
		return nil
	}
	if d.Model != nil && len(d.Model.APIApply) == 0 {
		return invalid(d.ID, "model.api_apply is empty: an api-transport spawn can fork no process for model.apply to ride")
	}
	if d.Effort != nil && len(d.Effort.APIApply) == 0 {
		return invalid(d.ID, "effort.api_apply is empty: an api-transport spawn can fork no process for effort.apply to ride")
	}
	return nil
}
