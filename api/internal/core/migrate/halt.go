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

// fail halts the session on step. The halt is set and returned even when the
// ledger cannot record it: the ledger refuses every append once one failed.
func (s *Session) fail(
	reason HaltReason,
	step Step,
	cause error,
) error {
	h := s.newHalt(reason, step, cause)
	s.halted.Store(h)
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
