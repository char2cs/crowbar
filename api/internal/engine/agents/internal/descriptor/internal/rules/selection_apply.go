package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

type selectionApply struct{}

func (selectionApply) Name() string { return "selection_apply" }

// Check refuses a declared model:/effort: block with no apply steps.
//
// Such a block is the worst possible shape: it advertises a capability, fills a
// picker, accepts a choice and then delivers it nowhere — a chat that reports
// running under the model the user picked while the CLI runs under its default.
// A capability with no delivery is not a capability.
func (selectionApply) Check(d *spec.Descriptor) error {
	if d.Model != nil && len(d.Model.Apply) == 0 {
		return invalid(d.ID, "model.apply is empty")
	}
	if d.Effort != nil && len(d.Effort.Apply) == 0 {
		return invalid(d.ID, "effort.apply is empty")
	}
	return nil
}
