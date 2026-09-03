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

// permissionCanonicalOrder is Crowbar's own guarded → trusted → full-auto
// ordering, least to most permissive — purely for PermissionLevels' own
// stable-order guarantee below, mirroring the identical ordering
// runner.canonicalPermissionLevels keeps on the OTHER side of the
// engine/agents package boundary (that package cannot reach this one's
// internal spec types, so it keeps its own copy of the same three names).
var permissionCanonicalOrder = []string{"guarded", "trusted", "full-auto"}

// PermissionLevels reports which of Crowbar's own level names this provider
// can actually reach — the descriptor's own permission_levels.levels map
// keys, and nothing clamped or inferred beyond them, in Crowbar's fixed
// least-to-most-permissive order (a map's own iteration order is random,
// which would jitter a rendered menu on every reload). A provider with no
// permission_levels block at all offers none.
func PermissionLevels(
	d *spec.Descriptor,
) []string {
	if d == nil || d.PermissionLevels == nil {
		return nil
	}
	out := make([]string, 0, len(d.PermissionLevels.Levels))
	for _, level := range permissionCanonicalOrder {
		if _, ok := d.PermissionLevels.Levels[level]; ok {
			out = append(out, level)
		}
	}
	return out
}

// PermissionVars is level's own named values (see PermissionLevelSpec's own
// doc comment) — nil for a level the provider doesn't declare, same as an
// absent block.
func PermissionVars(
	d *spec.Descriptor,
	level string,
) map[string]string {
	if d == nil || d.PermissionLevels == nil {
		return nil
	}
	return d.PermissionLevels.Levels[level].Vars
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
	if sel.PermissionLevel != "" && d.PermissionLevels != nil {
		if lvl, ok := d.PermissionLevels.Levels[sel.PermissionLevel]; ok {
			out = append(out, spec.CloneSteps(lvl.Apply)...)
		}
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
