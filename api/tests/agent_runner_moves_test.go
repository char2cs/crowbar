//go:build integration

package tests

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file is the black-box contract for the thing the AgentRunner refactor exists
// to make impossible: a vendor CLI moving between conversations — the user typing
// /clear or /resume INSIDE it — tearing Crowbar's model in half.
//
// The old model kept the running CLI as a SEGMENT ROW INSIDE the chat aggregate, so
// a move was EndSegment(A) + OpenSegment(B): two writes, two aggregates, no
// transaction. In production it tore exactly where you would expect. The first write
// committed, the second failed on chat B's state, and chat A was left with no way
// back into it — permanently unusable, from one keystroke inside a CLI.
//
// A move is now ONE write to ONE aggregate (runner.Move). The runner points at
// the chat; the chat never points back, and the chat being LEFT is not written to at
// all. Every test here is written against the specific OUTCOME the old model
// produced — a bricked source chat, two live CLIs on one chat, a turn landing in a
// chat the CLI has left — not merely against the happy path, so that a regression to
// two-writes-two-aggregates fails them rather than sliding past.
//
// NO TIMING. Every wait blocks on a real signal:
//   - POST /chats/hooks runs IngestHook SYNCHRONOUSLY, so its 202 means the move,
//     the eviction and the SIGTERM have all already happened;
//   - runner commands are asynx SendWait, so a placement is durable AND visible to
//     the read model the instant the command returns;
//   - a process death is nobody's to schedule, so it is awaited as the `exited`
//     lifecycle FRAME the PTY's own death produces (agentFrames.awaitRunner);
//   - harness.Quiesce (asynx WaitPublish) is the read-your-writes barrier before a
//     plain REST read of an async projection.
//
// There is no sleep, no poll, and no timeout used as synchronisation anywhere below.

// agentFrames records every frame the agent lifecycle WebSocket delivers and lets a
// test wait for one BY PREDICATE, regardless of the order the frames arrive in.
//
// That order-independence is the whole reason it exists and is not optional here. A
// single conversation move produces a burst of frames from several aggregates at once
// (the mover's `moved`, the evicted incumbent's `displaced`, then its `exited` once
// its PTY actually dies) and Crowbar makes no promise about their interleaving —
// displacement is a synchronous placement fact while the death that follows it is the
// operating system's business. waitForChatFrame DISCARDS every frame that does not
// match, so waiting for two of those in the wrong order would drop one on the floor
// and then block forever. Recording first and matching after cannot lose a frame.
type agentFrames struct {
	t    *testing.T
	mu   sync.Mutex
	seen []map[string]any
	// ping carries a single wakeup token: buffered(1) and non-blocking to send, so a
	// burst of frames can never deadlock the reader and a wakeup can never be lost —
	// await always re-scans the WHOLE record after waking, so one token covers any
	// number of frames that arrived while it was not looking.
	ping chan struct{}
}

// recordAgentWS dials the agent lifecycle WS at path and starts recording its frames.
func recordAgentWS(
	t *testing.T,
	h *harness,
	path string,
) *agentFrames {
	t.Helper()
	src := dialAgentWS(t, h, path)
	f := &agentFrames{t: t, ping: make(chan struct{}, 1)}
	go func() {
		for frame := range src {
			f.mu.Lock()
			f.seen = append(f.seen, frame)
			f.mu.Unlock()
			select {
			case f.ping <- struct{}{}:
			default:
			}
		}
	}()
	return f
}

// await blocks until some frame satisfying match has been seen — one that arrived
// before this call included. It never sleeps and never polls: it re-scans on a real
// signal (a frame landed) and the test context's Done() is the sole backstop.
func (f *agentFrames) await(
	match func(map[string]any) bool,
	what string,
) map[string]any {
	f.t.Helper()
	for {
		f.mu.Lock()
		for _, frame := range f.seen {
			if match(frame) {
				f.mu.Unlock()
				return frame
			}
		}
		f.mu.Unlock()

		select {
		case <-f.ping:
		case <-f.t.Context().Done():
			f.t.Fatalf("timed out waiting for %s; frames seen: %v", what, f.snapshot())
			return nil
		}
	}
}

// awaitRunner waits for a runner-lifecycle frame (started/session_bound/moved/
// displaced/exited) for runnerID. It keys on the runner id ALONE, never the chat: a
// `displaced` frame deliberately carries an EMPTY chatId — that emptiness IS its
// meaning (Crowbar has taken the runner off its chat and it now holds nothing).
func (f *agentFrames) awaitRunner(
	runnerID string,
	kind string,
) map[string]any {
	f.t.Helper()
	return f.await(
		func(m map[string]any) bool { return m["runnerId"] == runnerID && m["kind"] == kind },
		fmt.Sprintf("runner %s's %q frame", runnerID, kind),
	)
}

