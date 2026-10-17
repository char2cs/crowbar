package migrate

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
)

// runIDLayout names a run by its start time. It is one path segment because the
// run id also names the run's backup directory.
const runIDLayout = "20060102T150405.000000000Z"

// runState is the run in progress: how many steps it will run, how many it has,
// and what they counted.
type runState struct {
	id     string
	total  int
	done   int
	counts Counts
}

// RunPreOpen starts the boot's run when any step is pending, preflights every
// pending step of both phases (plans, git, the state backup), then runs the
// pending pre-open steps, in catalog order, before any database is open. A
// failure halts the session and is returned as the *Halt; a ctx cancelled
// before or during a step ends the run as interrupted, and its error is
// returned as is.
func (s *Session) RunPreOpen(
	ctx context.Context,
	impls Impls,
) error {
	return s.runPhase(ctx, PreOpen, impls)
}

// RunPostOpen runs the pending post-open steps and closes the run RunPreOpen
// started. It refuses to run while a pre-open step is pending or no run is open.
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

// ensureRun opens the boot's run the first time anything is pending, whichever
// phase it is in, and preflights every pending step before any of them runs.
// Only RunPreOpen may start it: RunPostOpen runs after the stores are open.
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

// admit refuses a phase that cannot run in full, before it changes anything.
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
	for _, step := range leading(pending, phase) {
		if impls[step.Number].Apply == nil {
			return s.failUnstarted(HaltNoImpl, step, errors.New("no implementation in this build"))
		}
	}
	return nil
}

// checkKnown catches an implementation wired to a step the catalog lacks.
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

// leading is the steps at the front of the pending order that belong to phase.
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
	cp := newCheckpoints(s.led, run.id, step.ID(), s.now)
	env := Env{Home: s.home, Log: s.opts.Log}
	counts, err := impl.Apply(ctx, env, cp)
	if err != nil {
		return s.stepError(ctx, HaltStepFailed, step, err)
	}
	if impl.Verify != nil {
		if err := impl.Verify(ctx, env); err != nil {
			return s.stepError(ctx, HaltVerifyFailed, step, err)
		}
	}
	return s.complete(ctx, step, cp, counts)
}

// stepError ends the run as interrupted when err is the step stopping for a
// cancelled ctx, and halts on it otherwise.
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

// interrupted reports whether err is a call giving up because ctx was cancelled.
// An error that merely arrives while ctx is cancelled is a failure of its own.
func interrupted(
	ctx context.Context,
	err error,
) bool {
	return err != nil && ctx.Err() != nil && errors.Is(err, ctx.Err())
}

// complete makes a verified step durable: LAYOUT first for a bump, since LAYOUT
// alone decides a bump is done, then the ledger's record of it.
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
	s.run.done++
	s.run.counts.Add(total)
	s.opts.Log.InfoContext(ctx, "migrate: step done", "step", step.Number, "name", step.Name, "counts", total)
	return nil
}

// beginRun writes run.start for the steps pending now. A ledger that cannot be
// written halts on the step that would have run first: the boot must not go on
// unrecorded, and a halt is what the health endpoint can report.
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

// finish closes the run once nothing is pending. Every step is already durable
// by then, so a ledger that cannot take the run.end does not undo the migration;
// the next boot closes the dangling run.
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

// interrupt closes the run a cancelled ctx stopped. It is not a failure, so
// there is no halt; the next boot resumes from the checkpoints.
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

// writeRunEnd appends the record that closes run id, whether or not this
// session started it.
func (s *Session) writeRunEnd(
	id string,
	counts Counts,
	outcome string,
	detail string,
) error {
	layout := s.layout
	err := s.led.append(record{
		Kind:    kindRunEnd,
		At:      s.now(),
		Run:     id,
		To:      &layout,
		Counts:  counts,
		Outcome: outcome,
		Error:   detail,
	})
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
