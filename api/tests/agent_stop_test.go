//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync"
	"syscall"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestRegression_AgentStopKeepsChatDormantAndResumable proves the close-a-chat-tab
// route end-to-end: POST .../chats/:id/stop gracefully terminates the chat's
// live vendor CLI but LEAVES THE CHAT — dormant and resumable — rather than deleting
// it. This is the counterpart of Delete (which erases the chat) and the whole point of
// the tab-close behaviour: the process stops, the conversation survives, and reopening
// revives it.
//
// It spawns LIVESTUB (`cat`), which holds its PTY open, so the chat genuinely has a live
// runner to stop; posts a session_start (binding a real conversation) and a turn_stop
// (writing the ledger) so there is a conversation to keep; then stops and asserts the
// chat is present, dormant, and still carries its bound conversation.
func TestRegression_AgentStopKeepsChatDormantAndResumable(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	chatID, runnerID := createLiveStubChat(t, h, imported)

	// A real, resumable conversation: the CLI announced its native session and wrote a
	// turn (a session id is not a conversation until something lands in it).
	postHook(t, h, repoBase(imported), "livestub", runnerID, "session_start", `{"session_id":"sess-stop-1"}`)
	postHook(t, h, repoBase(imported), "livestub", runnerID, "turn_stop", `{"session_id":"sess-stop-1","last_assistant_message":"hello"}`)
	h.Quiesce()

	// Close the tab: stop the CLI, keep the chat.
	resp := h.raw(http.MethodPost, repoBase(imported)+"/chats/"+chatID+"/stop", nil, http.StatusNoContent)
	_ = resp.Body.Close()
	h.Quiesce()

	// The chat is DORMANT — its CLI was terminated and no runner points at it — but it
	// is STILL THERE (a 200, not the Delete route's 404), with its bound conversation
	// retained. That is exactly the state ResumeChat consumes.
	post := getAgentChat(t, h, repoBase(imported), chatID)
	assert.Empty(t, post.LiveRunnerID,
		"closing a chat tab must terminate its live CLI: the chat is dormant, with no runner to attach to")
	assert.Equal(t, []string{"sess-stop-1"}, post.sessionIDs(),
		"a close STOPS the process but must keep the conversation it can be resumed into — this is not a delete")

	// And it still appears in List — a close is not a delete.
	var list []agentChatDTO
	h.get(repoBase(imported)+"/chats", &list)
	var found bool
	for _, c := range list {
		if c.ID == chatID {
			found = true
			assert.Empty(t, c.LiveRunnerID, "the listed chat is dormant after a stop")
		}
	}
	require.True(t, found, "a stopped chat must still appear in List — closing the tab does not delete it")
}

// POST /v0/chats/:chatId/kill is the force-kill: it answers 204, the projects
// scope hears the chat lose its runner, and the chat stays listed with its
// bound conversation.
func TestKill_FlatRouteEndsTheRunnerAndAnswers204(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	chatID, runnerID := createLiveStubChat(t, h, imported)
	postHook(t, h, repoBase(imported), "livestub", runnerID, "session_start", `{"session_id":"sess-kill-1"}`)
	postHook(t, h, repoBase(imported), "livestub", runnerID, "turn_stop", `{"session_id":"sess-kill-1","last_assistant_message":"hello"}`)
	h.Quiesce()
	feed := recordSidebar(t, h)

	live := getAgentChat(t, h, repoBase(imported), chatID)
	require.NotEmpty(t, live.LiveRunnerID, "precondition: the chat has a live runner to kill")

	resp := h.raw(http.MethodPost, "/v0/chats/"+chatID+"/kill", nil, http.StatusNoContent)
	_ = resp.Body.Close()
	feed.awaitRunnerGone(chatID, live.LiveRunnerID)

	post := getAgentChat(t, h, repoBase(imported), chatID)
	assert.Empty(t, post.LiveRunnerID, "a kill must leave no runner on the chat")
	assert.Equal(t, []string{"sess-kill-1"}, post.sessionIDs(), "a kill ends the process, never the conversation")
}

