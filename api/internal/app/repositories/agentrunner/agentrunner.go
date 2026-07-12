// Package agentrunner is the asynx event-sourced repository for the AgentRunner
// aggregate: ONE running vendor-CLI process in ONE PTY, and the chat +
// conversation it is currently pointed at.
//
// It exists to enforce one rule: persist PLACEMENT, never LIVENESS.
//
//   - LIVENESS belongs to the PTY, and to nothing else. There is no status field
//     and no status column anywhere in this aggregate. "Does a live row exist for
//     this chat" IS the liveness question. Two authorities on liveness always
//     drift, and that drift is the bug this package deletes: today a segment can
//     read "ended" while its CLI is demonstrably still running.
//
//   - PLACEMENT (which chat, which conversation) has exactly ONE writer —
//     Crowbar — so it cannot drift, and it is therefore safe to persist. It is
//     also the thing that must never tear: Move is a single write to a single
//     aggregate. Today's equivalent spans two chat aggregates with no transaction,
//     and in production it tore in half and destroyed a user's chat. One aggregate,
//     one write, and the torn state is not merely avoided — it is unrepresentable.
//
// The EventStore (event_store.go) is the sole repository: mutations dispatch the
// command layer with optimistic-concurrency retry, reads delegate to the
// store-package projections (the live-runner model and the append-only
// conversation history).
package agentrunner

import "errors"

// ErrNotFound is returned when no row backs a requested read: no live runner for
// an id/chat/session, or no conversation for a session/chat. The read-model store
// keeps its own local sentinel (to avoid an import cycle back into this package)
// and event_store.go bridges it to this one via mapNotFound, so every EventStore
// caller sees this single sentinel.
//
// On the LIVE-runner reads this is not an error condition at all: the absence of
// a row IS the answer, and the answer is "dormant" — that is exactly what having
// no status column buys.
var ErrNotFound = errors.New("agentrunner: not found")
