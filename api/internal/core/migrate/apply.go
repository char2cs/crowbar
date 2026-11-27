package migrate

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
)

const runIDLayout = "20060102T150405.000000000Z"

type runState struct {
	id     string
	total  int
	done   int
	counts Counts
}

func (s *Session) RunPreOpen(
	ctx context.Context,
	impls Impls,
) error {
	return s.runPhase(ctx, PreOpen, impls)
}

func (s *Session) RunPostOpen(
	ctx context.Context,
	impls Impls,
) error {
	return s.runPhase(ctx, PostOpen, impls)
}

func (s *Session) runPhase(
	ctx context.Context,
	phase Phase,
	impls Impls,
) error {
	if h := s.halted.Load(); h != nil {
		return h
	}
	if err := s.admit(phase, impls); err != nil {
		return err
	}
	if err := s.ensureRun(ctx, phase); err != nil {
		return err
	}
	for {
		next := s.pending()
		if len(next) == 0 || next[0].Phase != phase {
			return s.finish()
		}
		if err := ctx.Err(); err != nil {
			return s.interrupt(err)
		}
		if err := s.runStep(ctx, next[0], impls[next[0].Number]); err != nil {
			return err
		}
	}
}

func (s *Session) ensureRun(
	ctx context.Context,
	phase Phase,
) error {
	pending := s.pending()
	if len(pending) == 0 || s.run != nil {
		return nil
	}
	if phase == PostOpen {
		return errors.New("migrate: post-open steps are pending but RunPreOpen has not started a run")
	}
	if err := s.beginRun(pending); err != nil {
		return err
	}
	return s.preflight(ctx, pending)
}

func (s *Session) admit(
	phase Phase,
	impls Impls,
) error {
	if err := s.checkKnown(impls); err != nil {
		return err
	}
	pending := s.pending()
	if phase == PostOpen && len(pending) > 0 && pending[0].Phase == PreOpen {
		return fmt.Errorf("migrate: post-open steps cannot run while pre-open step %d %q is pending",
			pending[0].Number, pending[0].Name)
	}
	if step, ok := unimplemented(leading(pending, phase), impls.implements); ok {
		return s.failUnstarted(HaltNoImpl, step, errNoImpl)
	}
	return nil
}

var errNoImpl = errors.New("no implementation in this build")

func unimplemented(
	steps []Step,
	implemented func(number int) bool,
) (Step, bool) {
	i := slices.IndexFunc(steps, func(s Step) bool { return !implemented(s.Number) })
	if i < 0 {
		return Step{}, false
	}
	return steps[i], true
}

func (s *Session) checkKnown(
	impls Impls,
) error {
	for _, n := range slices.Sorted(maps.Keys(impls)) {
		if !s.cat.has(n) {
			return fmt.Errorf("migrate: unknown implementation for step %d: the catalog has no such step", n)
		}
	}
	return nil
}

func leading(
	pending []Step,
	phase Phase,
) []Step {
	n := 0
	for n < len(pending) && pending[n].Phase == phase {
		n++
	}
	return pending[:n]
}

func (s *Session) runStep(
	ctx context.Context,
	step Step,
	impl Impl,
) error {
	run := s.run
	s.progress.Store(&Progress{Phase: step.Phase, Step: step.ID(), Index: run.done + 1, Total: run.total})
	if err := s.led.append(s.stepRecord(kindStepStart, step)); err != nil {
		return s.fail(HaltStepFailed, step, fmt.Errorf("record start: %w", err))
	}
	s.opts.Log.InfoContext(ctx, "migrate: step started", "step", step.Number, "name", step.Name, "phase", step.Phase.String())
	env := s.env()
	cp := newCheckpoints(s.led, run.id, step.ID(), s.now)
	counts, err := impl.Apply(ctx, env, cp)
	if err != nil {
		return s.attemptFailed(ctx, HaltStepFailed, step, cp, err)
	}
	if impl.Verify == nil {
		return s.complete(ctx, step, cp, counts)
	}
	if err := impl.Verify(ctx, env); err != nil {
		return s.attemptFailed(ctx, HaltVerifyFailed, step, cp, err)
	}
	return s.complete(ctx, step, cp, counts)
}

func (s *Session) attemptFailed(
	ctx context.Context,
	reason HaltReason,
	step Step,
	cp *Checkpoints,
	err error,
) error {
	if s.again[step.Number] && !interrupted(ctx, err) {
		return s.skipAgain(ctx, step, cp, err)
	}
	return s.stepError(ctx, reason, step, err)
}

