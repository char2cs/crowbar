package selection

import (
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func Models(
	d *spec.Descriptor,
) []string {
	if d == nil || d.Model == nil {
		return nil
	}
	return append([]string(nil), d.Model.Available...)
}

func Efforts(
	d *spec.Descriptor,
	model string,
) []string {
	if d == nil || d.Effort == nil {
		return nil
	}
	if levels, ok := d.Effort.Available[model]; ok {
		return append([]string(nil), levels...)
	}
	return append([]string(nil), d.Effort.Available[spec.EffortFallbackKey]...)
}

func Steps(
	d *spec.Descriptor,
	sel models.Selection,
) []spec.InjectStep {
	if d == nil {
		return nil
	}
	var out []spec.InjectStep
	if sel.Model != "" && d.Model != nil {
		out = append(out, spec.CloneSteps(d.Model.Apply)...)
	}
	if sel.Effort != "" && d.Effort != nil {
		out = append(out, spec.CloneSteps(d.Effort.Apply)...)
	}
	return out
}

func RestartRequired(
	d *spec.Descriptor,
	launched models.Selection,
	desired models.Selection,
) bool {
	if d == nil {
		return false
	}
	if launched.Model != desired.Model && d.Model != nil && d.Model.Strategy == spec.DeliveryRestartTUI {
		return true
	}
	return launched.Effort != desired.Effort && d.Effort != nil &&
		d.Effort.Strategy == spec.DeliveryRestartTUI
}
