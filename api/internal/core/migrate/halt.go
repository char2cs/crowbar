package migrate

import (
	"errors"
	"fmt"
	"os"
)

// HaltReason says why a session stopped the boot.
type HaltReason string

const (
	// HaltLayoutNewer means LAYOUT is ahead of what this binary knows.
	HaltLayoutNewer HaltReason = "layout_newer"
	// HaltLayoutUnreadable means LAYOUT exists but is not a layout number.
	HaltLayoutUnreadable HaltReason = "layout_unreadable"
	// HaltLedgerCorrupt means a ledger line other than a torn tail is unreadable.
	HaltLedgerCorrupt HaltReason = "ledger_corrupt"
	// HaltPlanFailed means a pending step could not compute its plan.
	HaltPlanFailed HaltReason = "plan_failed"
	// HaltPreflight means a precondition (disk, git) for the pending steps failed.
	HaltPreflight HaltReason = "preflight_failed"
	// HaltStepFailed means a step's Apply, or recording its result, failed.
	HaltStepFailed HaltReason = "step_failed"
	// HaltVerifyFailed means a step applied but its Verify failed.
	HaltVerifyFailed HaltReason = "verify_failed"
	// HaltNoImpl means a pending step has no implementation in this binary.
	HaltNoImpl HaltReason = "no_implementation"
)

// Halt is the reason a session refused to let the daemon open its stores. It
// is also the JSON the daemon serves while halted.
type Halt struct {
	Reason  HaltReason `json:"reason"`
	Message string     `json:"message"`
	Step    *ID        `json:"step,omitempty"`
	Phase   string     `json:"phase,omitempty"`
	Layout  int        `json:"layout"`
	Known   int        `json:"known"`
	Run     string     `json:"run,omitempty"`
	Ledger  string     `json:"ledger,omitempty"`
	Backup  string     `json:"backup,omitempty"`
}

// Error returns the halt's message, so a Halt can travel as an error.
func (h *Halt) Error() string {
	return h.Message
}

// refuse halts the session for a reason no step is to blame for: the home itself
// cannot be taken on. It writes nothing, so a refused home stays as it was found.
func (s *Session) refuse(
	reason HaltReason,
	message string,
) {
	h := s.baseHalt(reason, message)
	s.halted.Store(h)
	s.opts.Log.Error("migrate: halted", "reason", string(reason), "message", message)
}

// fail halts the session on a step that was running, or about to. The halt is
// set and returned even when the ledger cannot record it: the ledger refuses
// every append once one failed.
func (s *Session) fail(
	reason HaltReason,
	step Step,
	cause error,
) error {
	return s.stop(reason, step, cause, true)
}

// failBeforeSteps halts on step before any step has run, as plan and preflight
// do: the run ends failed, with no step.fail, since no step failed.
func (s *Session) failBeforeSteps(
	reason HaltReason,
	step Step,
	cause error,
) error {
	return s.stop(reason, step, cause, false)
}

func (s *Session) stop(
	reason HaltReason,
	step Step,
	cause error,
	stepRan bool,
) error {
	h := s.newHalt(reason, step, cause)
	s.halted.Store(h)
	s.opts.Log.Error("migrate: halted", "reason", string(reason), "message", h.Message)
	if err := s.recordFailure(step, cause, h, stepRan); err != nil {
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
	stepRan bool,
) error {
	if s.run == nil {
		return nil
	}
	if stepRan {
		failed := s.stepRecord(kindStepFail, step)
		failed.Error = cause.Error()
		if err := s.led.append(failed); err != nil {
			return fmt.Errorf("migrate: record step failure: %w", err)
		}
	}
	return s.endRun(outcomeFailed, h.Message)
}

func (s *Session) newHalt(
	reason HaltReason,
	step Step,
	cause error,
) *Halt {
	h := s.baseHalt(reason, haltMessage(reason, step, cause))
	id := step.ID()
	h.Step, h.Phase = &id, step.Phase.String()
	return h
}

// baseHalt is a halt with everything the session itself knows. Backup is the
// copy of state/ the migration in progress started from, if it took one.
func (s *Session) baseHalt(
	reason HaltReason,
	message string,
) *Halt {
	h := &Halt{
		Reason:  reason,
		Message: message,
		Layout:  s.layout,
		Known:   s.cat.Known(),
		Backup:  s.view().backupInForce(),
	}
	if s.run != nil {
		h.Run = s.run.id
	}
	if _, err := os.Stat(s.ledgerPath()); err == nil {
		h.Ledger = s.ledgerPath()
	}
	return h
}

// haltVerbs say what happened to a step; a reason without one means it failed.
var haltVerbs = map[HaltReason]string{
	HaltPlanFailed:   "could not be planned",
	HaltPreflight:    "cannot run",
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
