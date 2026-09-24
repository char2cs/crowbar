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
		return "", fmt.Errorf("agent: resume chat: no conversation to resume: %w", err)
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
// The chat's OWN durable vendor is the third and last, and it is what makes a
// reopen safe: a chat BORN on such a provider has neither of the other two, so
// this refused every resume it was ever asked for — and the pane answered that
// refusal by starting the first enabled provider on the chat instead, converting
// it. A chat minted before that field still refuses, which is honest: there is
// genuinely nothing left that knows.
func (rs *Runners) lastActiveProviderID(ctx context.Context, chatID string) (string, error) {
	var conversations []agents.ChatConversation
	if last, lerr := rs.runnerStore.LastConversation(ctx, chatID); lerr == nil {
		conversations = []agents.ChatConversation{last}
	}

	interruptions, err := rs.activity.Interruptions(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("interruptions: %w", err)
	}

	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("chat: %w", err)
	}

	providerID, found := agents.ResolveProviderID(conversations, interruptions, chat.ProviderID)
	if !found {
		return "", agentrunner.ErrNotFound
	}
	return providerID, nil
}
