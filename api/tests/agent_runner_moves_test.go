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

// The lifecycle frame recorder the agent black-box tests share, and the two
// contracts a chat keeps over its provider's sessions: deleting a chat leaves the
// provider's own session alone, and an agent's rename lands on the chat its runner is on.

// agentFrames records every frame the agent lifecycle WebSocket delivers and lets a
// test wait for one BY PREDICATE, regardless of the order the frames arrive in.
//
// A teardown produces frames from several aggregates at once (a `displaced`, then an
// `exited` once the PTY dies) in no promised order. waitForChatFrame DISCARDS every
// frame that does not match; recording first and matching after cannot lose one.
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

// awaitRunner waits for a runner-lifecycle frame (started/session_bound/
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

// awaitConversation waits for a frame whose chat snapshot shows chatID bound to sessionID.
func (f *agentFrames) awaitConversation(chatID, sessionID string) map[string]any {
	f.t.Helper()
	return f.await(
		func(m map[string]any) bool { return boundTo(m, chatID, sessionID) },
		fmt.Sprintf("chat %s bound to session %s", chatID, sessionID),
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
// conversation it is now in, as a /clear or a /resume inside it does.
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
// The consequence is asserted too: the deleted chat's conversation is still in the
// CLI's own /resume picker, and resuming it binds the chat the CLI is on — it never
// resurrects the purged chat.
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
	frames.awaitConversation(doomed, "sess-doomed")
	announce(t, h, ws, otherRunner, "sess-other")
	frames.awaitConversation(other, "sess-other")

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

	// That conversation is still in the CLI's /resume picker. Picked from another
	// chat's CLI, it is no chat's any more, so it binds the chat that CLI is on.
	announce(t, h, ws, otherRunner, "sess-doomed")
	frames.awaitConversation(other, "sess-doomed")
	h.Quiesce()

	stillGone := h.raw(http.MethodGet, base+"/chats/"+doomed, nil, http.StatusNotFound)
	_ = stillGone.Body.Close()
	assert.Equal(t, otherRunner, getAgentChat(t, h, base, other).LiveRunnerID,
		"the CLI stays on its own chat: a purged chat is never resurrected")
}

// TestRegression_RenameResolvesChatAtCallTime: an agent titles its chat by its
// RUNNER id, resolved to the runner's placement at call time — so a /clear inside
// the CLI, which keeps it on its chat, titles that chat and mints no other.
func TestRegression_RenameResolvesChatAtCallTime(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := repoBase(ws)

	frames := recordAgentWS(t, h, base+"/chats/ws")

	chatA, runner := createLiveStubChat(t, h, ws)
	announce(t, h, ws, runner, "sess-A")
	frames.awaitConversation(chatA, "sess-A")

	announce(t, h, ws, runner, "sess-cleared")
	frames.awaitConversation(chatA, "sess-cleared")

	const title = "Titled after the clear"
	require.NoError(t, h.app.Usecases.AgentChat.RenameByRunner(
		context.Background(), runner, title, "agent",
	))

	frames.awaitChat(chatA, "title_set")
	h.Quiesce()

	assert.Equal(t, title, getAgentChat(t, h, base, chatA).Title,
		"the rename lands on the chat the runner is on now")
	var chats []agentChatDTO
	h.get(base+"/chats", &chats)
	assert.Len(t, conversationsOnly(chats), 1, "a /clear mints no chat")
}
