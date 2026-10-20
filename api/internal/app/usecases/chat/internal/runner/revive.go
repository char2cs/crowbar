// Package runner (file revive.go) is the server-owned revive: sending to a
// dormant chat brings its provider back on the chat's session, in the same
// hold of the chat as the delivery. The client only ever sends; it never
// orchestrates a revive of its own.
package runner

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// reviveForDelivery starts a dormant chat's provider carrying requestID: the
// one CLI the send launches is its carrier, so a refusal of the session settles
// it. handled=false leaves a live chat to the ordinary delivery.
func (rs *Runners) reviveForDelivery(
	ctx context.Context, h *hold, chat domain.Chat, requestID, text string,
) (domain.AgentPromptSubmission, bool, error) {
	if _, live := rs.live.byChat(chat.ID); live {
		return domain.AgentPromptSubmission{}, false, nil
	}
	providerID, err := rs.lastActiveProviderID(ctx, chat.ID)
	if err != nil {
		return domain.AgentPromptSubmission{}, true, fmt.Errorf("agent: submit prompt: revive: %w", err)
	}
	defer rs.enterPhase(ctx, chat.ID, snapshot.PhaseStarting)()
	submission, err := rs.startCarrying(ctx, h, chat, providerID, requestID, text)
	return submission, true, err
}

// startCarrying refuses a disabled provider or a working chat before the
// prompt is recorded, then launches providerID's CLI carrying it.
func (rs *Runners) startCarrying(
	ctx context.Context, h *hold, chat domain.Chat, providerID, requestID, text string,
) (domain.AgentPromptSubmission, error) {
	if err := rs.providers.RequireProviderEnabled(ctx, providerID); err != nil {
		return domain.AgentPromptSubmission{}, err
	}
	if err := rs.requireNotBusy(ctx, chat.ID); err != nil {
		return domain.AgentPromptSubmission{}, err
	}
	descriptor, _, err := rs.promptAgent(ctx, chat, providerID)
	if err != nil {
		return domain.AgentPromptSubmission{}, err
	}
	delivery, err := rs.resolvePromptDelivery(ctx, chat.ID, providerID, engineagents.Runner{}, descriptor)
	if err != nil {
		return domain.AgentPromptSubmission{}, err
	}
	return rs.launchCarrying(ctx, h, chat, requestID, text, delivery)
}
