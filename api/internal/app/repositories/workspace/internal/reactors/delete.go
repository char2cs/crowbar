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

type Forgetter interface {
	Forget(
		ctx context.Context,
		id string,
	) error
}

type Purger struct {
	ax               Forgetter
	dropRow          func(ctx context.Context, id string) error
	forgetDependents func(ctx context.Context, wsID string) error
	removeWorktree   func(ctx context.Context, tomb domain.Workspace) error
}

func NewPurger(
	ax Forgetter,
	dropRow func(ctx context.Context, id string) error,
	forgetDependents func(ctx context.Context, wsID string) error,
	removeWorktree func(ctx context.Context, tomb domain.Workspace) error,
) *Purger {
	return &Purger{ax: ax, dropRow: dropRow, forgetDependents: forgetDependents, removeWorktree: removeWorktree}
}

func (p *Purger) Purge(
	ctx context.Context,
	tomb domain.Workspace,
) error {
	if err := p.forgetDependents(ctx, tomb.ID); err != nil {
		return fmt.Errorf("forget dependents: %w", err)
	}
	if tomb.Provisioning == domain.WorkspaceProvisioned {
		if err := p.removeWorktree(ctx, tomb); err != nil {
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

type StoreReader interface {
	AwaitTombstone(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
}

type GiveUp func(ctx context.Context, wsID string, tomb *domain.Workspace, err error)

type Opt func(*deleteReactor)

func WithReactorTimeout(
	d time.Duration,
) Opt {
	return func(r *deleteReactor) {
		if d > 0 {
			r.timeout = d
		}
	}
}

func RegisterDeleteReactor(
	root context.Context,
	ax asynx.Asynx[domain.Workspace],
	storeReader StoreReader,
	purger *Purger,
	gate *drain.Gate,
	onGiveUp GiveUp,
	opts ...Opt,
) error {
	r := newDeleteReactor(root, storeReader, purger, gate, onGiveUp, opts...)
	if _, err := ax.Subscribe(asynx.Topic("workspace.deleted.*"), r.onEvent); err != nil {
		return fmt.Errorf("workspace delete reactor: subscribe: %w", err)
	}
	return nil
}

type deleteReactor struct {
	root        context.Context
	storeReader StoreReader
	purger      *Purger
	gate        *drain.Gate
	onGiveUp    GiveUp
	timeout     time.Duration
	retryBase   time.Duration
	retryCap    time.Duration
}

func newDeleteReactor(
	root context.Context,
	storeReader StoreReader,
	purger *Purger,
	gate *drain.Gate,
	onGiveUp GiveUp,
	opts ...Opt,
) *deleteReactor {
	r := &deleteReactor{
		root:        root,
		storeReader: storeReader,
		purger:      purger,
		gate:        gate,
		onGiveUp:    onGiveUp,
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
	_ context.Context,
	evt asynxModels.Event[domain.Workspace],
) {
	wsID := evt.AggregateID
	if wsID == "" {
		wsID = evt.Aggregate.ID
	}
	if !r.gate.Enter() {
		return
	}
	go r.run(wsID) //nolint:contextcheck // the purge must not inherit the handler ctx: asynx refuses a Forget made from it (ErrForgetFromHandler)
}

func (r *deleteReactor) run(
	wsID string,
) {
	defer r.gate.Leave()
	bg, cancel := context.WithTimeout(context.WithoutCancel(r.root), r.timeout)
	defer cancel()
	if !r.gate.Proceed(bg) {
		return
	}
	r.purgeUntilDone(bg, wsID)
}

func (r *deleteReactor) purgeUntilDone(
	ctx context.Context,
	wsID string,
) {
	delay := r.retryBase
	var tomb *domain.Workspace
	for attempt := 1; ; attempt++ {
		observed, err := r.attempt(ctx, wsID)
		if err == nil {
			return
		}
		if observed != nil {
			tomb = observed
		}
		if ctx.Err() != nil {
			r.giveUp(ctx, wsID, tomb, err)
			return
		}
		slog.WarnContext(ctx, "workspace delete reactor: purge failed; retrying",
			"id", wsID, "attempt", attempt, "retry_in", delay, "err", err)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			r.giveUp(ctx, wsID, tomb, err)
			return
		case <-timer.C:
		}
		delay = min(delay*2, r.retryCap)
	}
}

func (r *deleteReactor) giveUp(
	ctx context.Context,
	wsID string,
	tomb *domain.Workspace,
	err error,
) {
	r.onGiveUp(context.WithoutCancel(ctx), wsID, tomb, err)
}

func (r *deleteReactor) attempt(
	ctx context.Context,
	wsID string,
) (*domain.Workspace, error) {
	tomb, err := r.storeReader.AwaitTombstone(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("await tombstone: %w", err)
	}
	return &tomb, r.purger.Purge(ctx, tomb)
}
