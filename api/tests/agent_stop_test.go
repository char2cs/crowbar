//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegression_AgentStopKeepsChatDormantAndResumable(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	chatID, runnerID := createLiveStubChat(t, h, imported)

	postHook(t, h, repoBase(imported), "livestub", runnerID, "session_start", `{"session_id":"sess-stop-1"}`)
	postHook(t, h, repoBase(imported), "livestub", runnerID, "turn_stop", `{"session_id":"sess-stop-1","last_assistant_message":"hello"}`)
	h.Quiesce()

	feed := recordSidebar(t, h)
	resp := h.raw(http.MethodPost, repoBase(imported)+"/chats/"+chatID+"/stop", nil, http.StatusNoContent)
	_ = resp.Body.Close()
	feed.awaitRunnerGone(chatID, runnerID)

	post := getAgentChat(t, h, repoBase(imported), chatID)
	assert.Empty(t, post.LiveRunnerID,
		"closing a chat tab must terminate its live CLI: the chat is dormant, with no runner to attach to")
	assert.Equal(t, []string{"sess-stop-1"}, post.sessionIDs(),
		"a close STOPS the process but must keep the conversation it can be resumed into — this is not a delete")

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
