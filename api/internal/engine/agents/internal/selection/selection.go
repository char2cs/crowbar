// Package selection answers what a chat may choose to run a CLI as, and what
// honouring a choice costs.
//
// Every function here is pure: it reads a descriptor and a Selection and returns
// a value. Nothing consults the running process, and nothing may — the CLIs
// expose no readable notion of "the model I am running right now", so the only
// truthful answer to that question is what Crowbar recorded when it spawned
// them.
package selection

import (
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// Models lists the model ids the descriptor declares as selectable, or nil when
// it declares none. The copy is defensive: a caller handed the descriptor's own
// slice could reorder a shared, cached descriptor for every later chat.
func Models(
	d *spec.Descriptor,
) []string {
	if d == nil || d.Model == nil {
		return nil
	}
	return append([]string(nil), d.Model.Available...)
}

// Efforts lists the reasoning-effort levels declared for a model id.
//
// A model with no list of its own falls back to the catalogue's
// spec.EffortFallbackKey entry, which is how a descriptor whose levels do not
// vary by model declares them once. The empty model id — the provider's own
// default, which Crowbar deliberately does not resolve to a name — takes the
// same fallback.
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

// Steps renders the injection steps that carry a selection into the process.
//
// It contributes NOTHING for an empty field, and nothing for a field whose block
// the descriptor does not declare. That is the property the whole feature rests
// on: a chat that has chosen nothing produces byte-identical argv to one spawned
// before this package existed, so the inert path cannot regress a spawn.
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

// RestartRequired reports whether moving a process from the selection it was
// LAUNCHED with to the one a chat now wants obliges Crowbar to replace it.
//
// launched is what Crowbar recorded at spawn, never what the CLI believes. The
// difference matters: a user who changes the model inside the TUI has changed
// something Crowbar cannot see, and asking the process would mean trusting a
// channel that does not exist.
//
// The block's own `strategy` is what authorises the restart, and it is read
// rather than assumed. Today validation admits only restart_tui, so the check
// looks redundant — it is the seam a live-switching strategy would arrive
// through, and reading it now is what stops that arrival from being a rewrite of
// every caller.
//
// Empty differs from non-empty in BOTH directions on purpose: a first choice and
// a clear back to the provider default are equally a change, and both need a
// process that was started with the new argv.
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
