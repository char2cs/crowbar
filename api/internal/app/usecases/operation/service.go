// Package operation runs work that outlives its request as an operation: a row
// with an id, a state and an outcome, published on every change.
package operation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// Spec describes an operation to start. Validate, when set, runs only for a
// key the service has not seen, so a retry is never refused by what the first
// attempt created.
type Spec struct {
	ID        string
	Kind      domain.OperationKind
	Subject   domain.EntityRef
	ProjectID string
	Label     string
	Items     []domain.OperationItem
	Validate  func(context.Context) error
}

// Progress is how work reports an item: one with the same Label is replaced,
// any other is appended.
type Progress interface {
	Item(item domain.OperationItem)
}

// Work is what an operation runs.
type Work func(ctx context.Context, p Progress) error

const interruptedMessage = "Crowbar stopped before this finished"

// Service records operations and runs their work.
type Service struct {
	store   store.Store[domain.Operation, string]
	publish func(domain.Operation)
	clock   func() time.Time

	// mu orders every write, so versions reach the store and the stream in order.
	mu       sync.Mutex
	stopping bool
	saveErrs []error

	work    sync.WaitGroup
	cancels map[*context.CancelFunc]struct{}
	// busy is the running hierarchy verb per workspace, guarded by mu.
	busy map[domain.EntityRef]string
}

// New builds the service over the operations store; publish hears every saved row.
func New(
	opStore store.Store[domain.Operation, string],
	publish func(domain.Operation),
	clock func() time.Time,
) *Service {
	return &Service{
		store: opStore, publish: publish, clock: clock,
		cancels: map[*context.CancelFunc]struct{}{}, busy: map[domain.EntityRef]string{},
	}
}

// Start records spec as accepted and runs work detached from ctx. A key seen
// before returns that operation, whatever its state, and runs nothing.
func (s *Service) Start(
	ctx context.Context,
	spec Spec,
	work Work,
) (domain.Operation, error) {
	op, run, err := s.admit(context.WithoutCancel(ctx), spec, domain.OpAccepted, nil)
	if err != nil || run == nil {
		return op, err
	}
	go func() {
		defer run.release()
		s.execute(run.ctx, op, work)
	}()
	return op, nil
}

// Run is Start on the caller's goroutine, returning the settled operation. It
// is for boot, where nothing may be served before the work is done.
func (s *Service) Run(
	ctx context.Context,
	spec Spec,
	work Work,
) (domain.Operation, error) {
	op, run, err := s.admit(ctx, spec, domain.OpAccepted, nil)
	if err != nil || run == nil {
		return op, err
	}
	defer run.release()
	// Boot runs it before anything can stop the service, so the caller's ctx is its ctx.
	return s.execute(ctx, op, work), nil
}

// Fail records work the daemon started that has already failed with cause.
func (s *Service) Fail(
	ctx context.Context,
	spec Spec,
	cause error,
) (domain.Operation, error) {
	failure := problem.Of(cause)
	op, run, err := s.admit(ctx, spec, domain.OpFailed, &failure)
	if run != nil {
		// Before release, so a Shutdown that returns has no prune left in flight.
		s.mu.Lock()
		s.pruneLocked(ctx)
		s.mu.Unlock()
		run.release()
	}
	return op, err
}

// Dismiss marks a settled operation dismissed.
func (s *Service) Dismiss(
	ctx context.Context,
	id string,
) (domain.Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	found, err := s.store.FindByKey(ctx, id)
	if err != nil {
		return domain.Operation{}, fmt.Errorf("operation: dismiss %s: %w", id, err)
	}
	if found == nil {
		return domain.Operation{}, &problem.Error{Code: problem.NotFound, Message: "no operation " + id}
	}
	if !settled(found.State) {
		return domain.Operation{}, &problem.Error{Code: problem.OperationRunning, Message: "operation " + id + " is still running"}
	}
	op := *found
	op.Dismissed = true
	if err := s.saveLocked(ctx, &op); err != nil {
		return domain.Operation{}, err
	}
	return op, nil
}

// DismissFailed dismisses every failed kind operation naming subject: work the
// daemon finished later, such as a purge the boot sweep completed.
func (s *Service) DismissFailed(
	ctx context.Context,
	kind domain.OperationKind,
	subject domain.EntityRef,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ops, err := s.store.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("operation: dismiss %s of %s: %w", kind, subject.ID, err)
	}
	for _, op := range ops {
		if op.Kind != kind || op.Subject != subject || op.State != domain.OpFailed || op.Dismissed {
			continue
		}
		op.Dismissed = true
		if err := s.saveLocked(ctx, &op); err != nil {
			return err
		}
	}
	return nil
}

