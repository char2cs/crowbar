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

// catalogRuns enforces one deterministic provider probe per chat. Opening a new
// menu supersedes and cancels an older abandoned request; no result is cached.
//
// processSlots is the daemon-wide process budget, not a per-chat limit. A Claude
// inventory can fan out several descriptor-declared detail commands, and without
// one shared gate N windows opening `/` at once can fork N times that fanout. The
// engine acquires one slot around every provider command, so inventory, detail,
// and single-command adapters all consume the same bounded resource.
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

// acquireProcess waits for one daemon-wide provider-process slot. Waiting is
// cancellation-aware and is included in the probe's descriptor timeout, so an
// abandoned menu cannot remain queued after its request or chat probe is gone.
func (r *catalogRuns) acquireProcess(ctx context.Context) (func(), error) {
	select {
	case r.processSlots <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-r.processSlots }) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SlashCatalog runs the current live provider's descriptor-declared,
// deterministic capability probe in the chat worktree. It reads no provider
// files and retains no backend cache.
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
	// A catalog describes the exact TUI that was live when this request began.
	// Provider switching and native conversation movement deliberately do not take
	// the catalog request lock, so the process may finish after that runner has
	// left. Re-read placement before returning either data or a provider-command
	// error: stale data (or an error from a provider no longer selected) must never
	// be published as the current chat's menu.
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
