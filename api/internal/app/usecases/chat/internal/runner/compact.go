package runner

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
)

// compactStartEvent is the canonical outbound event a provider declares when Crowbar
// can ask it to compact. Key-presence on the descriptor is the whole capability check.
const compactStartEvent = "compact_start"

// Compact asks the chat's CLI to compact its own context.
//
// Crowbar does not compact anything itself — it cannot, the context belongs to the
// provider. It sends the provider's own declared gesture: claude has no API for it and
// declares the /compact slash command over the prompt transport, codex would call
// thread/compact/start. A provider that declares neither cannot be asked, and says so
// with ErrNotFound rather than silently doing nothing.
//
// The provider then reports back through compact_pre and compact_post, which is how
// the chat learns it happened; nothing here writes that record.
func (rs *Runners) Compact(ctx context.Context, chatID string) error {
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: compact: %w", err)
	}

	providerID, err := rs.conversations.ChatProviderID(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: compact: %w", err)
	}
	crowbarHome, _, _, _, err := rs.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		return fmt.Errorf("agent: compact: worktree dir: %w", err)
	}
	agent, err := rs.agents.Get(ctx, crowbarHome, providerID)
	if err != nil {
		return fmt.Errorf("agent: compact: resolve descriptor: %w", err)
	}

	wire, payload, ok := agent.OutboundCall(compactStartEvent, map[string]string{
		"session_id": chat.ID,
	})
	if !ok {
		return fmt.Errorf(
			"agent: compact: %q declares no compaction gesture: %w", providerID, apperr.ErrNotFound,
		)
	}

	// The only outbound wire this package can currently drive is the prompt path: the
	// text goes to the CLI exactly as a human typing it would. An RPC-transport
	// provider needs the jsonrpc transport, which does not exist yet — so rather than
	// half-send it, that case is refused explicitly.
	if wire != "prompt" {
		return fmt.Errorf(
			"agent: compact: %q compacts over the %q wire, which Crowbar cannot drive yet: %w",
			providerID, wire, apperr.ErrUnavailable,
		)
	}
	text := payload["text"]
	if text == "" {
		return fmt.Errorf(
			"agent: compact: %q declares compact_start with no text: %w", providerID, apperr.ErrInvalidArgument,
		)
	}

	if _, err := rs.SubmitPrompt(ctx, chatID, text, uuid.NewString()); err != nil {
		return fmt.Errorf("agent: compact: %w", err)
	}
	return nil
}
