//go:build integration

package tests

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// turnChat is a claude-fake chat whose CLI is running turn promptID.
type turnChat struct {
	h        *harness
	imported importedRepo
	fake     kit.FakeProvider
	chatID   string
	launch   kit.Launch
	conn     *websocket.Conn
}

func newTurnChat(t *testing.T, h *harness) turnChat {
	t.Helper()
	c := turnChat{h: h, imported: importWritableWorkspace(t, h)}
	c.fake = kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	c.chatID, c.launch = createFakeChat(t, h, c.imported, c.fake)
	c.conn = h.dial(repoBase(c.imported) + "/chats/ws")
	return c
}

// hook posts event as l's CLI, on the session l was launched with.
func (c turnChat) hook(t *testing.T, l kit.Launch, event string, payload map[string]any) {
	t.Helper()
	payload["session_id"] = argAfter(l.Argv, "--session-id")
	if resumed := argAfter(l.Argv, "--resume"); resumed != "" {
		payload["session_id"] = resumed
	}
	postSessionHook(t, l, c.h.url+repoBase(c.imported), "claude", event, payload)
}

// startTurn is the CLI reporting a prompt typed at its terminal: a turn
// Crowbar did not submit.
func (c turnChat) startTurn(t *testing.T, l kit.Launch, promptID string) {
	t.Helper()
	c.hook(t, l, "user_prompt", map[string]any{"prompt": "typed " + promptID, "prompt_id": promptID})
	require.True(t, c.working(t), "precondition: turn %s is running", promptID)
}

func (c turnChat) working(t *testing.T) bool {
	t.Helper()
	var chat struct {
		Working bool `json:"working"`
	}
	c.h.get(repoBase(c.imported)+"/chats/"+c.chatID, &chat)
	return chat.Working
}

func (c turnChat) frameFor(kind string) func(map[string]any) bool {
	return func(m map[string]any) bool { return m["chatId"] == c.chatID && m["kind"] == kind }
}

// Characterization: a CLI that dies mid-turn closes the turn it was running.
func TestRegression_Symptom4_ARunnerExitClosesItsTurnAndTheNextPromptGoesThrough(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	l := promptLaunch(t, c.h, c.imported, c.fake, c.chatID, "one", "")
	c.hook(t, l, "user_prompt", map[string]any{"prompt": "one", "prompt_id": uuid.NewString()})
	require.True(t, c.working(t), "precondition: the prompt's turn is running")

	l.Kill(t)
	readUntil(t, c.conn, c.frameFor("turn_stopped"))
	assert.False(t, c.working(t), "a CLI that died is running no turn")

	next := promptLaunch(t, c.h, c.imported, c.fake, c.chatID, "two", "")
	assert.NotEqual(t, l.RunnerID, next.RunnerID, "the next prompt launched a new CLI")
}

func TestRegression_Symptom4_StopClosesTheTurnAtOnce(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	c.startTurn(t, c.launch, "A")

	_ = c.h.raw(http.MethodPost, repoBase(c.imported)+"/chats/"+c.chatID+"/stop", nil, http.StatusAccepted).Body.Close()
	assert.False(t, c.working(t), "Stop records the turn's end before it returns")

	var kinds []string
	readUntil(t, c.conn, func(m map[string]any) bool {
		if c.frameFor("turn_stopped")(m) {
			kinds = append(kinds, "turn_stopped")
		}
		if m["kind"] == "exited" && m["runnerId"] == c.launch.RunnerID {
			kinds = append(kinds, "exited")
			return true
		}
		return false
	})
	assert.Equal(t, []string{"turn_stopped", "exited"}, kinds, "the turn closes before the CLI is told to quit")
}

func TestRegression_Symptom4_ProviderIdleClosesARunningTurn(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	c.startTurn(t, c.launch, "A")

	c.hook(t, c.launch, "idle", map[string]any{"message": "Claude is waiting for your input"})
	assert.False(t, c.working(t), "the provider saying it is idle closes the turn at once")
	assert.Equal(t, http.StatusOK, postPrompt(t, c.h, c.imported, c.chatID, "next", uuid.NewString()))
}

func TestRegression_Symptom4_ALateStopNeverClosesTheNextTurn(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	c.startTurn(t, c.launch, "A")
	_ = c.h.raw(http.MethodPost, repoBase(c.imported)+"/chats/"+c.chatID+"/stop", nil, http.StatusAccepted).Body.Close()
	c.h.QuiesceReactors()

	c.h.post(repoBase(c.imported)+"/chats/"+c.chatID+"/resume", nil, http.StatusOK, nil)
	c.h.QuiesceReactors()
	l2 := c.fake.AwaitLaunch(t, getAgentChat(t, c.h, repoBase(c.imported), c.chatID).LiveRunnerID)
	c.startTurn(t, l2, "B")

	c.hook(t, l2, "turn_stop", map[string]any{"prompt_id": "A", "last_assistant_message": ""})
	assert.True(t, c.working(t), "a stop naming turn A is late: B still runs")
	c.hook(t, l2, "turn_stop", map[string]any{"prompt_id": "B", "last_assistant_message": ""})
	assert.False(t, c.working(t), "B's own stop closes it")
}

