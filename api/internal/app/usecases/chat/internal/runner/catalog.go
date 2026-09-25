package runner

import (
	"context"
	"errors"
	"fmt"
	"os"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// SlashCatalog probes the chat's provider with the descriptor's own short-lived
// command. It needs no live TUI: the catalogue is the provider's and the
// worktree's, so a dormant chat is answered the same as a live one.
func (rs *Runners) SlashCatalog(
	ctx context.Context,
	chatID string,
) (engineagents.SlashCatalog, error) {
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return engineagents.SlashCatalog{}, fmt.Errorf("agent: slash catalog: chat: %w", err)
	}
	providerID, err := rs.conversations.ChatProviderID(ctx, chatID)
	if err != nil {
		return engineagents.SlashCatalog{}, fmt.Errorf("agent: slash catalog: %w", err)
	}
	cwdWorkspaceID, err := rs.cwdWorkspaceID(ctx, chat.ID, chat.WorkspaceID)
	if err != nil {
		return engineagents.SlashCatalog{}, err
	}
	home, _, _, worktree, err := rs.ws.WorktreeDir(ctx, cwdWorkspaceID)
	if err != nil {
		return engineagents.SlashCatalog{}, fmt.Errorf("agent: slash catalog: worktree: %w", err)
	}
	descriptor, err := rs.agents.Get(ctx, home, providerID)
	if err != nil {
		return engineagents.SlashCatalog{}, fmt.Errorf("agent: slash catalog: resolve descriptor: %w", err)
	}

	probeCtx, finish := rs.catalogs.Start(ctx, chatID)
	defer finish()
	catalog, err := descriptor.SlashCatalog(probeCtx, engineagents.ProbeOptions{
		Cwd: worktree,
		Env: os.Environ(),
	}, rs.catalogs.AcquireProcess)

	// A chat switched to another provider mid-probe gets that one's catalogue
	// on its next request; this one would be shown under the wrong CLI.
	if current, curErr := rs.conversations.ChatProviderID(ctx, chatID); curErr != nil || current != providerID {
		return engineagents.SlashCatalog{}, ErrSlashCatalogSuperseded
	}
	if err != nil {
		return engineagents.SlashCatalog{}, slashCatalogError(ctx, err)
	}
	return catalog, nil
}

func slashCatalogError(
	ctx context.Context,
	err error,
) error {
	if errors.Is(err, context.Canceled) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrSlashCatalogSuperseded
	}
	switch {
	case errors.Is(err, engineagents.ErrCatalogUnsupported):
		return ErrSlashCatalogUnsupported
	case errors.Is(err, engineagents.ErrProbeTimeout):
		return ErrSlashCatalogTimeout
	case errors.Is(err, engineagents.ErrProbeCommandUnavailable):
		return ErrSlashCatalogUnavailable
	case errors.Is(err, engineagents.ErrProbeOutputLimit):
		return ErrSlashCatalogOutputLimit
	case errors.Is(err, engineagents.ErrProbeCommandFailed):
		return ErrSlashCatalogCommand
	case errors.Is(err, engineagents.ErrCatalogMalformedOutput):
		return ErrSlashCatalogMalformed
	default:
		return fmt.Errorf("agent: slash catalog: probe: %w", err)
	}
}