// awaitChat waits for a chat-lifecycle frame (created/turn_started/turn_stopped/
// title_set/deleted) for chatID.
func (f *agentFrames) awaitChat(
	chatID string,
	kind string,
) map[string]any {
	f.t.Helper()
	return f.await(
		func(m map[string]any) bool { return m["chatId"] == chatID && m["kind"] == kind },
		fmt.Sprintf("chat %s's %q frame", chatID, kind),
	)
}

// snapshot copies the frames recorded so far, for assertions over the whole stream
// (e.g. "no turn_started frame for chat A was EVER emitted").
func (f *agentFrames) snapshot() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]map[string]any, len(f.seen))
	copy(out, f.seen)
	return out
}

// announce fires a session_start hook: the vendor CLI telling Crowbar which
// conversation it is now in. It is the ONLY input a conversation move ever has —
// Crowbar learns about a /clear or a /resume exactly the way this test does, from a
// changed conversation id under a live runner, with no `source` field and no other
// provider vocabulary involved.
func announce(
	t *testing.T,
	h *harness,
	imported importedRepo,
	runnerID string,
	sessionID string,
) {
	t.Helper()
	postAgentHook(t, h, imported, runnerID, "session_start", `{"session_id":"`+sessionID+`"}`)
}

// chatHandoff reads a chat's assembled ledger (GET .../chats/:id/handoff): the
// DURABLE record of what was actually said in it. It is the on-disk oracle for "did
// this turn land in this chat", answerable long after any in-memory state is gone.
func chatHandoff(
	t *testing.T,
	h *harness,
	base string,
	chatID string,
) string {
	t.Helper()
	var out struct {
		Handoff string `json:"handoff"`
	}
	h.get(base+"/chats/"+chatID+"/handoff", &out)
	return out.Handoff
}

// resumeChat revives a dormant chat (POST .../chats/:id/resume) and returns the
// id of the runner it brought back.
func resumeChat(
	t *testing.T,
	h *harness,
	base string,
	chatID string,
) string {
	t.Helper()
	var revived struct {
		ID string `json:"id"`
	}
	h.post(base+"/chats/"+chatID+"/resume", nil, http.StatusOK, &revived)
	return revived.ID
}

