// Package reconcile runs a deduplicated, one-shot background reconcile on a workspace's first Get:
// bounded git/provider IO, then a pure sync SendWait and a corrected broadcast. List never triggers it:
// a per-workspace network fan-out would bring back the blocking wake-storm.
package reconcile

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/repositories/drain"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

const defaultReconcileTimeout = 2 * time.Minute

// DeriveFunc does the cancelable, bounded git+provider IO and returns the pure sync command (no IO).
// A vanished workspace wraps problem.ErrNotFound, an idempotent skip.
type DeriveFunc func(
	ctx context.Context,
	wsID string,
) (asynxModels.Command[domain.Workspace], error)

// SendWaitFunc dispatches the pure command and blocks until projections applied it — the single
// sanctioned crash-recovery SendWait (axWorkspace.SendWait in production).
type SendWaitFunc func(
	ctx context.Context,
	cmd asynxModels.Command[domain.Workspace],
) (asynxModels.Event[domain.Workspace], error)

// Opt configures the reconciler's bounded-IO tunables. Production wiring uses the
// defaults; tests inject a short timeout for deterministic assertions.
type Opt func(*Reconciler)

// WithTimeout bounds the whole background reconcile (derive IO + SendWait).
// Non-positive values are ignored.
func WithTimeout(
	d time.Duration,
) Opt {
	return func(r *Reconciler) {
		if d > 0 {
			r.timeout = d
		}
	}
}

// Reconciler dispatches deduplicated, one-shot, background reconcile tasks off
// the per-id read path (spec §3.8). OnOpen is safe for concurrent use.
type Reconciler struct {
	derive    DeriveFunc
	sendWait  SendWaitFunc
	broadcast func(ws domain.Workspace)
	gate      *drain.Gate

	timeout time.Duration

	mu       sync.Mutex
	inflight map[string]struct{}
}

// New builds a Reconciler over injected derive, sendWait and broadcast, so this package holds no
// git/provider dependency. Tasks join drainWG so shutdown can wait them out.
func New(
	derive DeriveFunc,
	sendWait SendWaitFunc,
	broadcast func(ws domain.Workspace),
	gate *drain.Gate,
	opts ...Opt,
) *Reconciler {
	r := &Reconciler{
		derive:    derive,
		sendWait:  sendWait,
		broadcast: broadcast,
		gate:      gate,
		timeout:   defaultReconcileTimeout,
		inflight:  map[string]struct{}{},
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// OnOpen dispatches a deduplicated background reconcile for wsID and returns at once; the work runs
// on a detached, bounded context to outlive the read. A blank id is ignored.
func (r *Reconciler) OnOpen(
	ctx context.Context,
	wsID string,
) {
	if wsID == "" {
		return
	}
	if !r.claim(wsID) {
		return
	}
	if !r.gate.Enter() {
		// Draining: no run goroutine will release the claim, so undo it here or wsID stays "in flight"
		// and never reconciles again.
		r.release(wsID)
		return
	}
	go r.run(ctx, wsID)
}

func (r *Reconciler) claim(
	wsID string,
) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, running := r.inflight[wsID]; running {
		return false
	}
	r.inflight[wsID] = struct{}{}
	return true
}

func (r *Reconciler) release(
	wsID string,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.inflight, wsID)
}

func (r *Reconciler) run(
	ctx context.Context,
	wsID string,
) {
	defer r.gate.Leave()
	defer r.release(wsID)
	bg := context.WithoutCancel(ctx)
	bg, cancel := context.WithTimeout(bg, r.timeout)
	defer cancel()
	r.reconcile(bg, wsID)
}

// reconcile re-derives, folds via a pure SendWait, then broadcasts. Idempotent: ErrNotFound and
// ErrValidation are silent no-ops.
func (r *Reconciler) reconcile(
	ctx context.Context,
	wsID string,
) {
	cmd, err := r.derive(ctx, wsID)
	if errors.Is(err, problem.ErrNotFound) {
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "workspace reconcile-on-open: derive", "id", wsID, "err", err)
		return
	}
	evt, err := r.sendWait(ctx, cmd)
	if errors.Is(err, asynxModels.ErrValidation) {
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "workspace reconcile-on-open: send", "id", wsID, "err", err)
		return
	}
	if r.broadcast != nil {
		r.broadcast(evt.Aggregate)
	}
}
