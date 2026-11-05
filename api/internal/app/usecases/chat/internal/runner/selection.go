package runner

import engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"

// resolveSelectionForSpawn clears a stored model/effort THIS provider does not declare
// to "" (its default): after a switch the fields still carry the outgoing provider's ids.
// Never written back, so the chat's intent resolves fresh on every spawn.
func resolveSelectionForSpawn(
	descriptor engineagents.Agent,
	sel engineagents.Selection,
) engineagents.Selection {
	sel.PermissionLevel = resolvePermissionLevel(descriptor.PermissionLevels(), sel.PermissionLevel)
	discovered := descriptor.Capabilities().ModelDiscovery
	if sel.Model != "" && !engineagents.Allowed(descriptor.Models(), discovered, sel.Model) {
		sel.Model = ""
	}
	if sel.Effort != "" && !engineagents.Allowed(descriptor.Efforts(sel.Model), discovered, sel.Effort) {
		sel.Effort = ""
	}
	return sel
}
