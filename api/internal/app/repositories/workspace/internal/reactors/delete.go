// Package reactors holds the workspace aggregate's post-commit, cross-aggregate
// reactions. delete.go is the async delete reactor: it subscribes to the terminal
// workspace.deleted.<id> event and performs the physical teardown OFF the
// synchronous write path (cascade review-thread Forgets + rm -rf worktree +
// axWorkspace.Forget), all of which the pure Delete command deliberately does NOT
// do (spec §3.6 cross-aggregate reactions, §3.8 delete lifecycle). The reactor is
// gated, timeout-bounded, and idempotent so a crash mid-cascade re-drives cleanly
// via the boot orphan-sweep.
package reactors

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/adapter/store/wspaths"
	"github.com/char2cs/crowbar/api/internal/domain"

	"github.com/char2cs/crowbar/api/internal/app/repositories/drain"
)

const (
	defaultReactorTimeout   = 2 * time.Minute
	defaultGatePollInterval = 25 * time.Millisecond
)

// StoreReader observes the durable workspace read model so the reactor can gate
// its purge on the persisted "deleted" tombstone row (spec §3.6 ordering
// contract). Get returns nil when the read model has no row for the id. The
// save-only store projection's *Store satisfies this.
type StoreReader interface {
	Get(
		ctx context.Context,
		id string,
	) (*domain.Workspace, error)
}

// Opt configures the delete reactor's bounded-wait tunables. Production wiring
// (Task 14) uses the defaults; tests inject short intervals for deterministic,
// fast gate assertions.
type Opt func(*deleteReactor)

// WithReactorTimeout bounds the whole post-commit purge (gate wait + fs delete +
// Forget). Non-positive values are ignored.
func WithReactorTimeout(
	d time.Duration,
) Opt {
	return func(r *deleteReactor) {
		if d > 0 {
			r.timeout = d
		}
	}
}

// WithGatePollInterval sets how often the ordering gate re-reads the read model
// while waiting for the persisted "deleted" row. Non-positive values are ignored.
func WithGatePollInterval(
	d time.Duration,
) Opt {
	return func(r *deleteReactor) {
		if d > 0 {
			r.pollInterval = d
		}
	}
}

// RegisterDeleteReactor subscribes the async delete reactor to the terminal
// workspace.deleted.<id> event on the singleton axWorkspace. The topic MUST be
// "workspace.deleted.*" (asynx anchors it to ^workspace\.deleted\..*$, matching
// the id-suffixed event); a bare "workspace.deleted" would never fire, silently
// leaking every deleted worktree (spec §3.6).
//
// For each event the reactor detaches into its own goroutine (registered on
// drainWG for graceful shutdown) so the purge outlives the triggering request and
// so axWorkspace.Forget — itself a SendWait that re-enters the same shard's
// dispatcher — cannot deadlock against the projection-bus goroutine that invoked
// the handler. The goroutine gates on the persisted "deleted" row, then resolves
// the worktree path (pathsStore), cascades reviewThreadForget, rm -rf's the
// worktree, deletes the id↔path row (§3.9 write-point (c)), and finally Forgets
// the aggregate (its synchronous OnForget drops the read-model row). Every step
// is idempotent, so the boot orphan-sweep can re-drive it verbatim after a crash.
func RegisterDeleteReactor(
	ax asynx.Asynx[domain.Workspace],
	storeReader StoreReader,
	pathsStore wspaths.WorkspacePaths,
	reviewThreadForget func(ctx context.Context, wsID string) error,
	rmWorktree func(path string) error,
	gate *drain.Gate,
	opts ...Opt,
) error {
	r := newDeleteReactor(ax, storeReader, pathsStore, reviewThreadForget, rmWorktree, gate, opts...)
	if _, err := ax.Subscribe(asynx.Topic("workspace.deleted.*"), r.onEvent); err != nil {
		return fmt.Errorf("workspace delete reactor: subscribe: %w", err)
	}
	return nil
}

type deleteReactor struct {
	ax                 asynx.Asynx[domain.Workspace]
	storeReader        StoreReader
	pathsStore         wspaths.WorkspacePaths
	reviewThreadForget func(ctx context.Context, wsID string) error
	rmWorktree         func(path string) error
	gate               *drain.Gate
	timeout            time.Duration
	pollInterval       time.Duration
}

