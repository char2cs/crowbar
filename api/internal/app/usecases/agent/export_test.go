package agent

// WaitingForTurnLog is the log record a provider switch emits at the INSTANT it parks on
// an in-flight turn, exposed to this package's (external) tests. It is not production
// surface: this file is compiled only under `go test`.
//
// It exists so a test can block on THE SWITCH BEING PARKED — a real, causal signal —
// instead of sleeping and hoping. That matters more here than anywhere else in the
// package: the property under test is a NEGATIVE ("the outgoing CLI is not killed while
// the turn is still running"), and a negative can only be proven against a moment the
// test knows the switch has actually reached.
const WaitingForTurnLog = waitingForTurnLog

// ComposeContext exposes the {context} composition rule to this package's (external)
// tests. It is not production surface: this file is compiled only under `go test`.
//
// The rule is reached directly rather than through a spawn because one of its inputs
// comes from the process-wide config singleton, and config.Get MEMOISES on first use
// — so a spawn-level test could not present a blanked capabilities_instruction
// without depending on which test in the binary ran first. A composition rule tested
// through a memoised global is a test that passes for the wrong reason.
var ComposeContext = composeContext

// SetPromptJournalDirSync installs a deterministic durability fault for external
// package tests. It is test-only surface; production always uses fsync+close on
// the journal parent directory after the atomic rename.
func SetPromptJournalDirSync(u *Usecase, syncDir func(string) error) {
	u.prompts.syncDir = syncDir
}
