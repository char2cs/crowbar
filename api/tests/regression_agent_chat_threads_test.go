//go:build integration

package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/config"
)

const threadStubProviderDescriptorYAML = `id: threadstub
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  user_prompt:
    in: user_prompt
    map:
      message: prompt
  turn_stop:
    in: turn_stop
    map:
      session_id: session_id
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
mcp_injection:
  - write_file: { path: "{tmp}/runner-token", content: "{runner_token}" }
context_inject:
  - write_file: { path: "{tmp}/context.txt", content: "{context}" }
resume_context_inject:
  - write_file: { path: "{tmp}/context.txt", content: "{context}" }
`

func threadContextBlock(
	ids ...string,
) string {
	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		lines = append(lines, "- "+id)
	}
	return strings.ReplaceAll(config.GetPrompts().ThreadLineage, "{lineage}", strings.Join(lines, "\n"))
}

func writeThreadStubProviderDescriptor(
	t *testing.T,
	h *harness,
) {
	t.Helper()
	writeProviderDescriptor(t, h, "threadstub", threadStubProviderDescriptorYAML)
}

func newThreadChat(
	t *testing.T,
	h *harness,
	imported importedRepo,
) (chatID, runnerID string) {
	t.Helper()
	return createStubChat(t, h, imported, "threadstub")
}

func newThreadChatUnder(
	t *testing.T,
	h *harness,
	imported importedRepo,
	parentID string,
) (chatID, runnerID string) {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats",
		map[string]string{"provider": "threadstub", "parentId": parentID, "workspaceId": imported.workspaceID},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.QuiesceReactors()
	return created.ID, startChat(t, h, repoBase(imported), created.ID)
}

func runnerFile(
	t *testing.T,
	h *harness,
	imported importedRepo,
	runnerID string,
	name string,
) string {
	t.Helper()
	chatsDir, err := h.app.Usecases.AgentWorkspaceReader.AgentChatsDir(
		context.Background(), imported.workspaceID,
	)
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(chatsDir, "runners", runnerID+"-threadstub", name))
	require.NoError(t, err, "runner %s wrote no %s", runnerID, name)
	return string(data)
}

func say(
	t *testing.T,
	h *harness,
	imported importedRepo,
	runnerID string,
	prompt string,
	answer string,
) {
	t.Helper()
	postHook(t, h, repoBase(imported), "threadstub", runnerID, "user_prompt", `{"prompt":`+mustQuote(t, prompt)+`}`)
	postHook(t, h, repoBase(imported), "threadstub", runnerID, "turn_stop", `{"last_assistant_message":`+mustQuote(t, answer)+`}`)
	h.Quiesce()
}

func mustQuote(
	t *testing.T,
	s string,
) string {
	t.Helper()
	out, err := json.Marshal(s)
	require.NoError(t, err)
	return string(out)
}

func readChatLog(
	t *testing.T,
	h *harness,
	imported importedRepo,
	runnerID string,
	targetChatID string,
) string {
	t.Helper()
	rpc := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_chat_log",` +
		`"arguments":{"chatId":` + mustQuote(t, targetChatID) + `}}}`
	var out struct {
		RPC json.RawMessage `json:"rpc"`
	}
	h.post(repoBase(imported)+"/chats/runners/"+runnerID+"/mcp", map[string]any{
		"token": runnerFile(t, h, imported, runnerID, "runner-token"),
		"rpc":   json.RawMessage(rpc),
	}, http.StatusOK, &out)
	return string(out.RPC)
}

func TestRegression_AThreadReadsTheChatItHangsOff(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	parentID, parentRunner := newThreadChat(t, h, imported)
	say(t, h, imported, parentRunner, "work out the plan", "the plan is to rewrite the parser")

	threadID, threadRunner := newThreadChat(t, h, imported)
	placeChat(t, h, repoBase(imported), threadID, map[string]any{"parentId": parentID})
	h.Quiesce()

	got := readChatLog(t, h, imported, threadRunner, parentID)
	assert.Contains(t, got, "the plan is to rewrite the parser",
		"a thread must be able to read the chat it continues — that edge is the whole point of it")
}

func TestRegression_AChatCannotReadItsOwnThread(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	parentID, parentRunner := newThreadChat(t, h, imported)
	threadID, threadRunner := newThreadChat(t, h, imported)
	say(t, h, imported, threadRunner, "try the parser", "the thread's own findings")
	placeChat(t, h, repoBase(imported), threadID, map[string]any{"parentId": parentID})
	h.Quiesce()

	got := readChatLog(t, h, imported, parentRunner, threadID)
	assert.Contains(t, got, "never below", "the refusal must say which direction is closed")
	assert.NotContains(t, got, "the thread's own findings",
		"a parent reading its own thread closes the loop the tree exists to keep open")
}

