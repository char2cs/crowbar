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
// declares the /compact slash command over the prompt transport; codex declares
// thread/compact/start over the api transport. A provider that declares neither
// cannot be asked, and says so with ErrNotFound rather than silently doing nothing.
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

	if wire == "prompt" {
		text := payload["text"]
		if text == "" {
			return fmt.Errorf(
				"agent: compact: %q declares compact_start with no text: %w",
				providerID, apperr.ErrInvalidArgument,
			)
		}
		if _, err := rs.SubmitPrompt(ctx, chatID, text, uuid.NewString()); err != nil {
			return fmt.Errorf("agent: compact: %w", err)
		}
		return nil
	}

	// Any other wire is an api-transport call (codex: thread/compact/start),
	// driven over this chat's already-live connection exactly the way
	// interruptTurn drives turn/interrupt — a fresh dial has no session to
	// compact. A chat with no live api connection (still spawning, running
	// over hooks-only, or between runners) cannot be asked yet.
	live, err := rs.runnerStore.LiveRunnerForChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf(
			"agent: compact: %q compacts over the %q wire, which needs a live api "+
				"connection this chat does not currently have: %w",
			providerID, wire, apperr.ErrUnavailable,
		)
	}
	conn, ok := rs.apiConns.get(live.ID)
	if !ok {
		return fmt.Errorf(
			"agent: compact: %q compacts over the %q wire, which needs a live api "+
				"connection this chat does not currently have: %w",
			providerID, wire, apperr.ErrUnavailable,
		)
	}
	if err := conn.driver.Send(ctx, compactStartEvent, nil); err != nil {
		return fmt.Errorf("agent: compact: %w", err)
	}
	return nil
}