func TestKill_AnUnknownChatIs404(t *testing.T) {
	h := newHarness(t)

	resp := h.raw(http.MethodPost, "/v0/chats/"+uuid.NewString()+"/kill", nil, http.StatusNotFound)
	_ = resp.Body.Close()
}

// Kill has one route: the repo and home mounts no longer answer it.
func TestKill_TheNestedRouteIsGone(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	chatID, _ := createLiveStubChat(t, h, imported)

	for _, base := range []string{repoBase(imported), "/v0/projects/" + imported.projectID + "/home"} {
		resp := h.raw(http.MethodPost, base+"/chats/"+chatID+"/kill", nil, http.StatusNotFound)
		_ = resp.Body.Close()
	}
	assert.NotEmpty(t, getAgentChat(t, h, repoBase(imported), chatID).LiveRunnerID, "nothing killed the runner")
}

// A replacement launches only once the CLI it replaces is gone: the wait is on
// the old process's exit, never a clock.
func TestRegression_AReplacementNeverLandsBesideTheCLIItReplaces(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	chatID, l1 := createFakeChat(t, h, imported, fake)
	s := argAfter(l1.Argv, "--session-id")
	require.NotEmpty(t, s, "the first launch names its session: %v", l1.Argv)
	completeTurn(t, h, l1, h.url+repoBase(imported), "claude", s, "one")

	l2 := promptLaunch(t, h, imported, fake, chatID, "two", "")

	require.NotEqual(t, l1.RunnerID, l2.RunnerID, "precondition: prompt two restarted the CLI")
	assert.ErrorIs(t, syscall.Kill(l1.PID, 0), syscall.ESRCH,
		"launch 2 exists, so launch 1's process must already be gone")
}

// A prompt sent while a stop is tearing the CLI down is the chat's next turn:
// the stop ends the turn it was pressed on, never a request that came after it.
func TestRegression_APromptSentDuringAStopRunsAsTheNextTurn(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	c.startTurn(t, c.launch, "A")
	quitting, letGo := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(letGo) })
	gate := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(quitting)
		<-letGo
	}))
	defer gate.Close()
	defer release()
	// The CLI's way down waits on gate, so the stop stays under way until release.
	c.launch.OnQuit(t, c.fake, gate.URL, "claude", "turn_stop", []byte(`{}`))
	chat := c.h.url + repoBase(c.imported) + "/chats/" + c.chatID

	stopped := postAsync(t, chat+"/stop", nil, nil)
	<-quitting
	sent := make(chan struct{})
	prompted := postAsync(t, chat+"/prompts", map[string]string{"text": "next", "clientRequestId": uuid.NewString()}, sent)
	<-sent
	release()

	assert.Equal(t, http.StatusNoContent, <-stopped)
	require.Equal(t, http.StatusOK, <-prompted, "the prompt waits the stop out, then runs")
	c.h.QuiesceReactors()
	next := getAgentChat(t, c.h, repoBase(c.imported), c.chatID).LiveRunnerID
	require.NotEqual(t, c.launch.RunnerID, next, "the prompt launched the chat's next CLI")
	c.fake.AwaitLaunch(t, next)
}

// postAsync posts body to url and answers its status (0 when it failed);
// wrote, when set, is closed once the request is on the wire.
func postAsync(t *testing.T, url string, body any, wrote chan struct{}) <-chan int {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	ctx := t.Context()
	if wrote != nil {
		ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
			WroteRequest: func(httptrace.WroteRequestInfo) { close(wrote) },
		})
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	status := make(chan int, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			status <- 0
			return
		}
		if resp.Body.Close() != nil {
			status <- 0
			return
		}
		status <- resp.StatusCode
	}()
	return status
}