func TestRegression_Symptom4_CrashMidTurnLeavesTheChatUsable(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	c := newTurnChat(t, h)
	c.startTurn(t, c.launch, "A")

	h.crash()
	h2 := newHarnessAt(t, home)
	h2.Quiesce()
	c.h = h2
	assert.False(t, c.working(t), "the turn the old daemon died in is closed")
	assert.Equal(t, "daemon_restart", exitReason(t, h2, c.imported, c.chatID))
	next := promptLaunch(t, h2, c.imported, c.fake, c.chatID, "after the crash", "")
	assert.NotEqual(t, c.launch.RunnerID, next.RunnerID, "the prompt launched a new CLI")
}

func TestRegression_Symptom4_ASwitchWhileATurnRunsIsRefusedNotForced(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	writeProviderDescriptor(t, c.h, "quietstub", quietStubProviderDescriptorYAML)
	c.startTurn(t, c.launch, "A")

	_ = c.h.raw(http.MethodPost, repoBase(c.imported)+"/chats/"+c.chatID+"/switch",
		map[string]string{"provider": "quietstub"}, http.StatusConflict).Body.Close()
	require.NoError(t, syscall.Kill(c.launch.PID, 0), "the running CLI was not killed")
	assert.True(t, c.working(t))
}

// chatDeadLetters is the daemon's count of hooks the ingress could not place.
func chatDeadLetters(t *testing.T, h *harness) int {
	t.Helper()
	var health struct {
		ChatDeadLetters *int `json:"chatDeadLetters"`
	}
	h.get("/v0/health", &health)
	require.NotNil(t, health.ChatDeadLetters, "the health route reports the dead-letter count")
	return *health.ChatDeadLetters
}

// hookStatus posts payload as l's CLI and returns the status, -1 when the
// request never got one: it runs off the test goroutine.
func hookStatus(l kit.Launch, base, event, payload string) int {
	url := fmt.Sprintf("%s/chats/runners/%s/hooks/claude/%s", base, l.RunnerID, event)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, strings.NewReader(payload))
	if err != nil {
		return -1
	}
	req.Header.Set("X-Crowbar-Runner-Token", l.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return -1
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestRegression_ConcurrentHooksForOneChatAllLand(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	c.startTurn(t, c.launch, "A")
	session := argAfter(c.launch.Argv, "--session-id")
	base := c.h.url + repoBase(c.imported)

	codes := make(chan int, 40)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tool := fmt.Sprintf(`{"session_id":%q,"tool_use_id":"tool-%d","tool_name":"Bash",`+
				`"tool_input":{"command":"ls"}}`, session, i)
			for _, event := range []string{"tool_pre", "tool_post"} {
				codes <- hookStatus(c.launch, base, event, tool)
			}
		}()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		assert.Equal(t, http.StatusOK, code)
	}

	var activity struct {
		ToolCalls []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"toolCalls"`
	}
	c.h.get(repoBase(c.imported)+"/chats/"+c.chatID+"/activity", &activity)
	require.Len(t, activity.ToolCalls, 20, "every tool call landed")
	for _, call := range activity.ToolCalls {
		assert.Equal(t, "ok", call.Status, call.ID)
	}
}

func TestRegression_HooksAfterChatDeleteAreRefusedNotResurrected(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	_ = c.h.raw(http.MethodDelete, repoBase(c.imported)+"/chats/"+c.chatID, nil, http.StatusAccepted).Body.Close()
	c.h.QuiesceReactors()
	before := chatDeadLetters(t, c.h)

	payload := fmt.Sprintf(`{"session_id":%q,"tool_use_id":"late","tool_name":"Bash"}`,
		argAfter(c.launch.Argv, "--session-id"))
	resp := c.launch.PostHook(t, c.h.url+repoBase(c.imported), "claude", "tool_pre", []byte(payload)) //nolint:bodyclose // PostHook closes the body and returns a buffered copy
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "a deleted chat's runner is unknown")
	assert.Equal(t, before+1, chatDeadLetters(t, c.h), "and its hook is dead-lettered")
	_ = c.h.raw(http.MethodGet, repoBase(c.imported)+"/chats/"+c.chatID, nil, http.StatusNotFound).Body.Close()
}

func TestRegression_AnUnmappableHookIsRefusedVisibly(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	before := chatDeadLetters(t, c.h)

	resp := c.launch.PostHook(t, c.h.url+repoBase(c.imported), "claude", "user_prompt", []byte(`{"session_id":"s"}`)) //nolint:bodyclose // PostHook closes the body and returns a buffered copy
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "a prompt hook without its prompt")
	assert.Equal(t, before+1, chatDeadLetters(t, c.h))
}

func TestRegression_AHookRepliesOnlyAfterItsRowsAreReadable(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	c.hook(t, c.launch, "user_prompt", map[string]any{"prompt": "readable on the reply", "prompt_id": "R"})

	var page struct {
		Items []recordedMessage `json:"items"`
	}
	c.h.get(repoBase(c.imported)+"/chats/"+c.chatID+"/messages?limit=200", &page)
	texts := make([]string, 0, len(page.Items))
	for _, m := range page.Items {
		texts = append(texts, m.Text)
	}
	assert.Contains(t, texts, "readable on the reply")
}