// TestRegression_ResumeIntoOccupiedChat_DoesNotBrickSource is THE reported bug.
//
// A runner is live on chat A. Inside its CLI the user opens the vendor's own /resume
// picker and picks a conversation belonging to chat B — which ALREADY HAS ITS OWN LIVE
// CLI. By the time any hook reaches Crowbar the CLI has already switched: this cannot
// be refused, only reconciled.
//
// The old model reconciled it as two writes across two aggregates:
//
//	EndSegment(A)    // committed
//	OpenSegment(B)   // FAILED — chat B already had an active segment
//	                 // ...and there was no rollback.
//
// Chat A was left with its segment ended and nothing pointed at it — permanently
// unusable, its conversation unreachable — and chat B was never joined at all. One
// keystroke inside a CLI destroyed a chat.
//
// The assertions below are written against that exact outcome, so the old code fails
// every one of them:
//
//   - the mover HOLDS chat B (the old code never joined it);
//   - chat A SURVIVES — dormant, but intact and genuinely REVIVABLE (the old code
//     bricked it);
//   - the incumbent was displaced and its process terminated (invariant I3: two CLIs
//     on one provider conversation both write the same session file and corrupt it —
//     the provider's constraint, not ours, which is why the incumbent is evicted
//     rather than the newcomer refused).
func TestRegression_ResumeIntoOccupiedChat_DoesNotBrickSource(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := repoBase(ws)

	frames := recordAgentWS(t, h, base+"/chats/ws")

	chatA, runnerA := createLiveStubChat(t, h, ws)
	chatB, runnerB := createLiveStubChat(t, h, ws)

	// Each CLI announces its own conversation.
	announce(t, h, ws, runnerA, "sess-A")
	frames.awaitRunner(runnerA, "session_bound")
	announce(t, h, ws, runnerB, "sess-B")
	frames.awaitRunner(runnerB, "session_bound")
	h.Quiesce()

	require.Equal(t, runnerB, getAgentChat(t, h, base, chatB).LiveRunnerID,
		"precondition: chat B is OCCUPIED — it has its own live CLI, on its own conversation")

	// --- the bug: inside chat A's CLI, /resume chat B's conversation ---
	//
	// The hook endpoint runs IngestHook SYNCHRONOUSLY and every runner command is asynx
	// SendWait, so by the time this POST returns 202 the move, the eviction and the
	// SIGTERM have ALL already happened and are visible to the read model. The placement
	// assertions below therefore need no wait at all — which is what makes them fail
	// FAST and legibly when the invariant breaks, instead of hanging on a frame that is
	// never coming.
	announce(t, h, ws, runnerA, "sess-B")
	h.Quiesce()

	// Exactly one CLI is left on chat B, and it is the mover. Asserted with the PLURAL
	// read: the single-row read hands back the newest arrival either way, so it cannot
	// witness a second runner still placed there.
	require.Equal(t, []string{runnerA}, liveRunnersOnChat(t, h, chatB),
		"chat B must be left with EXACTLY ONE live CLI — the one that just moved in")

	// 1. Chat B was JOINED. The old code failed its OpenSegment(B) and left B holding
	//    the incumbent, so the CLI the user is now typing into was pointed at nothing.
	gotB := getAgentChat(t, h, base, chatB)
	assert.Equal(t, runnerA, gotB.LiveRunnerID,
		"the CLI that resumed chat B's conversation must now HOLD chat B — it is the process the user is "+
			"typing into, and a chat whose live runner is anyone else hands the pane the wrong PTY")
	assert.NotEqual(t, runnerB, gotB.LiveRunnerID, "the evicted incumbent must not still be serving chat B")

	// 2. The incumbent was really TERMINATED — not merely un-recorded. Displacement is a
	//    placement fact and says nothing about liveness, so the only honest proof the
	//    process is gone is its PTY's own death, which arrives as the `exited` frame.
	//    (Two CLIs writing one provider session file corrupt it; that is the provider's
	//    constraint, and the reason the incumbent goes rather than the newcomer.)
	frames.awaitRunner(runnerB, "displaced")
	frames.awaitRunner(runnerB, "exited")

	// 3. Chat A SURVIVES the move. This is the brick: the CLI left, but the chat is a
	//    Crowbar object, and the chat being left is not written to AT ALL by a move.
	gotA := getAgentChat(t, h, base, chatA)
	assert.Empty(t, gotA.LiveRunnerID, "chat A is DORMANT — its CLI left. Dormant is a query, not damage")
	assert.Empty(t, gotA.TerminalSessionID, "a dormant chat offers no PTY to attach to")
	assert.Equal(t, []string{"sess-A"}, gotA.sessionIDs(),
		"chat A keeps its conversation history: it is append-only, describes no process, and is the ONLY "+
			"thing that makes the chat revivable — losing it is what made the source chat unreachable")
	assert.Equal(t, "livestub", gotA.ActiveProviderID,
		"a dormant chat still names the provider it last ran, so the UI can render it and Resume knows who to bring back")

	// 4. And it is genuinely resumable — not merely present in the list. This is the
	//    assertion the bricked chat could never satisfy.
	revivedA := resumeChat(t, h, base, chatA)
	require.NotEmpty(t, revivedA, "the source chat must be revivable after another CLI resumed away from it")
	require.NotEqual(t, runnerA, revivedA, "resume must spawn a NEW CLI, not hand back the one that left")
	frames.awaitRunner(revivedA, "started")
	h.Quiesce()

	backA := getAgentChat(t, h, base, chatA)
	assert.Equal(t, revivedA, backA.LiveRunnerID, "the revived chat A must advertise its new runner")
	assert.NotEmpty(t, backA.TerminalSessionID, "and a real PTY to attach to")

	// The mover is untouched by chat A's revival: it is still on chat B.
	assert.Equal(t, runnerA, getAgentChat(t, h, base, chatB).LiveRunnerID,
		"reviving the source chat must not disturb the CLI that moved away from it")
}

