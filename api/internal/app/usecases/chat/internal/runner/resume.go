package runner

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// ResumeChat revives a dormant chat's last provider. Split out of lifecycle.go
// (responsibility, not size) once lastActiveProviderID's own fix landed there.
func (rs *Runners) ResumeChat(
	ctx context.Context,
	chatID string,
) (string, error) {
	defer rs.spawns.Lock(chatID)()

	live, err := rs.runnerStore.LiveRunnerForChat(ctx, chatID)
	if err == nil {
		return live.ID, nil
	}
	if !errors.Is(err, agentrunner.ErrNotFound) {
		return "", fmt.Errorf("agent: resume chat: live runner: %w", err)
	}
	providerID, err := rs.lastActiveProviderID(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: resume chat: %w", err)
	}
	// The gate is already held: call the inner body, never SwitchProvider itself.
	return rs.switchProviderLocked(ctx, chatID, providerID)
}

// lastActiveProviderID answers "who was really running here" for a dormant chat —
// what Resume brings back. See agents.ActiveProviderID for why LastConversation
// alone is blind to a provider that binds via its own connection identity rather
// than firing a session-bind (it writes no conversation row at all), and why the
// chat's durable interruption ledger is the second source that closes the gap.
//
// PLACEMENT history is the third, and it is what makes the answer complete. A
// placement is written whenever Crowbar points a runner at a chat — every runner,
// before any provider has announced anything — so it still names the vendor of a
// chat that was BORN on a provider that announces nothing and was never switched.
// That chat has no conversation row and no switch marker, and if it predates the
// chat's own durable vendor field it has no stored provider either: every resume
// it was ever asked for was refused, and the pane answered that refusal by
// starting the first enabled provider on the chat instead, converting it.
//
// The chat's OWN durable vendor is the last source. It is written at BIRTH now
// (Conversations.MintChat), so a chat created from here on carries it whether or
// not a CLI ever starts; placement history is what answers for every chat that
// already existed.
//
// A chat with none of the four has genuinely never had a CLI placed on it, and
// that is reported as ErrChatProviderUnknown — never as a missing RUNNER, which
// is both expected here and not what failed.
func (rs *Runners) lastActiveProviderID(ctx context.Context, chatID string) (string, error) {
	var conversations []agents.ChatConversation
	if last, lerr := rs.runnerStore.LastConversation(ctx, chatID); lerr == nil {
		conversations = []agents.ChatConversation{last}
	}

	interruptions, err := rs.activity.Interruptions(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("interruptions: %w", err)
	}

	placements, err := rs.runnerStore.PlacementsForChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("placements: %w", err)
	}

	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("chat: %w", err)
	}

	providerID, found := agents.ResolveProviderID(conversations, interruptions, placements, chat.ProviderID)
	if !found {
		return "", ErrChatProviderUnknown
	}
	return providerID, nil
}
