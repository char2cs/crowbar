// Package reconcile implements lazy, bounded, off-the-read-path
// reconcile-on-open (spec §3.8, decisions 4 & 9). The first per-id Get after
// boot dispatches a deduplicated, one-shot background task per workspace: it
// re-derives git+provider reality as cancelable, timeout-bounded IO (decision 9
// — network/git never sits unbounded in any path), then SendWaits a PURE sync
// command (decision 4's single sanctioned crash-recovery SendWait — the command
// itself does no IO) and broadcasts the corrected frame to clients. The List
// path never triggers reconcile: a per-workspace network fan-out across a List
// would reintroduce the blocking-network / wake-storm wedge this refactor exists
// to kill (§1.1/§1.2).
package reconcile

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"

	"github.com/char2cs/crowbar/api/internal/app/repositories/drain"
)

const defaultReconcileTimeout = 2 * time.Minute

// DeriveFunc performs the cancelable, timeout-bounded git+provider re-derivation
// for one workspace and returns the PURE sync command that folds the re-derived
// reality into aggregate state. All IO lives HERE (decision 9); the returned
// command does none (decision 9). A vanished workspace is signalled by wrapping
// apperr.ErrNotFound, which the reconciler treats as an idempotent skip.
type DeriveFunc func(
	ctx context.Context,
	wsID string,
) (asynxModels.Command[domain.Workspace], error)

// SendWaitFunc dispatches the pure sync command and blocks until every matching
// projection has applied it (asynx SendWait), returning the applied event. It is
// decision 4's single sanctioned crash-recovery SendWait; in production it is
// axWorkspace.SendWait.
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

// New builds a Reconciler over the injected git+provider re-derivation (derive),
// the pure-command dispatcher (sendWait, in production axWorkspace.SendWait), and
// the corrected-frame broadcaster. Every dispatched task joins drainWG so a
// graceful shutdown can wait it out (decision 11). The concrete IO stays owned by
// the caller, so this package holds no git/provider dependency.
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

// OnOpen dispatches a background reconcile for wsID unless one is already in
// flight for that id (dedup — repeat opens do not stack tasks). It returns
// immediately: the re-derivation and SendWait run on a detached, timeout-bounded
// context so they outlive the triggering read (spec §3.8, decision 9). A blank id
// is ignored.
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
		// Draining: no run goroutine will spawn, so its deferred release never fires.
		// Undo the claim by hand or this wsID is wedged "in flight" until the process dies
		// and can never reconcile again. (Benign today — the gate only refuses during
		// shutdown — but a leaked claim that outlives a drain would be a silent wedge.)
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

// reconcile re-derives reality, folds it via a pure SendWait, then broadcasts the
// corrected frame. It is idempotent by construction: a vanished workspace
// (apperr.ErrNotFound) and a non-applicable transition (asynxmodels.ErrValidation)
// are both silent no-ops (spec §3.8).
func (r *Reconciler) reconcile(
	ctx context.Context,
	wsID string,
) {
	cmd, err := r.derive(ctx, wsID)
	if errors.Is(err, apperr.ErrNotFound) {
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
