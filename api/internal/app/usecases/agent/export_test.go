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