func (s *Session) stepError(
	ctx context.Context,
	reason HaltReason,
	step Step,
	err error,
) error {
	if interrupted(ctx, err) {
		return s.interrupt(err)
	}
	return s.fail(reason, step, err)
}

func interrupted(
	ctx context.Context,
	err error,
) bool {
	return err != nil && ctx.Err() != nil && errors.Is(err, ctx.Err())
}

func (s *Session) complete(
	ctx context.Context,
	step Step,
	cp *Checkpoints,
	counts Counts,
) error {
	if step.bumps() {
		if err := writeLayout(s.home, step.To); err != nil {
			return s.fail(HaltStepFailed, step, err)
		}
		s.layout = step.To
	}
	total := cp.total()
	total.Add(counts)
	done := s.stepRecord(kindStepDone, step)
	done.Counts, done.Outcome = total, outcomeApplied
	if err := s.led.append(done); err != nil {
		return s.fail(HaltStepFailed, step, fmt.Errorf("record step.done: %w", err))
	}
	delete(s.again, step.Number)
	s.run.done++
	s.run.counts.Add(total)
	s.opts.Log.InfoContext(ctx, "migrate: step done", "step", step.Number, "name", step.Name, "counts", total)
	return nil
}

const skippedNote = "skipped"

func (s *Session) skipAgain(
	ctx context.Context,
	step Step,
	cp *Checkpoints,
	cause error,
) error {
	s.opts.Log.WarnContext(ctx, "migrate: a step that runs again failed; it is skipped this boot",
		"step", step.Number, "name", step.Name, "err", cause)
	failed := s.stepRecord(kindStepFail, step)
	failed.Error = cause.Error()
	if err := errors.Join(s.led.append(failed), cp.Note(skippedNote, Note{Detail: cause.Error()})); err != nil {
		return s.fail(HaltStepFailed, step, fmt.Errorf("record the skip: %w", err))
	}
	delete(s.again, step.Number)
	s.run.done++
	return nil
}

func (s *Session) beginRun(
	pending []Step,
) error {
	l, err := s.openForAppend()
	if err != nil {
		return s.fail(HaltStepFailed, pending[0], fmt.Errorf("open ledger: %w", err))
	}
	at := s.now()
	run := &runState{id: at.Format(runIDLayout), total: len(pending), counts: Counts{}}
	layout := s.layout
	err = l.append(record{
		Kind:    kindRunStart,
		At:      at,
		Run:     run.id,
		Version: s.opts.Version,
		From:    &layout,
		Pending: stepNumbers(pending),
	})
	if err != nil {
		return s.fail(HaltStepFailed, pending[0], fmt.Errorf("start run %s: %w", run.id, err))
	}
	s.run = run
	return nil
}

func (s *Session) finish() error {
	if s.run == nil || len(s.pending()) > 0 {
		return nil
	}
	if err := s.endRun(outcomeCompleted, ""); err != nil {
		s.opts.Log.Warn("migrate: every step is done but the run could not be closed", "run", s.run.id, "err", err)
		s.run = nil
	}
	return nil
}

func (s *Session) interrupt(
	cause error,
) error {
	s.opts.Log.Info("migrate: run interrupted", "run", s.run.id, "cause", cause)
	if err := s.endRun(outcomeInterrupted, cause.Error()); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func (s *Session) endRun(
	outcome string,
	detail string,
) error {
	if err := s.writeRunEnd(s.run.id, s.run.counts, outcome, detail); err != nil {
		return err
	}
	s.run = nil
	return nil
}

func (s *Session) writeRunEnd(
	id string,
	counts Counts,
	outcome string,
	detail string,
) error {
	err := s.led.append(runEnd{run: id, outcome: outcome, detail: detail}.record(s.now(), s.layout, counts))
	if err != nil {
		return fmt.Errorf("migrate: end run %s: %w", id, err)
	}
	return nil
}

func (s *Session) stepRecord(
	kind string,
	step Step,
) record {
	return record{
		Kind:  kind,
		At:    s.now(),
		Run:   s.run.id,
		Step:  step.Number,
		Name:  step.Name,
		Phase: step.Phase.String(),
	}
}

func stepNumbers(
	steps []Step,
) []int {
	out := make([]int, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.Number)
	}
	return out
}