// TestRegression_ResumeIntoOccupiedChat_OnADifferentConversation is the deterministic
// variant of the same bug, and it is the one that survives a fix that only enforces
// "at most one runner per CONVERSATION" (invariant I3) and forgets "at most one runner
// per CHAT" (invariant I2). They are different questions, and answering only the first
// leaves the reported bug alive with one variable changed.
//
// The shape:
//
//	Chat B's live CLI is on conversation sC. But chat B ALSO has an OLDER conversation,
//	sB, from a CLI that has since gone. Crowbar remembers sB (history is append-only) —
//	and so does the VENDOR, because Crowbar deliberately never deletes a provider's own
//	session file, which is exactly why sB is still sitting in the CLI's /resume picker.
//
//	Pick sB from inside chat A's CLI. NOBODY holds sB, so a conversation-eviction (I3)
//	finds nothing to evict and does nothing at all — and chat B ends up with TWO live
//	CLIs on it, indefinitely, both appending to its ledger, one of them invisible to
//	every serving read (which returns only the newest arrival).
//
// So the assertion is not "a runner is on chat B" — that is true in the broken code
// too. It is that chat B has EXACTLY ONE live runner, and that the incumbent's process
// is really gone.
func TestRegression_ResumeIntoOccupiedChat_OnADifferentConversation(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := repoBase(ws)

	frames := recordAgentWS(t, h, base+"/chats/ws")

	chatA, runnerA := createLiveStubChat(t, h, ws)
	chatB, runnerB1 := createLiveStubChat(t, h, ws)

	announce(t, h, ws, runnerA, "sess-A")
	frames.awaitRunner(runnerA, "session_bound")

	// Chat B's FIRST conversation. The vendor keeps its session file for this one
	// forever; Crowbar keeps the history row. Both outlive the CLI that opened it.
	announce(t, h, ws, runnerB1, "sess-B")
	frames.awaitRunner(runnerB1, "session_bound")

	// Chat B moves on: a new CLI takes it over (a provider switch — the same path a
	// Resume takes) and opens a DIFFERENT conversation, sC. sB is now nobody's.
	var switched struct {
		ID string `json:"id"`
	}
	h.post(base+"/chats/"+chatB+"/switch", map[string]string{"provider": "livestub"}, http.StatusOK, &switched)
	runnerB2 := switched.ID
	require.NotEmpty(t, runnerB2)
	require.NotEqual(t, runnerB1, runnerB2, "a switch spawns a new CLI")
	frames.awaitRunner(runnerB1, "displaced")
	frames.awaitRunner(runnerB1, "exited")
	frames.awaitRunner(runnerB2, "started")

	announce(t, h, ws, runnerB2, "sess-C")
	frames.awaitRunner(runnerB2, "session_bound")
	h.Quiesce()

	gotB := getAgentChat(t, h, base, chatB)
	require.Equal(t, runnerB2, gotB.LiveRunnerID, "precondition: chat B's live CLI is runnerB2")
	require.Equal(t, []string{"sess-B", "sess-C"}, gotB.sessionIDs(),
		"precondition: chat B has hosted BOTH conversations — the old one is still in its history, and "+
			"still in the vendor's own /resume picker")
	require.Empty(t, liveRunnersOnSession(t, h, ws.workspaceID, "sess-B"),
		"precondition: NOBODY holds sess-B any more — so a conversation-eviction (I3) will find nothing "+
			"to evict, and only a CHAT-eviction (I2) can save chat B")

	// --- the bug: from inside chat A's CLI, /resume chat B's OLD conversation ---
	//
	// IngestHook runs synchronously inside this POST and every runner command is asynx
	// SendWait, so the move and every eviction it triggers are already durable AND
	// visible to the read model when it returns. The invariant is therefore asserted
	// with no wait — a broken I2 fails the very next line rather than hanging on a
	// `displaced` frame that is never coming.
	announce(t, h, ws, runnerA, "sess-B")
	h.Quiesce()

	// THE assertion. A single-row read cannot see this bug — it hands back the newest
	// arrival either way — so ask for EVERYONE placed on the chat.
	live := liveRunnersOnChat(t, h, chatB)
	require.Len(t, live, 1,
		"chat B must have EXACTLY ONE live CLI. Two runners placed on one chat both append to its ledger, "+
			"and the older one is invisible to every serving read — the reported bug with one variable "+
			"changed. Enforcing only 'one runner per CONVERSATION' (I3) does not prevent it: nobody held "+
			"sess-B, so I3 evicted nobody. Only 'one runner per CHAT' (I2) saves chat B. Live runners "+
			"found: %v", live)
	assert.Equal(t, runnerA, live[0], "the newest arrival stays; the rest go")
	assert.Equal(t, runnerA, getAgentChat(t, h, base, chatB).LiveRunnerID)

	// And the evicted incumbent's process is really gone — displacement is a placement
	// fact and asserts nothing about liveness, so the PTY's own death is the only proof.
	frames.awaitRunner(runnerA, "moved")
	frames.awaitRunner(runnerB2, "displaced")
	frames.awaitRunner(runnerB2, "exited")

	// The source chat is intact and dormant, exactly as in the primary variant.
	gotA := getAgentChat(t, h, base, chatA)
	assert.Empty(t, gotA.LiveRunnerID, "chat A is dormant, not bricked")
	assert.Equal(t, []string{"sess-A"}, gotA.sessionIDs())
}

