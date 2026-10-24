package runner

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// compactStartEvent is the canonical outbound event a provider declares when Crowbar
// can ask it to compact. Key-presence on the descriptor is the whole capability check.
const compactStartEvent = "compact_start"

// Compact asks the chat's CLI to compact its own context by the gesture its
// descriptor declares (ErrNotFound for none), from Crowbar's own chat only. The
// CLI reports it back through compact_pre and compact_post.
func (rs *Runners) Compact(ctx context.Context, chatID string) error {
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: compact: %w", err)
	}
	// Before anything is resolved: the SURFACE decides, not the provider. See
	// ErrCompactionOffSurface (capabilities.go) — and CompactionOnChatSurface
	// beside it, which is the read the client gates its control on so the
	// button is absent rather than present-and-erroring.
	if chat.Surface == domain.SurfaceTerminal {
		return ErrCompactionOffSurface
	}

	providerID, err := rs.conversations.ChatProviderID(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: compact: %w", err)
	}
	cwdWorkspaceID, err := rs.cwdWorkspaceID(ctx, chat.ID, chat.WorkspaceID)
	if err != nil {
		return err
	}
	crowbarHome, _, _, _, err := rs.ws.WorktreeDir(ctx, cwdWorkspaceID)
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

	// Any other wire is a call over the chat's live api connection, as an
	// interrupt is: a fresh dial has no session to compact, so a chat with no
	// live connection cannot be asked yet.
	live, _ := rs.live.byChat(chatID) // dormant: no id, so no connection
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
