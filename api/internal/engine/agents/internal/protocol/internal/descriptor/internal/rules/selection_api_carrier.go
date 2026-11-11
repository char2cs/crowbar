package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

type selectionAPICarrier struct{}

func (selectionAPICarrier) Name() string { return "selection_api_carrier" }

// Check refuses an api-transport descriptor whose model/effort is declared only on
// the argv carrier: such a spawn forks no process, so apply: would be dropped while
// the runner records it as honoured. api_apply: carries it over the connection.
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