func TestRegression_AChatCannotReadAThreadItHasFiledInFolders(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	parentID, parentRunner := newThreadChat(t, h, imported)
	outer := createChatFolder(t, h, base, "attempts", parentID)
	inner := createChatFolder(t, h, base, "second pass", outer.ID)

	threadID, threadRunner := newThreadChat(t, h, imported)
	say(t, h, imported, threadRunner, "try again", "the filed thread's findings")
	placeChat(t, h, base, threadID, map[string]any{"parentId": inner.ID})
	h.Quiesce()

	got := readChatLog(t, h, imported, parentRunner, threadID)
	assert.Contains(t, got, "never below")
	assert.NotContains(t, got, "the filed thread's findings")
}

func TestRegression_SiblingThreadsStillReadEachOther(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	parentID, _ := newThreadChat(t, h, imported)
	firstID, firstRunner := newThreadChat(t, h, imported)
	secondID, secondRunner := newThreadChat(t, h, imported)
	say(t, h, imported, firstRunner, "attempt one", "the first attempt used a lexer")
	placeChat(t, h, base, firstID, map[string]any{"parentId": parentID})
	placeChat(t, h, base, secondID, map[string]any{"parentId": parentID})
	h.Quiesce()

	got := readChatLog(t, h, imported, secondRunner, firstID)
	assert.Contains(t, got, "the first attempt used a lexer",
		"siblings comparing notes is what this tool was built for and must keep working")
}

func TestRegression_AThreadIsSpawnedPointedAtItsLineage(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	parentID, parentRunner := newThreadChat(t, h, imported)
	say(t, h, imported, parentRunner, "work out the plan", "the parent's private reasoning")

	threadID, _ := newThreadChat(t, h, imported)
	placeChat(t, h, base, threadID, map[string]any{"parentId": parentID})
	h.Quiesce()

	var switched struct {
		ID string `json:"id"`
	}
	h.post(base+"/chats/"+threadID+"/switch",
		map[string]string{"provider": "threadstub"}, http.StatusOK, &switched)
	h.Quiesce()

	injected := runnerFile(t, h, imported, getAgentChat(t, h, base, threadID).LiveRunnerID, "context.txt")
	assert.Contains(t, injected, threadContextBlock(parentID),
		"the thread must be spawned already knowing which chat it hangs off and how to read it")
	assert.NotContains(t, injected, "the parent's private reasoning",
		"a pointer, not a paste: a pasted parent is frozen at this instant and can never be refreshed")
}

func TestRegression_AFiledChatIsSpawnedWithNoThreadContext(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	folder := createChatFolder(t, h, base, "spikes", "")
	plainID, _ := newThreadChat(t, h, imported)
	filedID, _ := newThreadChat(t, h, imported)
	placeChat(t, h, base, filedID, map[string]any{"parentId": folder.ID})
	h.Quiesce()

	for _, chatID := range []string{plainID, filedID} {
		var switched struct {
			ID string `json:"id"`
		}
		h.post(base+"/chats/"+chatID+"/switch",
			map[string]string{"provider": "threadstub"}, http.StatusOK, &switched)
		h.Quiesce()
	}

	plain := runnerFile(t, h, imported, getAgentChat(t, h, base, plainID).LiveRunnerID, "context.txt")
	filed := runnerFile(t, h, imported, getAgentChat(t, h, base, filedID).LiveRunnerID, "context.txt")
	assert.Equal(t, plain, filed,
		"filing a chat in a folder is organisation; it must not add one byte to what the agent is told")
	assert.NotContains(t, filed, "THREAD CONTEXT")
}

func TestRegression_FoldersAreTransparentToWhatAThreadIsTold(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	parentID, _ := newThreadChat(t, h, imported)
	outer := createChatFolder(t, h, base, "attempts", parentID)
	inner := createChatFolder(t, h, base, "second pass", outer.ID)

	directID, _ := newThreadChat(t, h, imported)
	filedID, _ := newThreadChat(t, h, imported)
	placeChat(t, h, base, directID, map[string]any{"parentId": parentID})
	placeChat(t, h, base, filedID, map[string]any{"parentId": inner.ID})
	h.Quiesce()

	docs := map[string]string{}
	for _, chatID := range []string{directID, filedID} {
		var switched struct {
			ID string `json:"id"`
		}
		h.post(base+"/chats/"+chatID+"/switch",
			map[string]string{"provider": "threadstub"}, http.StatusOK, &switched)
		h.Quiesce()
		docs[chatID] = runnerFile(t, h, imported,
			getAgentChat(t, h, base, chatID).LiveRunnerID, "context.txt")
	}

	assert.Contains(t, docs[directID], threadContextBlock(parentID),
		"the directly threaded chat is told what it reads")
	assert.Contains(t, docs[filedID], threadContextBlock(parentID),
		"and so is the one buried two folders down")
	assert.Equal(t, docs[directID], docs[filedID],
		"a thread two folders deep reads exactly what one sitting directly under the chat reads")
}