func newDeleteReactor(
	ax asynx.Asynx[domain.Workspace],
	storeReader StoreReader,
	pathsStore wspaths.WorkspacePaths,
	reviewThreadForget func(ctx context.Context, wsID string) error,
	rmWorktree func(path string) error,
	gate *drain.Gate,
	opts ...Opt,
) *deleteReactor {
	r := &deleteReactor{
		ax:                 ax,
		storeReader:        storeReader,
		pathsStore:         pathsStore,
		reviewThreadForget: reviewThreadForget,
		rmWorktree:         rmWorktree,
		gate:               gate,
		timeout:            defaultReactorTimeout,
		pollInterval:       defaultGatePollInterval,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *deleteReactor) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.Workspace],
) {
	wsID := evt.AggregateID
	if wsID == "" {
		wsID = evt.Aggregate.ID
	}
	// Refused once the daemon is draining (drain.Gate). Not a dropped event: the DBs
	// this purge would write to are about to close, and the boot sweep re-purges.
	if !r.gate.Enter() {
		return
	}
	go r.run(ctx, wsID)
}

func (r *deleteReactor) run(
	ctx context.Context,
	wsID string,
) {
	defer r.gate.Leave()
	bg := context.WithoutCancel(ctx)
	bg, cancel := context.WithTimeout(bg, r.timeout)
	defer cancel()
	r.purge(bg, wsID)
}

func (r *deleteReactor) purge(
	ctx context.Context,
	wsID string,
) {
	if !r.awaitTombstone(ctx, wsID) {
		slog.WarnContext(ctx, "workspace delete reactor: tombstone not observed before deadline; deferring to boot sweep", "id", wsID)
		return
	}
	path, ok := r.resolvePath(ctx, wsID)
	if !ok {
		return
	}
	if err := r.reviewThreadForget(ctx, wsID); err != nil {
		// The injected callback is the composed cross-aggregate forget cascade
		// (review threads + agent chats — see repositories.Container.forgetDependents),
		// so this label stays generic; the wrapped err names the half that failed.
		slog.ErrorContext(ctx, "workspace delete reactor: delete cascade", "id", wsID, "err", err)
		return
	}
	if !r.removeWorktree(path, wsID) {
		return
	}
	if err := r.pathsStore.Delete(ctx, wsID); err != nil {
		slog.ErrorContext(ctx, "workspace delete reactor: delete id-path row", "id", wsID, "err", err)
		return
	}
	r.forget(ctx, wsID)
}

func (r *deleteReactor) awaitTombstone(
	ctx context.Context,
	wsID string,
) bool {
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	for {
		if r.tombstonePresent(ctx, wsID) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}

func (r *deleteReactor) tombstonePresent(
	ctx context.Context,
	wsID string,
) bool {
	ws, err := r.storeReader.Get(ctx, wsID)
	return err == nil && ws != nil && ws.Status == domain.WorkspaceStatusDeleted
}

// resolvePath returns the last-known worktree path for the workspace. A missing
// id↔path row is an idempotent no-op (empty path, ok=true) — there is nothing to
// rm; a real store error aborts the purge so the tombstone survives for the boot
// sweep to re-drive.
func (r *deleteReactor) resolvePath(
	ctx context.Context,
	wsID string,
) (string, bool) {
	path, err := r.pathsStore.Get(ctx, wsID)
	if errors.Is(err, wspaths.ErrNotFound) {
		return "", true
	}
	if err != nil {
		slog.ErrorContext(ctx, "workspace delete reactor: resolve id-path row", "id", wsID, "err", err)
		return "", false
	}
	return path, true
}

func (r *deleteReactor) removeWorktree(
	path string,
	wsID string,
) bool {
	if path == "" {
		return true
	}
	if err := r.rmWorktree(path); err != nil {
		slog.Error("workspace delete reactor: rm worktree", "id", wsID, "path", path, "err", err)
		return false
	}
	return true
}

// forget purges the aggregate as the terminal step. An ErrValidation means the
// aggregate is already Forgotten (an idempotent re-drive) and is swallowed; any
// other error leaves the tombstone for the boot sweep to re-drive.
func (r *deleteReactor) forget(
	ctx context.Context,
	wsID string,
) {
	err := r.ax.Forget(ctx, wsID)
	if err == nil || errors.Is(err, asynxModels.ErrValidation) {
		return
	}
	slog.ErrorContext(ctx, "workspace delete reactor: forget aggregate", "id", wsID, "err", err)
}
