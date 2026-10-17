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

// RunPreOpen runs the pending pre-open steps, in catalog order, before any
// database is open. A step failure halts the session and is returned as the
// *Halt; a cancelled ctx stops before the next step and is returned as is.
func (s *Session) RunPreOpen(
	ctx context.Context,
	impls Impls,
) error {
	return s.runPhase(ctx, PreOpen, impls)
}

// RunPostOpen runs the pending post-open steps and closes the run. It must
// follow RunPreOpen: it refuses to run while a pre-open step is still pending.
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
	if s.halted != nil {
		return s.halted
	}
	if err := s.admit(phase, impls); err != nil {
		return err
	}
	for {
		next := s.pending()
		if len(next) == 0 || next[0].Phase != phase {
			return s.finish()
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.runStep(ctx, next[0], impls[next[0].Number]); err != nil {
			return err
		}
	}
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
			return s.fail(HaltNoImpl, step, errors.New("no implementation in this build"))
		}
	}
	return nil
}

// checkKnown catches an implementation wired to a step the catalog lacks.
func (s *Session) checkKnown(
	impls Impls,
) error {
	known := make(map[int]bool, len(s.cat.steps))
	for _, step := range s.cat.steps {
		known[step.Number] = true
	}
	for _, n := range slices.Sorted(maps.Keys(impls)) {
		if !known[n] {
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
	run, err := s.beginRun()
	if err != nil {
		return err
	}
	s.progress.Store(&Progress{Phase: step.Phase, Step: step.ID(), Index: run.done + 1, Total: run.total})
	if err := s.led.append(s.stepRecord(kindStepStart, step)); err != nil {
		return fmt.Errorf("migrate: step %d %q: record start: %w", step.Number, step.Name, err)
	}
	s.opts.Log.InfoContext(ctx, "migrate: step started", "step", step.Number, "name", step.Name, "phase", step.Phase.String())
	cp := newCheckpoints(s.led, run.id, step.ID(), s.now)
	env := Env{Home: s.home, Log: s.opts.Log}
	counts, err := impl.Apply(ctx, env, cp)
	if err != nil {
		return s.fail(HaltStepFailed, step, err)
	}
	if impl.Verify != nil {
		if err := impl.Verify(ctx, env); err != nil {
			return s.fail(HaltVerifyFailed, step, err)
		}
	}
	return s.complete(ctx, step, cp, counts)
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

// beginRun starts the boot's run on the first step to run, so a boot with
// nothing to do, or one cancelled first, writes nothing.
func (s *Session) beginRun() (*runState, error) {
	if s.run != nil {
		return s.run, nil
	}
	l, err := s.openForAppend()
	if err != nil {
		return nil, fmt.Errorf("migrate: open ledger: %w", err)
	}
	pending := s.pending()
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
		return nil, fmt.Errorf("migrate: start run %s: %w", run.id, err)
	}
	s.run = run
	return run, nil
}

// finish closes the run once nothing is pending.
func (s *Session) finish() error {
	if s.run == nil || len(s.pending()) > 0 {
		return nil
	}
	return s.endRun(outcomeCompleted, "")
}

func (s *Session) endRun(
	outcome string,
	detail string,
) error {
	layout := s.layout
	err := s.led.append(record{
		Kind:    kindRunEnd,
		At:      s.now(),
		Run:     s.run.id,
		To:      &layout,
		Counts:  s.run.counts,
		Outcome: outcome,
		Error:   detail,
	})
	if err != nil {
		return fmt.Errorf("migrate: end run %s: %w", s.run.id, err)
	}
	s.run = nil
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

// fail halts the session on step. The halt is set and returned even when the
// ledger cannot record it: the ledger refuses every append once one failed.
func (s *Session) fail(
	reason HaltReason,
	step Step,
	cause error,
) error {
	h := s.newHalt(reason, step, cause)
	s.halted = h
	s.opts.Log.Error("migrate: halted", "reason", string(reason), "message", h.Message)
	if err := s.recordFailure(step, cause, h); err != nil {
		return errors.Join(h, err)
	}
	return h
}

// recordFailure ends the run the failure interrupted; with no run open there
// is nothing in the ledger to close.
func (s *Session) recordFailure(
	step Step,
	cause error,
	h *Halt,
) error {
	if s.run == nil {
		return nil
	}
	failed := s.stepRecord(kindStepFail, step)
	failed.Error = cause.Error()
	if err := s.led.append(failed); err != nil {
		return fmt.Errorf("migrate: record step failure: %w", err)
	}
	return s.endRun(outcomeFailed, h.Message)
}

func (s *Session) newHalt(
	reason HaltReason,
	step Step,
	cause error,
) *Halt {
	id := step.ID()
	h := &Halt{
		Reason:  reason,
		Message: haltMessage(reason, step, cause),
		Step:    &id,
		Phase:   step.Phase.String(),
		Layout:  s.layout,
		Known:   s.cat.Known(),
		Ledger:  s.ledgerPath(),
	}
	if s.run != nil {
		h.Run = s.run.id
	}
	return h
}

// haltVerbs say what happened to a step; a reason without one means it failed.
var haltVerbs = map[HaltReason]string{
	HaltVerifyFailed: "failed verification",
	HaltNoImpl:       "cannot run",
}

func haltMessage(
	reason HaltReason,
	step Step,
	cause error,
) string {
	what, ok := haltVerbs[reason]
	if !ok {
		what = "failed"
	}
	return fmt.Sprintf("migration step %d %q %s: %v", step.Number, step.Name, what, cause)
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
