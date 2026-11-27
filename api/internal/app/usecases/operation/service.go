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

type Spec struct {
	ID        string
	Kind      domain.OperationKind
	Subject   domain.EntityRef
	ProjectID string
	Label     string
	Items     []domain.OperationItem
	Validate  func(context.Context) error
}

type Progress interface {
	Item(item domain.OperationItem)
}

type Work func(ctx context.Context, p Progress) error

const interruptedMessage = "Crowbar stopped before this finished"

type Service struct {
	store   store.Store[domain.Operation, string]
	publish func(domain.Operation)
	clock   func() time.Time
	booted  time.Time

	mu       sync.Mutex
	stopping bool
	saveErrs []error

	work    sync.WaitGroup
	cancels map[*context.CancelFunc]struct{}
	busy    map[domain.EntityRef]string
}

func New(
	opStore store.Store[domain.Operation, string],
	publish func(domain.Operation),
	clock func() time.Time,
) *Service {
	return &Service{
		store: opStore, publish: publish, clock: clock, booted: clock(),
		cancels: map[*context.CancelFunc]struct{}{}, busy: map[domain.EntityRef]string{},
	}
}

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
	return s.execute(ctx, op, work), nil
}

func (s *Service) Fail(
	ctx context.Context,
	spec Spec,
	cause error,
) (domain.Operation, error) {
	failure := problem.Of(cause)
	op, run, err := s.admit(ctx, spec, domain.OpFailed, &failure)
	if run != nil {
		logFailure(ctx, op, cause)
		s.mu.Lock()
		s.pruneLocked(ctx)
		s.mu.Unlock()
		run.release()
	}
	return op, err
}

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

func (s *Service) Snapshot(ctx context.Context) ([]domain.Operation, error) {
	ops, err := s.store.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("operation: snapshot: %w", err)
	}
	sort.SliceStable(ops, func(i, j int) bool { return ops[i].UpdatedAt.Before(ops[j].UpdatedAt) })
	return ops, nil
}

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

type admitted struct {
	ctx     context.Context
	release func()
}

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
	known, err := s.knownLocked(ctx, id, spec)
	if err != nil || known != nil {
		return deref(known), nil, err
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
	settleItems(&op)
	if err := s.saveLocked(ctx, &op); err != nil {
		return domain.Operation{}, nil, err
	}
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

func (s *Service) Replay(
	ctx context.Context,
	key string,
	kind domain.OperationKind,
	subject domain.EntityRef,
) (domain.Operation, bool, error) {
	if key == "" {
		return domain.Operation{}, false, nil
	}
	id, err := operationID(key)
	if err != nil {
		return domain.Operation{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	known, err := s.knownLocked(ctx, id, Spec{Kind: kind, Subject: subject})
	if err != nil || known == nil {
		return domain.Operation{}, false, err
	}
	return *known, true, nil
}

func (s *Service) knownLocked(
	ctx context.Context,
	id string,
	spec Spec,
) (*domain.Operation, error) {
	known, err := s.store.FindByKey(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("operation: find %s: %w", id, err)
	}
	if known != nil && (known.Kind != spec.Kind || known.Subject != spec.Subject) {
		return nil, &problem.Error{
			Code:    problem.OperationIDConflict,
			Message: fmt.Sprintf("operation %s is a %s of %s %s", id, known.Kind, known.Subject.Kind, known.Subject.ID),
		}
	}
	return known, nil
}

func deref(op *domain.Operation) domain.Operation {
	if op == nil {
		return domain.Operation{}
	}
	return *op
}

func exclusive(kind domain.OperationKind) bool {
	return strings.HasPrefix(string(kind), "workspace.") ||
		kind == domain.OpRepoDelete || kind == domain.OpProjectDelete
}

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

func (s *Service) execute(
	ctx context.Context,
	op domain.Operation,
	work Work,
) domain.Operation {
	op.Items = slices.Clone(op.Items)
	run := &running{s: s, ctx: ctx, op: op}
	run.update(func(o *domain.Operation) { o.State = domain.OpRunning })
	err := call(ctx, op.Kind, run, work)
	logFailure(ctx, op, err)
	run.update(func(o *domain.Operation) { settle(ctx, o, err) })
	s.mu.Lock()
	s.pruneLocked(ctx)
	s.mu.Unlock()
	return run.snapshot()
}

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

func settled(state domain.OperationState) bool {
	return state == domain.OpSucceeded || state == domain.OpFailed
}

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
