// Package reactors holds the workspace aggregate's post-commit, cross-aggregate
// reactions. delete.go is the physical half of a workspace delete: the Purger
// that tears a tombstoned workspace down, and the async delete reactor that runs
// it when the terminal workspace.deleted.<id> event lands (spec §3.6/§3.8). The
// Purger is the ONLY physical purger — the boot sweep re-drives the very same
// Purge for a tombstone a crash left behind (spec §7-D) — so the two paths cannot
// diverge again.
package reactors

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"

	"github.com/char2cs/crowbar/api/internal/app/repositories/drain"
)

const (
	defaultReactorTimeout = 2 * time.Minute
	defaultRetryBase      = 25 * time.Millisecond
	defaultRetryCap       = 2 * time.Second
)

// Forgetter purges an aggregate's event log; its synchronous OnForget drops the
// read-model row. asynx.Asynx satisfies it.
type Forgetter interface {
	Forget(
		ctx context.Context,
		id string,
	) error
}

// Purger physically tears down one tombstoned workspace: its dependents (review
// threads, agent chats and everything they own), its on-disk root, and finally
// the aggregate itself. It reads everything it needs off the TOMBSTONE — the
// persisted "deleted" row — so it needs no side index and works identically for
// the reactor and for the boot sweep.
type Purger struct {
	ax               Forgetter
	dropRow          func(ctx context.Context, id string) error
	forgetDependents func(ctx context.Context, wsID string) error
	removeWorktree   func(path string) error
}

// NewPurger builds the one physical purger. dropRow deletes the read-model row
// directly; removeWorktree is the hardened workspace-root remover
// (repositories/workspace/purge.WorktreeRemover).
func NewPurger(
	ax Forgetter,
	dropRow func(ctx context.Context, id string) error,
	forgetDependents func(ctx context.Context, wsID string) error,
	removeWorktree func(path string) error,
) *Purger {
	return &Purger{ax: ax, dropRow: dropRow, forgetDependents: forgetDependents, removeWorktree: removeWorktree}
}

// Purge runs the teardown once. Every step is idempotent, so a re-drive after a
// crash or a transient failure converges: the dependents are forgotten FIRST,
// while they are still listable (their rows live beside the worktree); the root
// is removed from the tombstone's own WorktreePath (an unprovisioned placeholder
// has none); the aggregate is Forgotten LAST, so a failure anywhere earlier
// leaves the tombstone for the next re-drive.
//
// Forget's OnForget drops the read-model row. When the aggregate is ALREADY
// Forgotten (ErrValidation) that projection will never run again — a crash
// between the Forget and its row delete leaves exactly this state — so the row
// is dropped here directly. Without it the tombstone outlived every boot sweep.
func (p *Purger) Purge(
	ctx context.Context,
	tomb domain.Workspace,
) error {
	if err := p.forgetDependents(ctx, tomb.ID); err != nil {
		return fmt.Errorf("forget dependents: %w", err)
	}
	if tomb.WorktreePath != "" {
		if err := p.removeWorktree(tomb.WorktreePath); err != nil {
			return fmt.Errorf("remove worktree %q: %w", tomb.WorktreePath, err)
		}
	}
	err := p.ax.Forget(ctx, tomb.ID)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, asynxModels.ErrValidation):
		if dropErr := p.dropRow(ctx, tomb.ID); dropErr != nil {
			return fmt.Errorf("drop the row of an already-forgotten aggregate: %w", dropErr)
		}
		return nil
	default:
		return fmt.Errorf("forget aggregate: %w", err)
	}
}

