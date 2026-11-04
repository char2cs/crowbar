//go:build integration

package tests

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// The lifecycle frame recorder the agent black-box tests share, and the two
// contracts a chat keeps over its provider's sessions: deleting a chat leaves the
// provider's own session alone, and an agent's rename lands on the chat its runner is on.

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
	postHook(t, h, repoBase(imported), "livestub", runnerID, "session_start", `{"session_id":"`+sessionID+`"}`)
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
	rows, err := h.app.Repositories.ChatLedger.Read(context.Background(), chatID, 0, 0)
	require.NoError(t, err)
	for _, row := range rows {
		if strings.Contains(string(row.Body), text) {
			return
		}
	}
	t.Fatalf("%s: no row of chat %s carries %q (%d rows recorded)", msg, chatID, text, len(rows))
}

func TestRegression_DeleteChat_LeavesProviderSessionIntact(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := repoBase(ws)
	ctx := context.Background()

	frames := recordSidebar(t, h)

	doomed, doomedRunner := createLiveStubChat(t, h, ws)
	other, otherRunner := createLiveStubChat(t, h, ws)

	announce(t, h, ws, doomedRunner, "sess-doomed")
	frames.awaitConversation(doomed, "sess-doomed")
	announce(t, h, ws, otherRunner, "sess-other")
	frames.awaitConversation(other, "sess-other")

	// Give the doomed chat a real conversation — Crowbar's own plaintext copy of what
	// was said, the thing the purge MUST reap (leaving it behind is a privacy bug).
	const privateText = "something private"
	postHook(t, h, repoBase(ws), "livestub", doomedRunner, "user_prompt", `{"prompt":"`+privateText+`"}`)
	frames.awaitTurnStart(doomed)
	// Close the turn first: invariant 9 refuses to delete a WORKING row, and this
	// test is about what survives a delete, not about that refusal.
	postHook(t, h, repoBase(ws), "livestub", doomedRunner, "turn_stop", `{"last_assistant_message":"noted"}`)
	frames.awaitTurnEnd(doomed)
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
	resp := h.raw(http.MethodDelete, base+"/chats/"+doomed, nil, http.StatusOK)
	_ = resp.Body.Close()
	frames.awaitGone(stream.KindChatEntity, doomed)
	h.QuiesceReactors() // its CLI ends with the chat
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
	rows, err := h.app.Repositories.ChatLedger.Read(ctx, doomed, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, rows,
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

	frames := recordSidebar(t, h)

	chatA, runner := createLiveStubChat(t, h, ws)
	announce(t, h, ws, runner, "sess-A")
	frames.awaitConversation(chatA, "sess-A")

	announce(t, h, ws, runner, "sess-cleared")
	frames.awaitConversation(chatA, "sess-cleared")

	const title = "Titled after the clear"
	require.NoError(t, h.app.Usecases.AgentChat.RenameByRunner(
		context.Background(), runner, title, "agent",
	))

	frames.awaitChat(chatA, "titled", func(chat map[string]any) bool { return chat["title"] == title })
	h.Quiesce()

	assert.Equal(t, title, getAgentChat(t, h, base, chatA).Title,
		"the rename lands on the chat the runner is on now")
	var chats []agentChatDTO
	h.get(base+"/chats", &chats)
	assert.Len(t, conversationsOnly(chats), 1, "a /clear mints no chat")
}
