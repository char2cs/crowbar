package conversation

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// providerID is the vendor the chat is BORN on, written durably here rather than
// waiting for the spawn that follows. It is the difference between a chat that
// can always say what it runs and one that can only say so once a CLI has
// actually started on it: the spawn-time SetProvider (runner.recordChatProvider)
// is best-effort and runs after a fork that may never happen, so a create that
// failed at the spawn — or predated that write entirely — left the field empty
// forever, and a dormant chat with no conversation row and no switch marker then
// had nothing anywhere that named its provider. Writing it at birth closes that
// at the source; the spawn-time write remains correct, and is what records a
// later provider SWITCH.
//
// "" is the honest value for a chat minted with no vendor in mind at all — an
// owning/placeholder row, which never gets a CLI.
//
// surface is the VIEW this chat is born on (design spec 2.5) — "" for the
// provider's own default landing. See domain.Chat.Surface for why it is
// durable rather than a one-shot spawn argument.
//
// chatID is the id the caller is already drawing the row under, or "" to mint
// one; a taken id is the chat aggregate's IDConflict.
func (c *Conversations) MintChat(
	ctx context.Context,
	chatID string,
	workspaceID string,
	providerID string,
	surface string,
) (string, error) {
	if chatID == "" {
		chatID = uuid.NewString()
	}
	_, err := c.chats.Create(ctx, agentchat.CreateInput{
		ID:          chatID,
		WorkspaceID: workspaceID,
		Type:        domain.ChatTypeChat,
		ProviderID:  providerID,
		Surface:     surface,
		Now:         time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("agent: mint chat: %w", err)
	}
	c.SeedPermissionLevel(ctx, chatID)
	return chatID, nil
}

// SeedPermissionLevel durably writes the CURRENT global default onto a
// freshly created chat — the RAW, provider-blind choice, never a
// provider-clamped one, and NON-explicit: it seeds a display value for a
// chat that has not opted out of the dial, it does not pin one. ChatSelection
// re-resolves the LIVE global default for this chat on every future spawn
// until (if ever) SetChatPermissionLevel makes an explicit choice — so a
// chat that later switches providers still has its own real, CURRENT intent
// to resolve against, not whatever an earlier provider happened to clamp it
// to, nor whatever the default was on the day it was minted. A read failure
// is swallowed, not propagated: the chat must still get created even if the
// lookup has trouble, and an unseeded chat still answers something sane —
// see the write failure's own best-effort logging for why the write side is
// the same story.
func (c *Conversations) SeedPermissionLevel(
	ctx context.Context,
	chatID string,
) {
	level, err := c.defaultPermissionLevel(ctx)
	if err != nil {
		return
	}
	if _, err := c.chats.SeedPermissionLevel(ctx, chatID, level); err != nil {
		slog.WarnContext(ctx, "agent: seed permission level (best-effort, continuing)",
			"chat_id", chatID, "err", err)
	}
}
