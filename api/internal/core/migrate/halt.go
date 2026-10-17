package migrate

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