func TestRegression_ReParentingIsRecordedInTheThreadsOwnLedger(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	parentID, _ := newThreadChat(t, h, imported)
	threadID, threadRunner := newThreadChat(t, h, imported)
	say(t, h, imported, threadRunner, "a question", "an answer given with none of that context")

	placeChat(t, h, base, threadID, map[string]any{"parentId": parentID})
	h.Quiesce()

	var assembled struct {
		Handoff string `json:"handoff"`
	}
	h.get(base+"/chats/"+threadID+"/handoff", &assembled)
	handoff := assembled.Handoff

	assert.Contains(t, handoff, "an answer given with none of that context")
	assert.Contains(t, handoff, parentID, "the record must name what the chat now reads")
	assert.Contains(t, handoff, "from this point on",
		"and must date the change, or it reads as though the chat always had this context")
	assert.Less(t,
		strings.Index(handoff, "an answer given with none of that context"),
		strings.Index(handoff, "from this point on"),
		"the note lands AFTER the turns that were had without the context, never before them")
}

func TestRegression_AChatCreatedUnderAChatIsThreadedOnItsFirstSpawn(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	parentID, parentRunner := newThreadChat(t, h, imported)
	say(t, h, imported, parentRunner, "work out the plan", "the plan is to rewrite the parser")

	threadID, threadRunner := newThreadChatUnder(t, h, imported, parentID)

	assert.Equal(t, parentID, getAgentChat(t, h, base, threadID).ParentID,
		"the chat is born threaded, not threaded a moment later")
	assert.Contains(t, runnerFile(t, h, imported, threadRunner, "context.txt"),
		threadContextBlock(parentID),
		"the FIRST CLI on a new thread must already know which chat it continues")

	assert.Contains(t, readChatLog(t, h, imported, threadRunner, parentID),
		"the plan is to rewrite the parser")
}

func TestRegression_AChatCreatedInAFolderIsPlacedButNotThreaded(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	folder := createChatFolder(t, h, base, "spikes", "")
	chatID, runnerID := newThreadChatUnder(t, h, imported, folder.ID)

	assert.Equal(t, folder.ID, getAgentChat(t, h, base, chatID).ParentID)
	assert.NotContains(t, runnerFile(t, h, imported, runnerID, "context.txt"), "THREAD CONTEXT",
		"a chat filed in a folder inherits nothing and must be spawned like any other")
}

func TestRegression_AChatCreatedInAFolderInsideAChatIsStillItsThread(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	parentID, _ := newThreadChat(t, h, imported)
	outer := createChatFolder(t, h, base, "attempts", parentID)
	inner := createChatFolder(t, h, base, "second pass", outer.ID)

	_, runnerID := newThreadChatUnder(t, h, imported, inner.ID)

	assert.Contains(t, runnerFile(t, h, imported, runnerID, "context.txt"),
		threadContextBlock(parentID),
		"folders are transparent to a create for the same reason they are to a drag")
}

func TestRegression_AChatCreatedWithNoParentIsUnchanged(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	plainID, plainRunner := newThreadChat(t, h, imported)
	rootedID, rootedRunner := newThreadChatUnder(t, h, imported, "")

	assert.Empty(t, getAgentChat(t, h, base, plainID).ParentID)
	assert.Empty(t, getAgentChat(t, h, base, rootedID).ParentID)
	assert.Equal(t,
		runnerFile(t, h, imported, plainRunner, "context.txt"),
		runnerFile(t, h, imported, rootedRunner, "context.txt"),
		"an omitted parentId and an empty one are the same request, and it is today's request")
}

func TestRegression_CreatingAChatUnderAnUnknownParentLeavesNothingBehind(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	before := chatIDs(t, h, base)
	resp := h.raw(http.MethodPost, base+"/chats",
		map[string]string{"provider": "threadstub", "parentId": "no-such-row"}, http.StatusNotFound)
	_ = resp.Body.Close()
	h.Quiesce()

	assert.Equal(t, before, chatIDs(t, h, base), "a refused create creates nothing")
}
