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

	const privateText = "something private"
	postHook(t, h, repoBase(ws), "livestub", doomedRunner, "user_prompt", `{"prompt":"`+privateText+`"}`)
	frames.awaitTurnStart(doomed)
	postHook(t, h, repoBase(ws), "livestub", doomedRunner, "turn_stop", `{"last_assistant_message":"noted"}`)
	frames.awaitTurnEnd(doomed)
	h.Quiesce()

	vendorStore := t.TempDir()
	vendorSession := filepath.Join(vendorStore, "sessions", "sess-doomed.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(vendorSession), 0o750))
	const vendorContent = `{"session":"sess-doomed","turns":["the user's actual conversation"]}`
	require.NoError(t, os.WriteFile(vendorSession, []byte(vendorContent), 0o600))
	require.False(t, strings.HasPrefix(vendorSession, h.home+string(os.PathSeparator)),
		"precondition: a provider's session store lies outside crowbar home — Crowbar does not own it")

	requireChatHoldsText(t, h, doomed, privateText,
		"precondition: Crowbar keeps its own plaintext copy of what was said")

	resp := h.raw(http.MethodDelete, base+"/chats/"+doomed, nil, http.StatusOK)
	_ = resp.Body.Close()
	frames.awaitGone(stream.KindChatEntity, doomed)
	h.QuiesceReactors()
	h.Quiesce()

	assert.FileExists(t, vendorSession,
		"Crowbar must NEVER delete a provider's own session file. Deleting a CROWBAR chat is not a "+
			"licence to destroy the VENDOR's conversation — that is the user's data, held by a tool "+
			"Crowbar does not own")
	got, err := os.ReadFile(vendorSession)
	require.NoError(t, err)
	assert.Equal(t, vendorContent, string(got), "and it must not have been rewritten or truncated either")

	rows, err := h.app.Repositories.ChatLedger.Read(ctx, doomed, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, rows,
		"the chat's conversation record must be reaped with the chat: a hard delete that leaves what was "+
			"said readable under .crowbar has not deleted anything the user cares about")
	gone := h.raw(http.MethodGet, base+"/chats/"+doomed, nil, http.StatusNotFound)
	_ = gone.Body.Close()

	announce(t, h, ws, otherRunner, "sess-doomed")
	frames.awaitConversation(other, "sess-doomed")
	h.Quiesce()

	stillGone := h.raw(http.MethodGet, base+"/chats/"+doomed, nil, http.StatusNotFound)
	_ = stillGone.Body.Close()
	assert.Equal(t, otherRunner, getAgentChat(t, h, base, other).LiveRunnerID,
		"the CLI stays on its own chat: a purged chat is never resurrected")
}

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

func TestRegression_ResumeInAChatThatNeverTurned_MovesTheConversationThere(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := repoBase(ws)
	frames := recordSidebar(t, h)

	held, heldRunner := createLiveStubChat(t, h, ws)
	announce(t, h, ws, heldRunner, "sess-held")
	frames.awaitConversation(held, "sess-held")
	postHook(t, h, base, "livestub", heldRunner, "user_prompt", `{"prompt":"the conversation"}`)
	frames.awaitTurnStart(held)
	postHook(t, h, base, "livestub", heldRunner, "turn_stop", `{"last_assistant_message":"held here"}`)
	frames.awaitTurnEnd(held)

	taker, takerRunner := createLiveStubChat(t, h, ws)
	announce(t, h, ws, takerRunner, "sess-stand-in")
	frames.awaitConversation(taker, "sess-stand-in")

	announce(t, h, ws, takerRunner, "sess-held")
	frames.awaitConversation(taker, "sess-held")
	frames.awaitChat(held, "lost its session", func(chat map[string]any) bool {
		conversation, _ := chat["conversation"].(map[string]any)
		return conversation["sessionId"] == "sess-held" && conversation["state"] == "lost"
	})
	frames.awaitRunnerGone(held, heldRunner)

	const said = "said where it was resumed"
	postHook(t, h, base, "livestub", takerRunner, "user_prompt", `{"prompt":"go on"}`)
	frames.awaitTurnStart(taker)
	postHook(t, h, base, "livestub", takerRunner, "turn_stop", `{"last_assistant_message":"`+said+`"}`)
	frames.awaitTurnEnd(taker)
	h.Quiesce()

	requireChatHoldsText(t, h, taker, said, "the resumed CLI is recorded on the chat it was resumed in")
	assert.Equal(t, takerRunner, getAgentChat(t, h, base, taker).LiveRunnerID, "and keeps running there")
}
