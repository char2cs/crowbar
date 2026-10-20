// Package runner (file revive.go) is the server-owned revive: sending to a
// dormant chat brings its provider back on the chat's session, in the same
// hold of the chat as the delivery. The client only ever sends; it never
// orchestrates a revive of its own.
package runner

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// deliveryLaunch is the provider a send launches itself: the one staged with it
// when that is not the chat's own, else a dormant chat's. launches=false leaves
// a live chat's CLI to the ordinary delivery.
func (rs *Runners) deliveryLaunch(ctx context.Context, chat domain.Chat, staged string) (string, bool, error) {
	if staged != "" {
		current, err := rs.conversations.ChatProviderID(ctx, chat.ID)
		if err != nil || staged != current {
			return staged, err == nil, err
		}
	}
	if _, live := rs.live.byChat(chat.ID); live {
		return "", false, nil
	}
	providerID, err := rs.lastActiveProviderID(ctx, chat.ID)
	if err != nil {
		return "", false, fmt.Errorf("agent: submit prompt: revive: %w", err)
	}
	return providerID, true, nil
}

// launchForDelivery launches providerID carrying requestID and the pick staged
// with it, in place of the chat's CLI if it has one: the send's one launch.
func (rs *Runners) launchForDelivery(
	ctx context.Context, h *hold, chat domain.Chat, providerID, requestID, text string,
	selection *domain.ChatSelection,
) (domain.AgentPromptSubmission, error) {
	defer rs.enterPhase(ctx, chat.ID, rs.replacementPhase(chat.ID))()
	delivery, err := rs.planCarrying(ctx, chat, providerID)
	if err != nil {
		return domain.AgentPromptSubmission{}, err
	}
	delivery.selection = selection
	return rs.launchCarrying(ctx, h, chat, requestID, text, delivery)
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
