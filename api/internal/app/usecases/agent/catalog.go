package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type catalogRun struct {
	id     uint64
	cancel context.CancelFunc
}

const maxCatalogProcesses = 4

type catalogRuns struct {
	mu           sync.Mutex
	nextID       uint64
	runs         map[string]catalogRun
	processSlots chan struct{}
}

func newCatalogRuns() *catalogRuns {
	return &catalogRuns{
		runs:         map[string]catalogRun{},
		processSlots: make(chan struct{}, maxCatalogProcesses),
	}
}

func (r *catalogRuns) start(parent context.Context, chatID string) (context.Context, func()) {
	r.mu.Lock()
	if old, ok := r.runs[chatID]; ok {
		old.cancel()
	}
	r.nextID++
	id := r.nextID
	ctx, cancel := context.WithCancel(parent)
	r.runs[chatID] = catalogRun{id: id, cancel: cancel}
	r.mu.Unlock()
	return ctx, func() {
		cancel()
		r.mu.Lock()
		if current, ok := r.runs[chatID]; ok && current.id == id {
			delete(r.runs, chatID)
		}
		r.mu.Unlock()
	}
}

func (r *catalogRuns) acquireProcess(ctx context.Context) (func(), error) {
	select {
	case r.processSlots <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-r.processSlots }) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (u *Usecase) SlashCatalog(
	ctx context.Context,
	chatID string,
) (engineagents.SlashCatalog, error) {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return engineagents.SlashCatalog{}, fmt.Errorf("agent: slash catalog: chat: %w", err)
	}
	runner, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return engineagents.SlashCatalog{}, ErrSlashCatalogNoLiveTUI
	}
	if err != nil {
		return engineagents.SlashCatalog{}, fmt.Errorf("agent: slash catalog: live runner: %w", err)
	}
	home, _, _, worktree, err := u.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		return engineagents.SlashCatalog{}, fmt.Errorf("agent: slash catalog: worktree: %w", err)
	}
	descriptor, err := u.agents.Get(ctx, home, runner.ProviderID)
	if err != nil {
		return engineagents.SlashCatalog{}, fmt.Errorf("agent: slash catalog: resolve descriptor: %w", err)
	}

	probeCtx, finish := u.catalogs.start(ctx, chatID)
	defer finish()
	catalog, err := descriptor.SlashCatalog(probeCtx, engineagents.ProbeOptions{
		Cwd: worktree,
		Env: os.Environ(),
	}, u.catalogs.acquireProcess)

	current, placementErr := u.runners.LiveRunnerForChat(ctx, chatID)
	if errors.Is(placementErr, agentrunner.ErrNotFound) ||
		(placementErr == nil && (current.ID != runner.ID || current.ProviderID != runner.ProviderID)) {
		return engineagents.SlashCatalog{}, ErrSlashCatalogSuperseded
	}
	if placementErr != nil {
		return engineagents.SlashCatalog{}, fmt.Errorf("agent: slash catalog: revalidate live runner: %w", placementErr)
	}
	if err == nil {
		return catalog, nil
	}
	if errors.Is(err, context.Canceled) {
		if ctx.Err() != nil {
			return engineagents.SlashCatalog{}, ctx.Err()
		}
		return engineagents.SlashCatalog{}, ErrSlashCatalogSuperseded
	}
	switch {
	case errors.Is(err, engineagents.ErrCatalogUnsupported):
		return engineagents.SlashCatalog{}, ErrSlashCatalogUnsupported
	case errors.Is(err, engineagents.ErrProbeTimeout):
		return engineagents.SlashCatalog{}, ErrSlashCatalogTimeout
	case errors.Is(err, engineagents.ErrProbeCommandUnavailable):
		return engineagents.SlashCatalog{}, ErrSlashCatalogUnavailable
	case errors.Is(err, engineagents.ErrProbeOutputLimit):
		return engineagents.SlashCatalog{}, ErrSlashCatalogOutputLimit
	case errors.Is(err, engineagents.ErrProbeCommandFailed):
		return engineagents.SlashCatalog{}, ErrSlashCatalogCommand
	case errors.Is(err, engineagents.ErrCatalogMalformedOutput):
		return engineagents.SlashCatalog{}, ErrSlashCatalogMalformed
	default:
		return engineagents.SlashCatalog{}, fmt.Errorf("agent: slash catalog: probe: %w", err)
	}
}
