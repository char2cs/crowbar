package runner

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

const compactStartEvent = "compact_start"

func (rs *Runners) Compact(ctx context.Context, chatID string) error {
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: compact: %w", err)
	}
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
			"agent: compact: %q declares no compaction gesture: %w", providerID, problem.ErrNotFound,
		)
	}

	if wire == "prompt" {
		text := payload["text"]
		if text == "" {
			return fmt.Errorf(
				"agent: compact: %q declares compact_start with no text: %w",
				providerID, problem.ErrInvalidArgument,
			)
		}
		if _, err := rs.SubmitPrompt(ctx, chatID, text, uuid.NewString()); err != nil {
			return fmt.Errorf("agent: compact: %w", err)
		}
		return nil
	}

	live, _ := rs.live.byChat(chatID)
	conn, ok := rs.apiConns.get(live.ID)
	if !ok {
		return fmt.Errorf(
			"agent: compact: %q compacts over the %q wire, which needs a live api "+
				"connection this chat does not currently have: %w",
			providerID, wire, problem.ErrUnavailable,
		)
	}
	if err := conn.driver.Send(ctx, compactStartEvent, nil); err != nil {
		return fmt.Errorf("agent: compact: %w", err)
	}
	return nil
}
