package resources

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

type OriginSyncWorkspaces interface {
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
}

type OriginFetcher interface {
	FetchRef(
		ctx context.Context,
		repoPath string,
		branch string,
	) error
	MergeFFOnly(
		ctx context.Context,
		repoPath string,
		branch string,
	) error
}

const OriginSyncInterval = 5 * time.Minute

const originSyncTimeout = 30 * time.Second

type OriginSync struct {
	root      context.Context
	interval  time.Duration
	workspace OriginSyncWorkspaces
	git       OriginFetcher
	mu        sync.Mutex
	cancels   map[string]context.CancelFunc
	closed    bool

	runners sync.WaitGroup

	ticks     <-chan time.Time
	cycleDone chan<- struct{}
}

func NewOriginSync(
	root context.Context,
	interval time.Duration,
	workspace OriginSyncWorkspaces,
	git OriginFetcher,
) *OriginSync {
	return &OriginSync{
		root:      root,
		interval:  interval,
		workspace: workspace,
		git:       git,
		cancels:   make(map[string]context.CancelFunc),
	}
}

func (m *OriginSync) Start(
	wsID string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errClosed
	}
	ctx, cancel := context.WithCancel(m.root)
	m.cancels[wsID] = cancel
	m.runners.Add(1)
	go m.run(ctx, cancel, wsID)
	return nil
}

func (m *OriginSync) Stop(
	wsID string,
) {
	m.mu.Lock()
	cancel, ok := m.cancels[wsID]
	delete(m.cancels, wsID)
	m.mu.Unlock()
	if ok {
		cancel()
	}
}

func (m *OriginSync) Close() {
	m.mu.Lock()
	m.closed = true
	cancels := m.cancels
	m.cancels = make(map[string]context.CancelFunc)
	m.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
	m.runners.Wait()
}

func (m *OriginSync) run(
	ctx context.Context,
	cancel context.CancelFunc,
	wsID string,
) {
	defer m.runners.Done()
	defer cancel()
	defer safego.Recover("resources.originSync.run")
	select {
	case <-ctx.Done():
		return
	default:
		m.syncTick(ctx, wsID, true)
		m.notifyCycle(ctx)
	}

	tickC, stopTicker := m.tickSource()
	defer stopTicker()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tickC:
			m.syncTick(ctx, wsID, false)
			m.notifyCycle(ctx)
		}
	}
}

func (m *OriginSync) tickSource() (<-chan time.Time, func()) {
	m.mu.Lock()
	ticks := m.ticks
	m.mu.Unlock()

	if ticks != nil {
		return ticks, func() {}
	}
	ticker := time.NewTicker(m.interval)
	return ticker.C, ticker.Stop
}

func (m *OriginSync) notifyCycle(
	ctx context.Context,
) {
	m.mu.Lock()
	done := m.cycleDone
	m.mu.Unlock()

	if done == nil {
		return
	}
	select {
	case done <- struct{}{}:
	case <-ctx.Done():
	}
}

func (m *OriginSync) syncTick(
	ctx context.Context,
	wsID string,
	advance bool,
) {
	m.syncTickWithTimeout(ctx, wsID, originSyncTimeout, advance)
}

func (m *OriginSync) syncTickWithTimeout(
	ctx context.Context,
	wsID string,
	timeout time.Duration,
	advance bool,
) {
	syncCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ws, err := m.workspace.Get(syncCtx, wsID)
	if err != nil {
		slog.WarnContext(syncCtx, "origin sync: load workspace", "workspace_id", wsID, "err", err)
		return
	}
	if ws.ParentID != "" {
		return
	}
	if err := m.git.FetchRef(syncCtx, ws.WorktreePath, ws.Branch); err != nil {
		slog.DebugContext(syncCtx, "origin sync: fetch ref", "workspace_id", wsID, "branch", ws.Branch, "err", err)
		return
	}
	if advance {
		m.advanceLockedRoot(syncCtx, ws)
	}
}

func (m *OriginSync) advanceLockedRoot(
	ctx context.Context,
	ws domain.Workspace,
) {
	if ws.Status != domain.WorkspaceStatusLocked || ws.Provisioning != domain.WorkspaceProvisioned {
		return
	}
	if ctx.Err() != nil {
		return
	}
	err := m.git.MergeFFOnly(context.WithoutCancel(ctx), ws.WorktreePath, "origin/"+ws.Branch)
	if err == nil {
		return
	}
	if errors.Is(err, enginegit.ErrDirtyTree) {
		slog.DebugContext(ctx, "origin sync: locked root has local changes; leaving it where it is",
			"workspace_id", ws.ID, "branch", ws.Branch)
		return
	}
	slog.WarnContext(ctx, "origin sync: could not fast-forward locked root",
		"workspace_id", ws.ID, "branch", ws.Branch, "err", err)
}
