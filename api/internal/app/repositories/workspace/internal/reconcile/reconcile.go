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

type DeriveFunc func(
	ctx context.Context,
	wsID string,
) (asynxModels.Command[domain.Workspace], error)

type SendWaitFunc func(
	ctx context.Context,
	cmd asynxModels.Command[domain.Workspace],
) (asynxModels.Event[domain.Workspace], error)

type Opt func(*Reconciler)

func WithTimeout(
	d time.Duration,
) Opt {
	return func(r *Reconciler) {
		if d > 0 {
			r.timeout = d
		}
	}
}

type Reconciler struct {
	derive    DeriveFunc
	sendWait  SendWaitFunc
	broadcast func(ws domain.Workspace)
	gate      *drain.Gate

	timeout time.Duration

	mu       sync.Mutex
	inflight map[string]struct{}
}

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
