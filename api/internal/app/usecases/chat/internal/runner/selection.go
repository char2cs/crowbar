package runner

import engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"

// resolveSelectionForSpawn clamps a chat's stored model/effort to what THIS
// provider actually declares — the same reasoning as resolvePermissionLevel
// (permission.go), and for the same reason ChatSelection never validates
// them itself: its own doc comment says minting reads Model/Effort as "the
// provider's own default", but a provider SWITCH is not a mint — it reads
// the chat's raw stored fields, which still carry the OUTGOING provider's
// choice. Left unclamped, that model string reaches this provider's spawn
// argv verbatim: confirmed live, switching a chat from codex (model
// gpt-5.4-mini) to claude spawned claude with --model gpt-5.4-mini, which
// claude rejects outright as model_not_found — while the switch's OWN
// divider claims the new selection is the target provider's default,
// because that marker is recorded from a separate, later call that never
// reaches the CLI already spawned (and already dead) by then.
//
// Clearing to "" reads as "this provider's own default", exactly like a
// fresh mint — and, like PermissionLevel, is never written back: the
// chat's own stored intent is untouched, so switching providers again
// resolves fresh each time.
func resolveSelectionForSpawn(
	descriptor engineagents.Agent,
	sel engineagents.Selection,
) engineagents.Selection {
	sel.PermissionLevel = resolvePermissionLevel(descriptor.PermissionLevels(), sel.PermissionLevel)
	if sel.Model != "" && !containsLevel(descriptor.Models(), sel.Model) {
		sel.Model = ""
	}
	if sel.Effort != "" && !containsLevel(descriptor.Efforts(sel.Model), sel.Effort) {
		sel.Effort = ""
	}
	return sel
}
