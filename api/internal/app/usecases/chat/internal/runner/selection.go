package runner

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// resolveSelectionForSpawn clears a stored model/effort THIS provider does not declare
// to "" (its default): a discovered catalogue can drop a model the chat still holds.
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

// movedPick is an unstaged launch's pick: one moving the chat onto another provider
// takes what it last ran as here, else its default, never the outgoing one's model.
func (rs *Runners) movedPick(
	ctx context.Context, chat domain.Chat, providerID string, staged *domain.ChatSelection,
) (*domain.ChatSelection, error) {
	if staged != nil || chat.ProviderID == "" || chat.ProviderID == providerID {
		return staged, nil
	}
	st, err := rs.ledger.State(ctx, chat.ID)
	if err != nil {
		return nil, fmt.Errorf("agent: spawn runner: last launch: %w", err)
	}
	last := st.LastLaunch(providerID)
	return &domain.ChatSelection{Model: last.LaunchModel, Effort: last.LaunchEffort}, nil
}
