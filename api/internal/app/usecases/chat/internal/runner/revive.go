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
	delivery, err := rs.planCarrying(ctx, chat, providerID)
	if err != nil {
		return domain.AgentPromptSubmission{}, true, err
	}
	submission, err := rs.launchCarrying(ctx, h, chat, requestID, text, delivery)
	return submission, true, err
}

// planCarrying refuses a disabled provider or a working chat before anything
// is recorded, then plans providerID's launch in place of the chat's CLI, if any.
func (rs *Runners) planCarrying(ctx context.Context, chat domain.Chat, providerID string) (promptDelivery, error) {
	if err := rs.providers.RequireProviderEnabled(ctx, providerID); err != nil {
		return promptDelivery{}, err
	}
	if err := rs.requireNotBusy(ctx, chat.ID); err != nil {
		return promptDelivery{}, err
	}
	descriptor, _, err := rs.promptAgent(ctx, chat, providerID)
	if err != nil {
		return promptDelivery{}, err
	}
	live, _ := rs.live.byChat(chat.ID)
	return rs.resolvePromptDelivery(ctx, chat.ID, providerID, live, descriptor)
}
