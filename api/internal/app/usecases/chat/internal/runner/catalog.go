package runner

import (
	"context"
	"errors"
	"fmt"
	"os"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

func (rs *Runners) SlashCatalog(
	ctx context.Context,
	chatID string,
) (engineagents.SlashCatalog, error) {
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return engineagents.SlashCatalog{}, fmt.Errorf("agent: slash catalog: chat: %w", err)
	}
	runner, err := rs.runnerStore.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return engineagents.SlashCatalog{}, ErrSlashCatalogNoLiveTUI
	}
	if err != nil {
		return engineagents.SlashCatalog{}, fmt.Errorf("agent: slash catalog: live runner: %w", err)
	}
	cwdWorkspaceID, err := rs.cwdWorkspaceID(ctx, chat.ID, chat.WorkspaceID)
	if err != nil {
		return engineagents.SlashCatalog{}, err
	}
	home, _, _, worktree, err := rs.ws.WorktreeDir(ctx, cwdWorkspaceID)
	if err != nil {
		return engineagents.SlashCatalog{}, fmt.Errorf("agent: slash catalog: worktree: %w", err)
	}
	descriptor, err := rs.agents.Get(ctx, home, runner.ProviderID)
	if err != nil {
		return engineagents.SlashCatalog{}, fmt.Errorf("agent: slash catalog: resolve descriptor: %w", err)
	}

	probeCtx, finish := rs.catalogs.Start(ctx, chatID)
	defer finish()
	catalog, err := descriptor.SlashCatalog(probeCtx, engineagents.ProbeOptions{
		Cwd: worktree,
		Env: os.Environ(),
	}, rs.catalogs.AcquireProcess)

	if err := rs.catalogStillCurrent(ctx, chatID, runner); err != nil {
		return engineagents.SlashCatalog{}, err
	}
	if err != nil {
		return engineagents.SlashCatalog{}, slashCatalogError(ctx, err)
	}
	return catalog, nil
}

func (rs *Runners) catalogStillCurrent(
	ctx context.Context,
	chatID string,
	probed engineagents.Runner,
) error {
	current, err := rs.runnerStore.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return ErrSlashCatalogSuperseded
	}
	if err != nil {
		return fmt.Errorf("agent: slash catalog: revalidate live runner: %w", err)
	}
	if current.ID != probed.ID || current.ProviderID != probed.ProviderID {
		return ErrSlashCatalogSuperseded
	}
	return nil
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
