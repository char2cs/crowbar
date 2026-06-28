package app

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/realtime"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
	"github.com/char2cs/crowbar/api/internal/engine/provider"
	"github.com/char2cs/crowbar/api/internal/engine/provider/poll"
)

// Container is the application layer: the hub, the aggregate repositories, the
// GORM CRUD stores, the composed usecases, and the realtime service owning the
// lazy file-watcher and LSP lifecycles.
type Container struct {
	Hub          *hub.Hub
	Repositories *repositories.Container
	GORM         *GORMStores
	Usecases     *usecases.Container
	Realtime     *realtime.Service
}

// New constructs the application layer from the engine and adapter containers
// and wires the aggregate repositories into the hub (00 §7).
func New(
	ctx context.Context,
	engines *engine.Container,
	adapters *adapter.Container,
) (*Container, error) {
	axChat, err := newAsynx[domain.Chat](adapters.ChatES())
	if err != nil {
		return nil, fmt.Errorf("app: asynx chat: %w", err)
	}
	axReviewThread, err := newAsynx[domain.ReviewThread](adapters.ReviewThreadES())
	if err != nil {
		return nil, fmt.Errorf("app: asynx review thread: %w", err)
	}

	gormStores, err := newGORMStores(adapters.GlobalView())
	if err != nil {
		return nil, err
	}

	h := hub.NewHub()
	repos, err := repositories.New(
		ctx,
		adapters,
		h,
		axChat,
		axReviewThread,
		newAsynx[domain.Workspace],
		engines.Git,
	)
	if err != nil {
		return nil, fmt.Errorf("app: repositories: %w", err)
	}

	// Path-deriving usecases must share the adapter's resolved home so git
	// worktrees and per-entity storages land under the same root.
	crowbarHome := adapters.CrowbarHome()
	homeFunc := func() (string, error) { return crowbarHome, nil }
	ucs, err := usecases.New(repos, toUsecaseStores(gormStores), engines, homeFunc)
	if err != nil {
		return nil, fmt.Errorf("app: usecases: %w", err)
	}

	startProviderSweep(ctx, engines, repos, ucs)
	startRecoverySweep(ctx, ucs)

	rt := realtime.New(
		ctx,
		h,
		repos.Workspace,
		engines.Git,
		engines.FS,
		realtime.NewLSPLifecycle(engines.LSP),
		ucs.ProviderSync,
		poll.PerConnectionInterval,
		time.Now,
	)

	return &Container{
		Hub:          h,
		Repositories: repos,
		GORM:         gormStores,
		Usecases:     ucs,
		Realtime:     rt,
	}, nil
}

// Close tears down the application layer's live realtime resources: it stops
// every file watcher and LSP host the service still holds. It is idempotent and
// runs on graceful shutdown so fsnotify file descriptors and LSP subprocesses
// are released promptly.
func (c *Container) Close() {
	c.Realtime.Close()
}

func toUsecaseStores(
	gormStores *GORMStores,
) usecases.GORMStores {
	return usecases.GORMStores{
		Projects:         gormStores.Projects,
		Repositories:     gormStores.Repositories,
		TerminalProfiles: gormStores.TerminalProfiles,
		TerminalSessions: gormStores.TerminalSessions,
	}
}

func startProviderSweep(
	ctx context.Context,
	engines *engine.Container,
	repos *repositories.Container,
	ucs *usecases.Container,
) {
	engines.Provider.StartBackgroundSweep(
		ctx,
		sweepTargets(repos.Workspace),
		sweepCallback(ctx, ucs),
	)
}

// startRecoverySweep runs the one-shot startup recovery sweep (H19) in the
// background so boot stays fast. Unlike the provider sweep this is not a cron —
// it runs exactly once at startup, re-syncing each workspace's git state from
// disk and reaping orphaned worktrees. Its effects broadcast over WS as each
// workspace re-syncs. A panic is contained by safego; ReconcileAll is itself
// best-effort and never returns a fatal error, so its result is ignored.
func startRecoverySweep(
	ctx context.Context,
	ucs *usecases.Container,
) {
	safego.Go("app.recoverySweep", func() {
		_ = ucs.Worktree.ReconcileAll(context.WithoutCancel(ctx))
	})
}

func sweepCallback(
	ctx context.Context,
	ucs *usecases.Container,
) func(
	wsID string,
	state provider.ProviderState,
) {
	return func(
		wsID string,
		state provider.ProviderState,
	) {
		_ = ucs.ProviderSync.SyncFromState(
			context.WithoutCancel(ctx),
			wsID,
			state,
			time.Now(),
		)
	}
}

// shouldSweep reports whether the global cron should re-poll a workspace: it
// must have a live PR (PRUrl != "") and must not be in a terminal PR state
// (pr-merged/pr-closed), which are never re-polled (D10/§11). This widens the
// old Status==pr-open filter so pr-open->pr-merged/closed transitions are
// observed on unwatched workspaces and pr-conflicts workspaces keep syncing.
func shouldSweep(
	ws domain.Workspace,
) bool {
	if ws.PRUrl == "" {
		return false
	}
	if ws.Status == domain.WorkspaceStatusPRMerged ||
		ws.Status == domain.WorkspaceStatusPRClosed {
		return false
	}
	return true
}

func sweepTargets(
	repo workspace.Workspace,
) func() []poll.SweepTarget {
	return func() []poll.SweepTarget {
		rows, err := repo.List(context.Background())
		if err != nil {
			return nil
		}
		targets := make([]poll.SweepTarget, 0, len(rows))
		for _, ws := range rows {
			if !shouldSweep(ws) {
				continue
			}
			targets = append(targets, poll.SweepTarget{
				WSID:      ws.ID,
				RepoPath:  ws.WorktreePath,
				Branch:    ws.Branch,
				HasOpenPR: true,
			})
		}
		return targets
	}
}