// liveRunnersOnChat lists the ids of EVERY runner currently placed on chatID, newest
// arrival first. The REST surface deliberately exposes only the single newest one
// (liveRunnerId — that is the whole liveness contract a client needs), which is
// precisely why it cannot witness a two-CLIs-on-one-chat break: it would return the
// same id whether the invariant holds or not. So this asks the read model the plural
// question the placement paths themselves ask.
func liveRunnersOnChat(
	t *testing.T,
	h *harness,
	chatID string,
) []string {
	t.Helper()
	runners, err := h.app.Repositories.AgentRunner.LiveRunnersForChat(context.Background(), chatID)
	require.NoError(t, err)
	out := make([]string, 0, len(runners))
	for _, r := range runners {
		out = append(out, r.ID)
	}
	return out
}

// liveRunnersOnSession lists the ids of every runner currently HOLDING a provider
// conversation — the I3 twin of liveRunnersOnChat.
func liveRunnersOnSession(
	t *testing.T,
	h *harness,
	workspaceID string,
	sessionID string,
) []string {
	t.Helper()
	runners, err := h.app.Repositories.AgentRunner.LiveRunnersForSession(context.Background(), workspaceID, sessionID)
	require.NoError(t, err)
	out := make([]string, 0, len(runners))
	for _, r := range runners {
		out = append(out, r.ID)
	}
	return out
}

// TestRegression_HookAfterMove_DoesNotPolluteTheChatItLeft pins the routing rule that
// makes an orphaned CLI harmless: EVERY hook resolves its chat by reading the RUNNER's
// durable placement (runnerID → runner → CurrentChatID), at the moment the hook
// arrives.
//
// Nothing in memory can disagree with that, because there is nothing in memory to
// disagree: the old model answered "which chat is this CLI on?" from an in-process
// registry, which a move updated separately from the durable state. A CLI that had
// moved could still have its turns filed against the chat it LEFT — the user watching
// chat A see a turn it never took, its ledger growing with a conversation happening
// somewhere else, and the next handoff quoting it.
//
// The proof is deliberately DURABLE, not just live state: the turn's text must be in
// the new chat's ledger on disk and absent from the old one's. A ledger cannot be
// explained away by a stale projection.
func TestRegression_HookAfterMove_DoesNotPolluteTheChatItLeft(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := repoBase(ws)
	ctx := context.Background()

	frames := recordAgentWS(t, h, base+"/chats/ws")

	chatA, runner := createLiveStubChat(t, h, ws)
	announce(t, h, ws, runner, "sess-A")
	frames.awaitRunner(runner, "session_bound")

	// /clear: a conversation nobody has ever seen appears under the SAME live process,
	// so Crowbar mints a chat for it and the runner moves in.
	announce(t, h, ws, runner, "sess-cleared")
	chatC, _ := frames.awaitRunner(runner, "moved")["chatId"].(string)
	require.NotEmpty(t, chatC, "a moved frame names the chat the runner moved INTO")
	require.NotEqual(t, chatA, chatC, "/clear must move the CLI to a DIFFERENT chat")

	// The CLI now takes a turn. It belongs to the chat the runner is on NOW.
	const canary = "CANARY-MUST-NOT-LEAK-INTO-THE-CHAT-THE-CLI-LEFT"
	postAgentHook(t, h, ws, runner, "user_prompt", `{"prompt":"`+canary+`"}`)
	frames.awaitChat(chatC, "turn_started")
	h.Quiesce()

	// The durable oracle: the ledgers on disk.
	assert.Contains(t, chatHandoff(t, h, base, chatC), canary,
		"the turn must be recorded in the chat the runner is on NOW")
	assert.NotContains(t, chatHandoff(t, h, base, chatA), canary,
		"the turn must NEVER reach the chat the CLI has LEFT: a hook routed through anything but the "+
			"runner's own durable placement files an orphan's turns against the chat it walked out of")

	// The derived title follows the same routing — it is set from the very same hook.
	assert.Equal(t, canary, getAgentChat(t, h, base, chatC).Title,
		"the first prompt titles the chat the runner is on")
	assert.Empty(t, getAgentChat(t, h, base, chatA).Title,
		"and never the chat it left")

	// And live turn state agrees: chat A must not be left spinning on somebody else's
	// turn. (A workspace's whole working overlay is the OR of its chats', so a chat
	// spinning on a turn it never took spins the workspace icon too.)
	workingC, err := h.app.Usecases.AgentChat.GetChat(ctx, chatC)
	require.NoError(t, err)
	assert.True(t, workingC.Working, "the chat the runner is on is mid-turn")

	workingA, err := h.app.Usecases.AgentChat.GetChat(ctx, chatA)
	require.NoError(t, err)
	assert.False(t, workingA.Working, "the chat the CLI left must not be working: the turn is not its turn")

	// No turn_started frame for chat A was EVER emitted — the live-signal counterpart
	// of the ledger assertion, over the whole recorded stream rather than a point read.
	for _, frame := range frames.snapshot() {
		if frame["kind"] == "turn_started" {
			assert.NotEqual(t, chatA, frame["chatId"],
				"a turn_started frame for the chat the CLI left would light its spinner (and the workspace's)")
		}
	}
}

