package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/core/config"
)

func (u *chatUsecase) ReadChatLog(
	ctx context.Context,
	chatID string,
) ([]agenttools.ChatTurn, error) {
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		return nil, fmt.Errorf("agent: read chat log: chat: %w", err)
	}
	turns, err := u.chatTurns(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("agent: read chat log: turns: %w", err)
	}
	out := make([]agenttools.ChatTurn, 0, len(turns))
	for _, t := range turns {
		out = append(out, agenttools.ChatTurn{Speaker: t.Speaker(), Body: t.Text})
	}
	return out, nil
}

func (u *chatUsecase) assembleConversation(
	ctx context.Context,
	chatID string,
	resuming bool,
	leftAt time.Time,
) (string, error) {
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		return "", fmt.Errorf("agent: assemble conversation: chat: %w", err)
	}

	wrapper := config.GetPrompts().HandoffWrapper
	cut := time.Time{}
	if resuming {
		wrapper = config.GetPrompts().HandoffResumeWrapper
		cut = leftAt
	}

	blob, err := u.renderConversation(ctx, chatID, cut)
	if err != nil {
		return "", fmt.Errorf("agent: assemble conversation: %w", err)
	}
	if len(blob) == 0 {
		return "", nil
	}
	return strings.ReplaceAll(wrapper, "{conversation}", string(blob)), nil
}

func composeContext(sections ...string) string {
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		if section != "" {
			parts = append(parts, section)
		}
	}
	return strings.Join(parts, "\n\n")
}

func (u *chatUsecase) AssembleHandoff(
	ctx context.Context,
	chatID string,
) (string, error) {
	return u.assembleConversation(ctx, chatID, false, time.Time{})
}

func (u *chatUsecase) threadContext(
	ctx context.Context,
	chatID string,
	minting bool,
) (string, error) {
	if minting || u.lineage == nil {
		return "", nil
	}
	ancestors, err := u.lineage.Ancestors(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: spawn runner: chat lineage: %w", err)
	}
	if len(ancestors) == 0 {
		return "", nil
	}
	return strings.ReplaceAll(config.GetPrompts().ThreadLineage, "{lineage}", renderLineage(ancestors)), nil
}

func renderLineage(
	ancestors []string,
) string {
	lines := make([]string, 0, len(ancestors))
	for _, id := range ancestors {
		lines = append(lines, "- "+id)
	}
	return strings.Join(lines, "\n")
}
