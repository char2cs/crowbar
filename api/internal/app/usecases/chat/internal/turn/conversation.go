// Package turn (file conversation.go) answers one question the hook
// ingress asks of every event: is this the conversation the runner is on, or
// another one the provider pushed down the same connection?
package turn

import (
	"context"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// namesAnotherConversation reports whether ev describes a conversation other
// than the one this runner is on — in which case it is not this chat's to
// record, whatever wire carried it here.
//
// A CONNECTION IS NOT A CONVERSATION. Confirmed live (codex-cli 0.149.1): codex
// pushes a child thread's COMPLETE, independent turn/started..item/*..
// turn/completed cycle down the SAME websocket the runner's own thread uses,
// having never been asked to open it — for a collab agent, and for the review,
// compaction and memory-consolidation threads it spawns on its own. Measured on
// a security review that delegated to a sub-agent: the child's turn/completed
// landed 83 SECONDS before the user's turn actually ended, and closeTurnFromStop
// filed it against the user's chat — StopTurn, Working=false, spinner dark,
// while codex was still writing the answer. The child's assistant messages were
// recorded into the user's transcript on the way past, and its
// thread/status/changed(idle) armed the 5s provider-idle fuse under that same
// live turn.
//
// inbound.Parse skips its own ownership guard for api-transport events, on the
// reasoning that the socket "IS the scoping". It is not. This is the check that
// actually scopes them, and it needs no provider vocabulary to do it: the
// descriptor already maps session_id for exactly these events, so all that was
// missing was the identity comparison.
//
// THE API CHANNEL IS ANSWERED BY CAUSE, NOT BY IDENTITY. Crowbar's own driver
// mints every conversation this chat legitimately runs on — the first one
// included (apidriver.EstablishSession claims it) — so a conversation it never
// opened is one the provider opened for itself. Comparing ids instead was the
// bug: a pure api-transport spawn leaves CurrentSession AND LaunchSessionID
// empty (47 of 49 real codex turn rows), the comparison had nothing to run
// against, and every child thread was waved through. Learning the parent's id
// off the wire instead ("first conversation named wins") was REVERTED the same
// day: it latched onto a CHILD and then dropped the PARENT's frames wholesale
// — a codex chat running three subagents recorded zero subagents and zero wire
// tool calls. Against the originated set the parent can never be foreign,
// because our own driver is what opened it, so that failure mode cannot recur.
//
// THE HOOKS BRANCH MUST NOT BE UNIFIED WITH IT. The hooks relay carries a
// different id namespace entirely: the companion PTY every api-transport spawn
// forks fires the descriptor's whole hook set under the CLI's OWN session id,
// which our driver never originated, and a provider's own internal sessions
// fire the same hooks under an id of their own (the recorded Stop.json is a
// codex memory-consolidation session's). Judged against the originated set,
// every real hook event of a codex chat would be dropped.
//
// The api branch also asks whether the connection is STILL THERE, because the
// originated record belongs to it: a runner with no connection answers
// "originated nothing" to every id, which is the absence of an authority rather
// than a verdict, and acting on it would drop whatever a torn-down connection
// still had in flight. The row comparison stands in for that window.
//
// session_start is exempt: it is the one event that legitimately announces a
// session this runner does not have yet, and move.Decide already arbitrates
// whether that binds, moves or is ignored.
func (t *Turns) namesAnotherConversation(
	ctx context.Context,
	runner engineagents.Runner,
	ev engineagents.CanonicalEvent,
) bool {
	if ev.Kind == engineagents.HookSessionStart {
		return false
	}
	if ev.SessionID == "" {
		return false
	}
	if channelFor(ctx) == engineagents.ChannelAPI && t.hasAPIConnection(runner.ID) {
		return !t.originatedSession(runner.ID, ev.SessionID)
	}
	mine := runnerConversation(runner)
	if mine == "" {
		return false
	}
	return ev.SessionID != mine
}

// runnerConversation is which conversation this runner is on, for the hooks
// channel's identity comparison — the same CurrentSession-then-LaunchSessionID
// fallback resumeTarget (promptdelivery.go) already resolves a resume against.
// A row naming neither drops nothing.
func runnerConversation(runner engineagents.Runner) string {
	if runner.CurrentSession != "" {
		return runner.CurrentSession
	}
	return runner.LaunchSessionID
}

// originatedSession reports whether THIS runner's own api connection produced
// sessionID, or is producing one right now — the causal authority behind the
// api branch above. A claim still open biases it to true
// (originatedSessions.has, runner package), which is the conservative
// direction: while Crowbar is mid-mint nothing is dropped.
//
// Nil-safe, the same reason apiConnRegistry is: tests build a bare &Turns{} to
// exercise one guard and never reach New. FALSE is the nil answer — true there
// would make such a test silently drop nothing and assert nothing.
func (t *Turns) originatedSession(runnerID, sessionID string) bool {
	if t.runners == nil {
		return false
	}
	return t.runners.OriginatedSession(runnerID, sessionID)
}

// hasAPIConnection reports whether the connection that holds runnerID's
// originated record still exists. Nil-safe the same way, and FALSE there for
// the same reason: a bare &Turns{} has no port to ask, so the guard falls back
// to the row rather than judging against a record nothing is keeping.
func (t *Turns) hasAPIConnection(runnerID string) bool {
	if t.runners == nil {
		return false
	}
	return t.runners.HasLiveAPIConnection(runnerID)
}