// TestRegression_ClearMintsChat_KeepsSamePTY pins what makes a conversation switch
// feel instant: /clear moves the RUNNER to a new chat, and the runner IS the process —
// same id, same provider, SAME PTY.
//
// The old model expressed /clear as ending a segment here and opening a segment there,
// which meant the terminal session id the pane was attached to belonged to a row that
// no longer existed. The pane had to find the new segment and remount on it, and the
// user's scrollback went with it. Nothing about the process actually changed: it is
// the same CLI, in the same PTY, that simply started a new conversation.
//
// So this asserts the identity that a two-writes model cannot preserve: the chat the
// runner moved INTO advertises the SAME runner id and the SAME terminalSessionId as
// the chat it left was advertising a moment ago.
func TestRegression_ClearMintsChat_KeepsSamePTY(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := repoBase(ws)

	frames := recordAgentWS(t, h, base+"/chats/ws")

	chatA, runner := createLiveStubChat(t, h, ws)

	before := getAgentChat(t, h, base, chatA)
	pty := before.TerminalSessionID
	require.NotEmpty(t, pty, "precondition: the chat's runner has a PTY the pane can attach to")

	announce(t, h, ws, runner, "sess-A")
	frames.awaitRunner(runner, "session_bound")

	// /clear.
	announce(t, h, ws, runner, "sess-cleared")
	moved := frames.awaitRunner(runner, "moved")
	chatC, _ := moved["chatId"].(string)
	require.NotEmpty(t, chatC)
	require.NotEqual(t, chatA, chatC)
	h.Quiesce()

	gotC := getAgentChat(t, h, base, chatC)
	assert.Equal(t, runner, gotC.LiveRunnerID,
		"/clear moves the SAME live process to the new chat — it does not spawn a CLI, so the runner id "+
			"is stable across the move (which is what lets a hook fired mid-move still name its runner)")
	assert.Equal(t, pty, gotC.TerminalSessionID,
		"and it brings its PTY with it. The terminal session must be IDENTICAL: this is what lets the chat "+
			"pane follow a conversation switch without remounting — a new session id here means the user's "+
			"terminal is torn down and rebuilt under them on every /clear")
	assert.Equal(t, []string{"sess-cleared"}, gotC.sessionIDs(),
		"the new chat hosts exactly the conversation that minted it")
	assert.Equal(t, "livestub", gotC.ActiveProviderID)

	// The chat it left keeps everything except the process.
	gotA := getAgentChat(t, h, base, chatA)
	assert.Empty(t, gotA.LiveRunnerID, "the chat the CLI left is dormant — nothing points at it")
	assert.Empty(t, gotA.TerminalSessionID, "and it must not go on advertising the PTY that walked away: "+
		"a pane attaching to it would land in the CONVERSATION THE USER JUST CLEARED")
	assert.Equal(t, []string{"sess-A"}, gotA.sessionIDs(), "its own conversation history is untouched")
	assert.Equal(t, "livestub", gotA.ActiveProviderID, "so it can still be resumed")
}

// TestRegression_DeleteChat_LeavesProviderSessionIntact pins a standing project rule:
// CROWBAR NEVER DELETES A PROVIDER'S OWN SESSION FILE.
//
// A provider owns its conversations. claude keeps them under its own home, codex keeps
// its rollouts under CODEX_HOME — and Crowbar owns NEITHER, deliberately. It once did:
// pointing CODEX_HOME at a directory Crowbar created and reaped made Crowbar the
// custodian of codex's sessions, so leaving codex DESTROYED codex's own history and
// switching back resumed a thread that no longer existed. Every unit test passed; only
// a real CLI could show it.
//
// So a chat delete reaps CROWBAR's footprint — the chat aggregate, its conversation
// history, its plaintext handoff ledger — and STOPS THERE. What the vendor wrote is
// the vendor's.
//
// The consequence is deliberate and is asserted here too: the deleted chat's
// conversation is still sitting in the CLI's own /resume picker. Crowbar has dropped
// its own history for it, so resuming it must not resurrect the purged chat (a move
// onto a chat that does not exist is an invisible CLI writing nowhere, forever) — it
// must land somewhere real.
// requireChatHoldsText asserts that Crowbar's OWN record of a chat carries text.
//
// It reads the conversation record directly rather than through the chat's REST
// surface, because the record outlives the chat that indexes it: only a direct read
// distinguishes "the conversation was dropped" from "the chat that pointed at it was
// dropped, and the plaintext is still sitting there".
func requireChatHoldsText(
	t *testing.T,
	h *harness,
	chatID string,
	text string,
	msg string,
) {
	t.Helper()
	turns, err := h.app.Repositories.AgentActivity.Turns(context.Background(), chatID, 0, 0, 0)
	require.NoError(t, err)
	for _, turn := range turns {
		if strings.Contains(turn.Text, text) {
			return
		}
	}
	t.Fatalf("%s: no turn of chat %s carries %q (%d turns recorded)", msg, chatID, text, len(turns))
}

