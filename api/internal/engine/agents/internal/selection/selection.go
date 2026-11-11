package selection

import (
	"maps"

	"github.com/char2cs/crowbar/api/internal/domain"
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

// PermissionLevels reports the Crowbar level names this provider declares in
// permission_levels.levels, nothing inferred, in Crowbar's fixed least-to-most
// permissive order so a menu never jitters with map iteration.
func PermissionLevels(
	d *spec.Descriptor,
) []string {
	if d == nil || d.PermissionLevels == nil {
		return nil
	}
	out := make([]string, 0, len(d.PermissionLevels.Levels))
	for _, level := range domain.PermissionLevels() {
		if _, ok := d.PermissionLevels.Levels[level]; ok {
			out = append(out, level)
		}
	}
	return out
}

// PermissionVars is level's own named values (see PermissionLevelSpec's own
// doc comment), with its after-leave values over them when left — nil for a
// level the provider doesn't declare, same as an absent block.
func PermissionVars(
	d *spec.Descriptor,
	level string,
	left bool,
) map[string]string {
	if d == nil || d.PermissionLevels == nil {
		return nil
	}
	l := d.PermissionLevels.Levels[level]
	if !left || len(l.VarsAfterLeave) == 0 {
		return l.Vars
	}
	out := maps.Clone(l.Vars)
	if out == nil {
		out = map[string]string{}
	}
	maps.Copy(out, l.VarsAfterLeave)
	return out
}

// Steps is sel on the argv of a process the spawn FORKS.
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
	if sel.PermissionLevel != "" && d.PermissionLevels != nil {
		if lvl, ok := d.PermissionLevels.Levels[sel.PermissionLevel]; ok {
			out = append(out, spec.CloneSteps(lvl.Apply)...)
		}
	}
	return out
}

// APISteps is sel on the api channel's `serve` argv, used instead of Steps when a
// spawn forks no process. Permission levels deliberately add nothing: their api
// carrier is vars:, rendered into the establish call's send: tree.
func APISteps(
	d *spec.Descriptor,
	sel models.Selection,
) []spec.InjectStep {
	if d == nil {
		return nil
	}
	var out []spec.InjectStep
	if sel.Model != "" && d.Model != nil {
		out = append(out, spec.CloneSteps(d.Model.APIApply)...)
	}
	if sel.Effort != "" && d.Effort != nil {
		out = append(out, spec.CloneSteps(d.Effort.APIApply)...)
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
	if launched.Effort != desired.Effort && d.Effort != nil &&
		d.Effort.Strategy == spec.DeliveryRestartTUI {
		return true
	}
	return launched.PermissionLevel != desired.PermissionLevel && d.PermissionLevels != nil &&
		d.PermissionLevels.Strategy == spec.DeliveryRestartTUI
}
