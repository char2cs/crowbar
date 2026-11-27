package migrate

import (
	"errors"
	"fmt"
)

type HaltReason string

const (
	HaltLayoutNewer      HaltReason = "layout_newer"
	HaltLayoutUnreadable HaltReason = "layout_unreadable"
	HaltLedgerCorrupt    HaltReason = "ledger_corrupt"
	HaltIOFailed         HaltReason = "io_failed"
	HaltPlanFailed       HaltReason = "plan_failed"
	HaltPreflight        HaltReason = "preflight_failed"
	HaltStepFailed       HaltReason = "step_failed"
	HaltVerifyFailed     HaltReason = "verify_failed"
	HaltNoImpl           HaltReason = "no_implementation"
)

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

func (h *Halt) Error() string {
	return h.Message
}

func (s *Session) refuse(
	reason HaltReason,
	message string,
) {
	s.halt(s.baseHalt(reason, message))
}

func (s *Session) halt(
	h *Halt,
) {
	s.halted.Store(h)
	s.opts.Log.Info("migrate: halted", "reason", string(h.Reason), "message", h.Message)
}

func (s *Session) fail(
	reason HaltReason,
	step Step,
	cause error,
) error {
	return s.stop(reason, step, cause, true)
}

func (s *Session) failUnstarted(
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
	s.opts.Log.Info("migrate: halted", "reason", string(reason), "message", h.Message, "err", cause)
	if err := s.recordFailure(step, cause, h, stepRan); err != nil {
		return errors.Join(h, err)
	}
	return h
}

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
	return s.baseHalt(reason, haltMessage(reason, step, s.opts.plain(cause))).at(step)
}

func (s *Session) baseHalt(
	reason HaltReason,
	message string,
) *Halt {
	h := haltAt(s.home, s.cat, s.layout, s.view(), reason, message)
	if s.run != nil {
		h.Run = s.run.id
	}
	return h
}

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