// Snapshot is every retained operation, oldest change first.
func (s *Service) Snapshot(ctx context.Context) ([]domain.Operation, error) {
	ops, err := s.store.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("operation: snapshot: %w", err)
	}
	sort.SliceStable(ops, func(i, j int) bool { return ops[i].UpdatedAt.Before(ops[j].UpdatedAt) })
	return ops, nil
}

// Shutdown refuses later Starts and cancels running work at once, which
// settles as interrupted; it waits for that work until ctx ends. Work that
// ignores cancel (a git lock wait) is left behind rather than eat the drain.
func (s *Service) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.stopping = true
	for cancel := range s.cancels {
		(*cancel)()
	}
	s.mu.Unlock()
	done := make(chan struct{})
	go func() {
		s.work.Wait()
		close(done)
	}()
	var waitErr error
	select {
	case <-done:
	case <-ctx.Done():
		waitErr = fmt.Errorf("operation: shutdown: work still running: %w", ctx.Err())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return errors.Join(append(slices.Clone(s.saveErrs), waitErr)...)
}

// admitted is a new operation's work context, cancellable by Shutdown from
// the moment admit returns; release ends it.
type admitted struct {
	ctx     context.Context
	release func()
}

// admit applies the key rules and, for a new key, saves the first row in
// state and returns its work context, which holds one count on s.work.
func (s *Service) admit(
	ctx context.Context,
	spec Spec,
	state domain.OperationState,
	failure *problem.Problem,
) (domain.Operation, *admitted, error) {
	id, err := operationID(spec.ID)
	if err != nil {
		return domain.Operation{}, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping {
		return domain.Operation{}, nil, &problem.Error{Code: problem.Unavailable, Message: "Crowbar is stopping"}
	}
	known, err := s.store.FindByKey(ctx, id)
	if err != nil {
		return domain.Operation{}, nil, fmt.Errorf("operation: find %s: %w", id, err)
	}
	if known != nil {
		if known.Kind != spec.Kind || known.Subject != spec.Subject {
			return domain.Operation{}, nil, &problem.Error{
				Code:    problem.OperationIDConflict,
				Message: fmt.Sprintf("operation %s is a %s of %s %s", id, known.Kind, known.Subject.Kind, known.Subject.ID),
			}
		}
		// Same key, kind and subject is the same request whatever its body (R3).
		return *known, nil, nil
	}
	holds := exclusive(spec.Kind) && state == domain.OpAccepted
	if holds {
		if err := s.refuseBusyLocked(ctx, spec.Subject); err != nil {
			return domain.Operation{}, nil, err
		}
	}
	if spec.Validate != nil {
		if err := spec.Validate(ctx); err != nil {
			return domain.Operation{}, nil, err
		}
	}
	now := s.clock()
	op := domain.Operation{
		ID: id, Kind: spec.Kind, Subject: spec.Subject, ProjectID: spec.ProjectID, Label: spec.Label,
		State: state, Items: append([]domain.OperationItem{}, spec.Items...), Error: failure, CreatedAt: now,
	}
	if err := s.saveLocked(ctx, &op); err != nil {
		return domain.Operation{}, nil, err
	}
	// Registered under mu with the stopping check, so no Shutdown misses it.
	workCtx, cancel := context.WithCancel(ctx)
	s.cancels[&cancel] = struct{}{}
	s.work.Add(1)
	if holds {
		s.busy[spec.Subject] = id
	}
	return op, &admitted{ctx: workCtx, release: func() {
		s.mu.Lock()
		delete(s.cancels, &cancel)
		if holds && s.busy[spec.Subject] == id {
			delete(s.busy, spec.Subject)
		}
		s.mu.Unlock()
		cancel()
		s.work.Done()
	}}, nil
}

// exclusive kinds rewrite a workspace's branch, parent or worktree, so one runs
// per workspace at a time. git.* verbs are not among them: git serialises those
// itself, and a push beside a sync is legitimate.
func exclusive(kind domain.OperationKind) bool {
	return strings.HasPrefix(string(kind), "workspace.")
}

// refuseBusyLocked refuses subject while its hierarchy verb runs. The holder
// is released after its terminal frame, so a settled row means free.
func (s *Service) refuseBusyLocked(
	ctx context.Context,
	subject domain.EntityRef,
) error {
	holder, held := s.busy[subject]
	if !held {
		return nil
	}
	row, err := s.store.FindByKey(ctx, holder)
	if err != nil {
		return fmt.Errorf("operation: find %s: %w", holder, err)
	}
	if row != nil && settled(row.State) {
		return nil
	}
	return &problem.Error{
		Code:    problem.OperationRunning,
		Message: fmt.Sprintf("%s %s is busy with operation %s", subject.Kind, subject.ID, holder),
	}
}

func operationID(key string) (string, error) {
	if key == "" {
		return uuid.NewString(), nil
	}
	if _, err := uuid.Parse(key); err != nil {
		return "", &problem.Error{Code: problem.InvalidArgument, Message: "operation id " + key + " is not a UUID", Err: err}
	}
	return key, nil
}

// execute runs work and settles op from its outcome.
func (s *Service) execute(
	ctx context.Context,
	op domain.Operation,
	work Work,
) domain.Operation {
	run := &running{s: s, ctx: ctx, op: op}
	run.update(func(o *domain.Operation) { o.State = domain.OpRunning })
	err := call(ctx, op.Kind, run, work)
	run.update(func(o *domain.Operation) { settle(ctx, o, err) })
	s.mu.Lock()
	s.pruneLocked(ctx)
	s.mu.Unlock()
	return run.snapshot()
}

// call runs work, turning a panic into the operation's failure.
func call(
	ctx context.Context,
	kind domain.OperationKind,
	p Progress,
	work Work,
) (err error) {
	defer safego.RecoverFn("operation."+string(kind), func(r any) {
		err = &problem.Error{Code: problem.Internal, Message: fmt.Sprintf("internal error: %v", r)}
	})
	return work(ctx, p)
}

func settle(
	ctx context.Context,
	op *domain.Operation,
	err error,
) {
	op.State = domain.OpFailed
	switch failed := failedItems(op.Items); {
	case err != nil && ctx.Err() != nil:
		op.Error = &problem.Problem{Code: problem.Interrupted, Message: interruptedMessage}
	case err != nil:
		p := problem.Of(err)
		op.Error = &p
	case failed > 0:
		op.Error = &problem.Problem{Code: problem.Partial, Message: fmt.Sprintf("%d of %d failed", failed, len(op.Items))}
	default:
		op.State = domain.OpSucceeded
	}
}

func failedItems(items []domain.OperationItem) int {
	n := 0
	for _, item := range items {
		if item.Error != nil || item.State == domain.OpFailed {
			n++
		}
	}
	return n
}

func settled(state domain.OperationState) bool {
	return state == domain.OpSucceeded || state == domain.OpFailed
}

// saveLocked bumps op's version, saves and publishes it; s.mu is held. The
// save outlives a cancelled ctx: the outcome of interrupted work is recorded too.
func (s *Service) saveLocked(
	ctx context.Context,
	op *domain.Operation,
) error {
	op.Version++
	op.UpdatedAt = s.clock()
	if err := s.store.Save(context.WithoutCancel(ctx), *op); err != nil {
		return fmt.Errorf("operation: save %s: %w", op.ID, err)
	}
	published := *op
	published.Items = slices.Clone(op.Items)
	s.publish(published)
	return nil
}

// running is one operation's in-memory row while its work runs.
type running struct {
	s   *Service
	ctx context.Context
	op  domain.Operation
}

func (r *running) Item(item domain.OperationItem) {
	r.update(func(o *domain.Operation) {
		for i := range o.Items {
			if o.Items[i].Label == item.Label {
				o.Items[i] = item
				return
			}
		}
		o.Items = append(o.Items, item)
	})
}

// update applies change and saves; a failed save has no caller to return to,
// so it is logged and kept for Shutdown.
func (r *running) update(change func(*domain.Operation)) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	change(&r.op)
	if err := r.s.saveLocked(r.ctx, &r.op); err != nil {
		slog.ErrorContext(r.ctx, "operation: record a change", "op", r.op.ID, "kind", r.op.Kind, "err", err)
		r.s.saveErrs = append(r.s.saveErrs, err)
	}
}

func (r *running) snapshot() domain.Operation {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	return r.op
}