func TestRegression_DeleteChat_LeavesProviderSessionIntact(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := repoBase(ws)
	ctx := context.Background()

	frames := recordAgentWS(t, h, base+"/chats/ws")

	doomed, doomedRunner := createLiveStubChat(t, h, ws)
	other, otherRunner := createLiveStubChat(t, h, ws)

	announce(t, h, ws, doomedRunner, "sess-doomed")
	frames.awaitRunner(doomedRunner, "session_bound")
	announce(t, h, ws, otherRunner, "sess-other")
	frames.awaitRunner(otherRunner, "session_bound")

	// Give the doomed chat a real conversation — Crowbar's own plaintext copy of what
	// was said, the thing the purge MUST reap (leaving it behind is a privacy bug).
	const privateText = "something private"
	postAgentHook(t, h, ws, doomedRunner, "user_prompt", `{"prompt":"`+privateText+`"}`)
	frames.awaitChat(doomed, "turn_started")
	// Close the turn before the delete below: invariant 9 (2026-08-28 addendum §2)
	// refuses to delete a WORKING row unconditionally, and this test is about what
	// survives a delete, not about that refusal (pinned separately).
	postAgentHook(t, h, ws, doomedRunner, "turn_stop", `{"last_assistant_message":"noted"}`)
	frames.awaitChat(doomed, "turn_stopped")
	h.Quiesce()

	// The PROVIDER's own session store, where a vendor CLI actually keeps its
	// conversations: OUTSIDE crowbar home entirely. Crowbar's only filesystem removals
	// are routed through a guard that refuses any target not strictly under crowbar
	// home (fail-closed), and that guard is what makes this file structurally safe —
	// but the rule is the point, so it is asserted, not assumed.
	vendorStore := t.TempDir()
	vendorSession := filepath.Join(vendorStore, "sessions", "sess-doomed.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(vendorSession), 0o750))
	const vendorContent = `{"session":"sess-doomed","turns":["the user's actual conversation"]}`
	require.NoError(t, os.WriteFile(vendorSession, []byte(vendorContent), 0o600))
	require.False(t, strings.HasPrefix(vendorSession, h.home+string(os.PathSeparator)),
		"precondition: a provider's session store lies outside crowbar home — Crowbar does not own it")

	// Crowbar's own copy of that conversation, read from the record itself rather
	// than through the chat — after the delete the chat is gone, so a read routed
	// through it could only ever answer "not found" and would prove nothing about
	// whether the text was actually dropped.
	//
	// The record IS the footprint now: the per-chat plaintext ledger directory this
	// once asserted on is gone, and the conversation lives in the activity record
	// PurgeChat forgets (agent.ChatUsecase.PurgeChat — "the conversation itself no longer
	// lives here, it is in the record dropped above").
	requireChatHoldsText(t, h, doomed, privateText,
		"precondition: Crowbar keeps its own plaintext copy of what was said")

	// --- hard delete ---
	resp := h.raw(http.MethodDelete, base+"/chats/"+doomed, nil, http.StatusAccepted)
	_ = resp.Body.Close()
	frames.awaitChat(doomed, "deleted")
	frames.awaitRunner(doomedRunner, "displaced") // its CLI is taken off the chat and killed
	h.Quiesce()

	// THE RULE: the vendor's session file is untouched, byte for byte.
	assert.FileExists(t, vendorSession,
		"Crowbar must NEVER delete a provider's own session file. Deleting a CROWBAR chat is not a "+
			"licence to destroy the VENDOR's conversation — that is the user's data, held by a tool "+
			"Crowbar does not own")
	got, err := os.ReadFile(vendorSession)
	require.NoError(t, err)
	assert.Equal(t, vendorContent, string(got), "and it must not have been rewritten or truncated either")

	// Crowbar's OWN footprint IS reaped — so the assertion above is a real confinement
	// claim (the purge ran, removed what is Crowbar's, and stopped), not a vacuous one.
	turns, err := h.app.Repositories.AgentActivity.Turns(ctx, doomed, 0, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, turns,
		"the chat's conversation record must be reaped with the chat: a hard delete that leaves what was "+
			"said readable under .crowbar has not deleted anything the user cares about")
	gone := h.raw(http.MethodGet, base+"/chats/"+doomed, nil, http.StatusNotFound)
	_ = gone.Body.Close()

	// And now the consequence of leaving the vendor's file alone: that conversation is
	// STILL in the CLI's /resume picker. Pick it from another chat's CLI. Crowbar
	// dropped its own history for it, so it is no longer a known conversation — it must
	// mint a fresh chat, never repoint the runner at the chat that no longer exists.
	announce(t, h, ws, otherRunner, "sess-doomed")
	landed, _ := frames.awaitRunner(otherRunner, "moved")["chatId"].(string)
	require.NotEmpty(t, landed, "the CLI must land on a real chat")
	assert.NotEqual(t, doomed, landed,
		"a purged chat must NEVER be resurrected as a move target: a runner pointed at a chat that does "+
			"not exist is an invisible CLI, writing nowhere")
	assert.NotEqual(t, other, landed, "an unknown conversation mints a NEW chat")
	h.Quiesce()

	stillGone := h.raw(http.MethodGet, base+"/chats/"+doomed, nil, http.StatusNotFound)
	_ = stillGone.Body.Close()
	assert.Equal(t, otherRunner, getAgentChat(t, h, base, landed).LiveRunnerID,
		"the CLI is placed on the chat it landed on, and is reachable there")
}

// TestRegression_RenameResolvesChatAtCallTime pins the fix for an agent titling the
// WRONG chat.
//
// An agent names its own conversation. The obvious way to wire that is to hand it the
// chat id at spawn time — and it is wrong, because the CLI can MOVE. A /clear puts the
// same live process on a different chat; the id it was given is now the id of a chat it
// has walked out of. The agent then dutifully titles the chat it LEFT, overwriting a
// title that belongs to a conversation it is no longer having, while the chat it IS
// having stays untitled.
//
// So titling is keyed by the RUNNER — the one id that is stable for the whole life of
// the process — and the chat is resolved from the runner's durable placement AT CALL
// TIME. There is no chat id to go stale, because the agent is never told one.
//
// It is driven here through RenameByRunner rather than over HTTP because that is now
// the whole path: the runner-keyed rename ROUTE is gone with the shell command that was
// its only caller, and the agent's set_chat_title tool calls this method directly after
// the MCP resolver has turned its per-boot token into a runner. Reaching the tool over
// /chats/runners/:segid/mcp instead would need that token, which is minted inside the
// daemon and deliberately not published — so the transport is skipped and the property
// the regression is about (runner → CURRENT chat, resolved late) is exercised whole.
func TestRegression_RenameResolvesChatAtCallTime(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := repoBase(ws)

	frames := recordAgentWS(t, h, base+"/chats/ws")

	chatA, runner := createLiveStubChat(t, h, ws)
	announce(t, h, ws, runner, "sess-A")
	frames.awaitRunner(runner, "session_bound")

	// /clear — the same process, a new chat. Everything the agent was told at spawn
	// time about WHICH chat it is on is now stale.
	announce(t, h, ws, runner, "sess-cleared")
	chatC, _ := frames.awaitRunner(runner, "moved")["chatId"].(string)
	require.NotEmpty(t, chatC)
	require.NotEqual(t, chatA, chatC)

	// The agent titles itself. This is exactly what set_chat_title passes: the
	// RUNNER's id, and a title. No chat id — it does not have one.
	const title = "Titled after the clear"
	require.NoError(t, h.app.Usecases.AgentChat.RenameByRunner(
		context.Background(), runner, title, "agent",
	))

	frames.awaitChat(chatC, "title_set")
	h.Quiesce()

	assert.Equal(t, title, getAgentChat(t, h, base, chatC).Title,
		"the rename must land on the chat the runner is on NOW — resolved from the runner at call time")
	assert.Empty(t, getAgentChat(t, h, base, chatA).Title,
		"and NEVER on the chat it left: a chat id baked into the agent's spawn-time instruction titles the "+
			"conversation the user walked away from, and leaves the real one nameless")

	// No title_set frame was ever emitted for the chat it left.
	for _, frame := range frames.snapshot() {
		if frame["kind"] == "title_set" {
			assert.NotEqual(t, chatA, frame["chatId"], "the chat the CLI left must never be renamed by it")
		}
	}
}
