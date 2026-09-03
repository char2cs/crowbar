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

// This file pins what a chat's PARENT EDGE actually does to the agent on the
// other end of it, end to end over the real HTTP surface.
//
// A chat whose parent is another CHAT is a thread of it and reads that chat's
// turns. A chat whose parent is a FOLDER is merely filed there. The three
// properties below are the whole contract, and each one has failed silently
// before in a way only a black-box test can catch:
//
//   - A thread is SPAWNED pointed at its lineage — the ids, not the turns — and a
//     chat with no chat ancestors is spawned exactly as it always was.
//   - The lineage steps THROUGH folders, so filing a thread away never changes a
//     word of what it reads.
//   - Reading runs DOWN and only down. A chat may read its ancestors, its
//     siblings and anything else in scope; it may NOT read its own threads.
//
// The MCP calls go through .../chats/runners/:segid/mcp, the same route a vendor
// CLI's relay uses, authenticated with the per-runner token the daemon minted at
// spawn. The stub descriptor writes that token to a file under {tmp} exactly as a
// real descriptor writes its hook config there — so the test reaches the tool
// surface the way a CLI does, rather than reaching past the transport into the
// usecase.

// threadStubProviderDescriptorYAML spawns `cat`, which holds its PTY open, and
// renders the two things this suite has to read back: the runner's MCP token, and
// the {context} document the spawn injected.
//
// Writing {context} to a FILE is what makes the injected text assertable at all.
// A real provider's channel is a flag or a config key on a process the test never
// sees; write_file is the same declarative verb every descriptor already uses for
// its hook config, pointed at the one thing this suite is about.
const threadStubProviderDescriptorYAML = `id: threadstub
spawn:
  cmd: "cat"
  interactive_required: true
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

// threadContextBlock renders the CONFIGURED thread_lineage prompt with the ids
// filled in, so these tests assert the text a spawn actually injects rather than
// re-hardcoding a copy of it that could drift.
//
// It matters that this is the whole block and not a substring of it. The
// re-parent note Crowbar writes into the chat's own ledger ALSO names the parent
// and also says get_chat_log, and it rides into the same document inside the
// handoff — so an assertion looking merely for the id would pass with the spawn
// injection switched off entirely.
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
	dir := filepath.Join(h.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "threadstub.yaml"),
		[]byte(threadStubProviderDescriptorYAML), 0o644))
}

// newThreadChat creates a chat on the thread stub and returns it with the runner
// placed on it.
func newThreadChat(
	t *testing.T,
	h *harness,
	imported importedRepo,
) (chatID, runnerID string) {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	h.post(wsBase(imported)+"/chats", map[string]string{"provider": "threadstub"},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.Quiesce()
	detail := getAgentChat(t, h, wsBase(imported), created.ID)
	require.NotEmpty(t, detail.LiveRunnerID, "a freshly created chat has a runner on it")
	return created.ID, detail.LiveRunnerID
}

// newThreadChatUnder creates a chat BORN under parentID — the "New thread" and
// "new chat in this folder" gestures — and returns it with the runner placed on
// it. That runner is the chat's FIRST, which is the whole point: what it was told
// at spawn is what the user's first question is answered with.
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
	h.post(wsBase(imported)+"/chats",
		map[string]string{"provider": "threadstub", "parentId": parentID},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.Quiesce()
	detail := getAgentChat(t, h, wsBase(imported), created.ID)
	require.NotEmpty(t, detail.LiveRunnerID, "a freshly created chat has a runner on it")
	return created.ID, detail.LiveRunnerID
}

// runnerFile reads one file out of a runner's per-spawn directory — the {tmp} its
// descriptor rendered into. The path is derived the way the daemon derives it
// (<chatsDir>/runners/<runnerID>-<provider>) off the workspace's own chats dir, so
// a home-kind workspace whose chats reroot under crowbar home would be followed
// too.
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

// say drives one full exchange into a chat's ledger through the hook route, which
// is the ONLY way turns are ever recorded: Crowbar builds this record itself from
// vendor-CLI hooks and never reads a file the CLI wrote.
func say(
	t *testing.T,
	h *harness,
	imported importedRepo,
	runnerID string,
	prompt string,
	answer string,
) {
	t.Helper()
	for _, hook := range []map[string]string{
		{"event": "user_prompt", "payload_raw": `{"prompt":` + mustQuote(t, prompt) + `}`},
		{"event": "turn_stop", "payload_raw": `{"last_assistant_message":` + mustQuote(t, answer) + `}`},
	} {
		hook["segment_id"] = runnerID
		hook["provider"] = "threadstub"
		_ = h.raw(http.MethodPost, wsBase(imported)+"/chats/hooks", hook, http.StatusAccepted).Body.Close()
	}
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

// readChatLog calls the get_chat_log tool as runnerID, over the real MCP route
// with the real per-runner token, and returns the text the model would see.
//
// A refused call is NOT an HTTP error: a tool refusal rides back to the model as
// result text, so the refusal wording is exactly what this returns and exactly
// what the tests assert on.
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
	h.post(wsBase(imported)+"/chats/runners/"+runnerID+"/mcp", map[string]any{
		"token": runnerFile(t, h, imported, runnerID, "runner-token"),
		"rpc":   json.RawMessage(rpc),
	}, http.StatusOK, &out)
	return string(out.RPC)
}

// TestRegression_AThreadReadsTheChatItHangsOff is the permission the whole
// feature rests on, and it is asserted rather than assumed.
//
// Threads live in the same workspace as their parent, and get_chat_log has always
// been workspace-scoped, so admitting an ancestor was EXPECTED to be a no-op. But
// "expected to be" is not a test, and the descendant refusal added beside it is
// exactly the kind of check that overshoots by one edge and closes the direction
// the feature exists for.
func TestRegression_AThreadReadsTheChatItHangsOff(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	parentID, parentRunner := newThreadChat(t, h, imported)
	say(t, h, imported, parentRunner, "work out the plan", "the plan is to rewrite the parser")

	threadID, threadRunner := newThreadChat(t, h, imported)
	placeChat(t, h, wsBase(imported), threadID, map[string]any{"parentId": parentID})
	h.Quiesce()

	got := readChatLog(t, h, imported, threadRunner, parentID)
	assert.Contains(t, got, "the plan is to rewrite the parser",
		"a thread must be able to read the chat it continues — that edge is the whole point of it")
}

// TestRegression_AChatCannotReadItsOwnThread is the load-bearing refusal.
//
// Context in this tree runs DOWN. A thread is handed its ancestors and may fetch
// them; a parent is handed nothing about its threads and may not fetch them
// either. Without the second half, three threads off one chat stop being three
// independent attempts the moment the parent looks into any of them.
func TestRegression_AChatCannotReadItsOwnThread(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	parentID, parentRunner := newThreadChat(t, h, imported)
	threadID, threadRunner := newThreadChat(t, h, imported)
	say(t, h, imported, threadRunner, "try the parser", "the thread's own findings")
	placeChat(t, h, wsBase(imported), threadID, map[string]any{"parentId": parentID})
	h.Quiesce()

	got := readChatLog(t, h, imported, parentRunner, threadID)
	assert.Contains(t, got, "never below", "the refusal must say which direction is closed")
	assert.NotContains(t, got, "the thread's own findings",
		"a parent reading its own thread closes the loop the tree exists to keep open")
}

// And the refusal survives being filed away. A thread two folders deep under a
// chat is still that chat's thread — the lineage the refusal is decided from
// steps straight through folders — so a check written against the stored parent
// id would have missed every filed thread, which is the same as no check at all.
func TestRegression_AChatCannotReadAThreadItHasFiledInFolders(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

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

// TestRegression_SiblingThreadsStillReadEachOther pins the permission the
// refusal must not swallow.
//
// Cross-chat reading is DELIBERATE and predates threads: agents working the same
// repo compare notes through this tool, and two threads off one chat are two such
// agents. Only the downward edge is closed, and this test is here so nobody
// re-reads the rule as "only your ancestors" and tightens it.
func TestRegression_SiblingThreadsStillReadEachOther(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

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

// TestRegression_AThreadIsSpawnedPointedAtItsLineage pins the injection: the
// thread's next CLI comes up already knowing which chats it reads.
//
// A POINTER, never a paste. The document names ids and tells the agent to fetch
// them, so what it reads is the parent AS IT STANDS at the moment it asks —
// including everything the parent says after this spawn. Pasting the parent's
// turns here would freeze it at this instant and turn a live relationship into a
// snapshot nothing could refresh.
func TestRegression_AThreadIsSpawnedPointedAtItsLineage(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	parentID, parentRunner := newThreadChat(t, h, imported)
	say(t, h, imported, parentRunner, "work out the plan", "the parent's private reasoning")

	threadID, _ := newThreadChat(t, h, imported)
	placeChat(t, h, base, threadID, map[string]any{"parentId": parentID})
	h.Quiesce()

	// The next spawn on that chat. A provider switch is the shortest way to ask
	// for one over HTTP; a reopened tab and a revive take the same seam.
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

// A chat merely FILED in a folder inherits nothing, and must be spawned exactly
// as it is today — compared against a chat that has never been near the tree,
// rather than against a substring, so "exactly" means exactly.
func TestRegression_AFiledChatIsSpawnedWithNoThreadContext(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

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

// TestRegression_FoldersAreTransparentToWhatAThreadIsTold is the folder rule
// stated where it has to hold: in the document the CLI is actually handed.
//
// A thread two folders deep under a chat must resolve exactly the lineage of one
// sitting directly under it. Asserted against the OTHER document rather than
// against a literal, so the two shapes can never be updated apart.
func TestRegression_FoldersAreTransparentToWhatAThreadIsTold(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

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

// TestRegression_ReParentingIsRecordedInTheThreadsOwnLedger pins the decision
// that re-parenting is NOT retroactive.
//
// Dragging a chat that already has turns of its own under another chat makes it a
// thread FROM THE MOVE ONWARD. Nothing it already said was said with that
// context, and nothing rewrites it to pretend otherwise — a silent retroactive
// rewrite of what a chat has read is the version nobody can audit afterwards. So
// the move is written into the chat's own append-only record, at the point in the
// conversation where it actually happened.
func TestRegression_ReParentingIsRecordedInTheThreadsOwnLedger(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	parentID, _ := newThreadChat(t, h, imported)
	threadID, threadRunner := newThreadChat(t, h, imported)
	say(t, h, imported, threadRunner, "a question", "an answer given with none of that context")

	placeChat(t, h, base, threadID, map[string]any{"parentId": parentID})
	h.Quiesce()

	// Read the chat's own record back the way anything else would: through the
	// handoff a joining provider is handed.
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

// TestRegression_AChatCreatedUnderAChatIsThreadedOnItsFirstSpawn is the
// regression the create path exists to close.
//
// The panel's "New thread" gesture used to create a chat and place it afterwards,
// which meant the thread's CLI was already running by the time the parent edge was
// written. Its FIRST session — the one the user just asked for and is about to
// type a question into — came up knowing nothing about the chat it was threaded
// off, and the lineage only arrived on some later spawn the user never asked for.
// A user right-clicking "New thread" and asking a follow-up question got a
// stranger.
//
// So the chat is minted, PLACED, and only then started. This asserts against the
// very first runner the chat ever had; it fails if the placement moves back after
// the spawn, because that runner's document would no longer name the parent.
func TestRegression_AChatCreatedUnderAChatIsThreadedOnItsFirstSpawn(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	parentID, parentRunner := newThreadChat(t, h, imported)
	say(t, h, imported, parentRunner, "work out the plan", "the plan is to rewrite the parser")

	threadID, threadRunner := newThreadChatUnder(t, h, imported, parentID)

	assert.Equal(t, parentID, getAgentChat(t, h, base, threadID).ParentID,
		"the chat is born threaded, not threaded a moment later")
	assert.Contains(t, runnerFile(t, h, imported, threadRunner, "context.txt"),
		threadContextBlock(parentID),
		"the FIRST CLI on a new thread must already know which chat it continues")

	// And the pointer works from that first session: it can read the parent now.
	assert.Contains(t, readChatLog(t, h, imported, threadRunner, parentID),
		"the plan is to rewrite the parser")
}

// A chat created in a FOLDER is "new chat in this folder": it is placed there and
// inherits nothing, because folders hold no turns. Same request, same path, and
// the only difference is what the lineage resolves to — which is the folder rule
// doing its job rather than a second code path.
func TestRegression_AChatCreatedInAFolderIsPlacedButNotThreaded(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	folder := createChatFolder(t, h, base, "spikes", "")
	chatID, runnerID := newThreadChatUnder(t, h, imported, folder.ID)

	assert.Equal(t, folder.ID, getAgentChat(t, h, base, chatID).ParentID)
	assert.NotContains(t, runnerFile(t, h, imported, runnerID, "context.txt"), "THREAD CONTEXT",
		"a chat filed in a folder inherits nothing and must be spawned like any other")
}

// A chat created inside a folder that is itself inside a chat IS a thread of that
// chat — the create resolves lineage through folders exactly as a drag does.
func TestRegression_AChatCreatedInAFolderInsideAChatIsStillItsThread(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	parentID, _ := newThreadChat(t, h, imported)
	outer := createChatFolder(t, h, base, "attempts", parentID)
	inner := createChatFolder(t, h, base, "second pass", outer.ID)

	_, runnerID := newThreadChatUnder(t, h, imported, inner.ID)

	assert.Contains(t, runnerFile(t, h, imported, runnerID, "context.txt"),
		threadContextBlock(parentID),
		"folders are transparent to a create for the same reason they are to a drag")
}

// An absent parentId is the panel root and behaves exactly as it always did.
func TestRegression_AChatCreatedWithNoParentIsUnchanged(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	plainID, plainRunner := newThreadChat(t, h, imported)
	rootedID, rootedRunner := newThreadChatUnder(t, h, imported, "")

	assert.Empty(t, getAgentChat(t, h, base, plainID).ParentID)
	assert.Empty(t, getAgentChat(t, h, base, rootedID).ParentID)
	assert.Equal(t,
		runnerFile(t, h, imported, plainRunner, "context.txt"),
		runnerFile(t, h, imported, rootedRunner, "context.txt"),
		"an omitted parentId and an empty one are the same request, and it is today's request")
}

// A parentId naming nothing is refused, and NOTHING is left behind — no chat, no
// CLI. A create the user was told failed must not put a row in their panel.
func TestRegression_CreatingAChatUnderAnUnknownParentLeavesNothingBehind(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	before := chatIDs(t, h, base)
	resp := h.raw(http.MethodPost, base+"/chats",
		map[string]string{"provider": "threadstub", "parentId": "no-such-row"}, http.StatusNotFound)
	_ = resp.Body.Close()
	h.Quiesce()

	assert.Equal(t, before, chatIDs(t, h, base), "a refused create creates nothing")
}