// StoreReader observes the durable workspace read model so the reactor purges
// only a PERSISTED tombstone (spec §3.6 ordering contract): a tombstone the
// projection has not yet saved could otherwise be re-saved after the worktree is
// gone, or be lost to a crash with nothing left for the boot sweep to find.
type StoreReader interface {
	// AwaitTombstone blocks until the read model holds id's "deleted" row and
	// returns it. It is woken by the projection's save, not by polling.
	AwaitTombstone(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
}

// Opt configures the delete reactor's bounded-wait tunables.
type Opt func(*deleteReactor)

// WithReactorTimeout bounds the whole post-commit purge (tombstone wait + every
// retry). Non-positive values are ignored.
func WithReactorTimeout(
	d time.Duration,
) Opt {
	return func(r *deleteReactor) {
		if d > 0 {
			r.timeout = d
		}
	}
}

// RegisterDeleteReactor subscribes the async delete reactor to the terminal
// workspace.deleted.<id> event on the singleton axWorkspace. The topic MUST be
// "workspace.deleted.*" (asynx anchors it to ^workspace\.deleted\..*$, matching
// the id-suffixed event); a bare "workspace.deleted" would never fire, silently
// leaking every deleted worktree (spec §3.6).
//
// For each event the reactor detaches into its own goroutine (joined to the drain
// gate for graceful shutdown) so the purge outlives the triggering request and so
// axWorkspace.Forget — itself a SendWait that re-enters the same shard's
// dispatcher — cannot deadlock against the projection-bus goroutine that invoked
// the handler.
func RegisterDeleteReactor(
	ax asynx.Asynx[domain.Workspace],
	storeReader StoreReader,
	purger *Purger,
	gate *drain.Gate,
	opts ...Opt,
) error {
	r := newDeleteReactor(storeReader, purger, gate, opts...)
	if _, err := ax.Subscribe(asynx.Topic("workspace.deleted.*"), r.onEvent); err != nil {
		return fmt.Errorf("workspace delete reactor: subscribe: %w", err)
	}
	return nil
}

type deleteReactor struct {
	storeReader StoreReader
	purger      *Purger
	gate        *drain.Gate
	timeout     time.Duration
	retryBase   time.Duration
	retryCap    time.Duration
}

func newDeleteReactor(
	storeReader StoreReader,
	purger *Purger,
	gate *drain.Gate,
	opts ...Opt,
) *deleteReactor {
	r := &deleteReactor{
		storeReader: storeReader,
		purger:      purger,
		gate:        gate,
		timeout:     defaultReactorTimeout,
		retryBase:   defaultRetryBase,
		retryCap:    defaultRetryCap,
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
	// Parked while a quiesce drains the bus (drain.Gate.Hold); never in production.
	if !r.gate.Proceed(bg) {
		return
	}
	r.purgeUntilDone(bg, wsID)
}

// purgeUntilDone waits for the persisted tombstone, then purges it, retrying a
// transient failure in either with capped exponential backoff until it succeeds
// or ctx's deadline passes. Re-running is safe — every step is idempotent — and
// the backoff keeps a persistent failure (a wedged rm, a store that is down) to
// a few dozen attempts over the reactor's lifetime instead of thousands. Past the
// deadline the tombstone is left for the boot sweep.
func (r *deleteReactor) purgeUntilDone(
	ctx context.Context,
	wsID string,
) {
	delay := r.retryBase
	for attempt := 1; ; attempt++ {
		err := r.attempt(ctx, wsID)
		if err == nil {
			return
		}
		if ctx.Err() != nil {
			slog.ErrorContext(ctx, "workspace delete reactor: purge did not complete; deferring to boot sweep",
				"id", wsID, "attempts", attempt, "err", err)
			return
		}
		slog.WarnContext(ctx, "workspace delete reactor: purge failed; retrying",
			"id", wsID, "attempt", attempt, "retry_in", delay, "err", err)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			slog.ErrorContext(ctx, "workspace delete reactor: purge did not complete; deferring to boot sweep",
				"id", wsID, "attempts", attempt, "err", err)
			return
		case <-timer.C:
		}
		delay = min(delay*2, r.retryCap)
	}
}

func (r *deleteReactor) attempt(
	ctx context.Context,
	wsID string,
) error {
	tomb, err := r.storeReader.AwaitTombstone(ctx, wsID)
	if err != nil {
		return fmt.Errorf("await tombstone: %w", err)
	}
	return r.purger.Purge(ctx, tomb)
}
