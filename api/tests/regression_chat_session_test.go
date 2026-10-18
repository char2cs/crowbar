//go:build integration

package tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

type chatConversationView struct {
	ProviderID string `json:"providerId"`
	SessionID  string `json:"sessionId"`
	State      string `json:"state"`
}

type chatSessionView struct {
	LiveRunnerID string                `json:"liveRunnerId"`
	Conversation *chatConversationView `json:"conversation"`
	Session      *struct {
		ExitReason string `json:"exitReason"`
	} `json:"session"`
	Conversations []agentChatConversation `json:"conversations"`
}

func readChatSession(t *testing.T, h *harness, imported importedRepo, chatID string) chatSessionView {
	t.Helper()
	var view chatSessionView
	h.get(repoBase(imported)+"/chats/"+chatID, &view)
	return view
}

func (v chatSessionView) sessionIDs() []string {
	out := make([]string, 0, len(v.Conversations))
	for _, c := range v.Conversations {
		out = append(out, c.SessionID)
	}
	return out
}

// argAfter is the argv entry following flag, "" when flag is absent.
func argAfter(argv []string, flag string) string {
	for i, arg := range argv {
		if arg == flag && i+1 < len(argv) {
			return argv[i+1]
		}
	}
	return ""
}

func postSessionHook(t *testing.T, l kit.Launch, hookBase, provider, event string, payload map[string]any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	resp := l.PostHook(t, hookBase, provider, event, raw) //nolint:bodyclose // PostHook closes the body and returns a buffered copy
	require.Equal(t, http.StatusOK, resp.StatusCode, event)
}

// completeTurn posts one whole turn on session, as the CLI reports it.
func completeTurn(t *testing.T, h *harness, l kit.Launch, hookBase, provider, session, prompt string) {
	t.Helper()
	postSessionHook(t, l, hookBase, provider, "session_start", map[string]any{"session_id": session, "source": "startup"})
	postSessionHook(t, l, hookBase, provider, "user_prompt", map[string]any{"session_id": session, "prompt": prompt})
	// Claude sends MessageDisplay before Stop; a Stop with text and no open message waits.
	postSessionHook(t, l, hookBase, provider, "message_delta", map[string]any{
		"session_id": session, "turn_id": uuid.NewString(), "message_id": uuid.NewString(),
		"index": 0, "final": true, "delta": "ok",
	})
	postSessionHook(t, l, hookBase, provider, "turn_stop", map[string]any{"session_id": session, "last_assistant_message": "ok"})
	h.QuiesceReactors()
}

// promptLaunch submits text, staged onto provider when it is not "", and
// returns the launch that carries it.
func promptLaunch(
	t *testing.T, h *harness, imported importedRepo, fake kit.FakeProvider, chatID, text, provider string,
) kit.Launch {
	t.Helper()
	body := map[string]string{"text": text, "clientRequestId": uuid.NewString()}
	if provider != "" {
		body["provider"] = provider
	}
	resp := h.raw(http.MethodPost, repoBase(imported)+"/chats/"+chatID+"/prompts", body, http.StatusOK)
	require.NoError(t, resp.Body.Close())
	h.QuiesceReactors()
	live := getAgentChat(t, h, repoBase(imported), chatID).LiveRunnerID
	require.NotEmpty(t, live, "a prompt to a dormant chat launches a CLI")
	return fake.AwaitLaunch(t, live)
}

func killAndAwaitExit(t *testing.T, h *harness, conn *websocket.Conn, l kit.Launch) {
	t.Helper()
	l.Kill(t)
	readUntil(t, conn, func(m map[string]any) bool {
		return m["kind"] == "exited" && m["runnerId"] == l.RunnerID
	})
	h.QuiesceReactors()
}

func chatExitedWith(chatID, reason string) func(map[string]any) bool {
	return func(m map[string]any) bool {
		chat, _ := m["chat"].(map[string]any)
		session, _ := chat["session"].(map[string]any)
		return m["chatId"] == chatID && session["exitReason"] == reason
	}
}

func TestRegression_Symptom2_ALaunchCarriesASessionIDTheChatAlreadyOwns(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})

	chatID, l := createFakeChat(t, h, imported, fake)

	s := argAfter(l.Argv, "--session-id")
	_, err := uuid.Parse(s)
	require.NoError(t, err, "the first launch names the session the chat owns: %v", l.Argv)
	view := readChatSession(t, h, imported, chatID)
	require.NotNil(t, view.Conversation, "the chat owns its session before the CLI says a word")
	assert.Equal(t, chatConversationView{ProviderID: "claude", SessionID: s, State: "bound"}, *view.Conversation)
}

func TestRegression_Symptom2_EveryLaterLaunchResumesTheBoundSession(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	chatID, l1 := createFakeChat(t, h, imported, fake)
	hookBase := h.url + repoBase(imported)
	s := argAfter(l1.Argv, "--session-id")
	require.NotEmpty(t, s, "the first launch names its session: %v", l1.Argv)
	completeTurn(t, h, l1, hookBase, "claude", s, "one")

	l2 := promptLaunch(t, h, imported, fake, chatID, "two", "")
	assert.Equal(t, s, argAfter(l2.Argv, "--resume"))
	assert.NotContains(t, l2.Argv, "--session-id", "a bound session is resumed, never assigned again")

	conn := h.dial(repoBase(imported) + "/chats/ws")
	killAndAwaitExit(t, h, conn, l2)
	l3 := promptLaunch(t, h, imported, fake, chatID, "three", "")
	assert.Equal(t, s, argAfter(l3.Argv, "--resume"), "an exit with no hook loses nothing")

	h.shutdown()
	h2 := newHarnessAt(t, home)
	l4 := promptLaunch(t, h2, imported, fake, chatID, "four", "")
	assert.Equal(t, s, argAfter(l4.Argv, "--resume"), "a restarted daemon resumes the same session")
}

func TestRegression_Symptom2_HooksBeforeTheAnnouncementNeverUnbindTheSession(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	chatID, l1 := createFakeChat(t, h, imported, fake)
	hookBase := h.url + repoBase(imported)
	s := argAfter(l1.Argv, "--session-id")
	require.NotEmpty(t, s, "the first launch names its session: %v", l1.Argv)
	completeTurn(t, h, l1, hookBase, "claude", s, "one")

	l2 := promptLaunch(t, h, imported, fake, chatID, "two", "")
	require.Equal(t, s, argAfter(l2.Argv, "--resume"))
	postSessionHook(t, l2, hookBase, "claude", "user_prompt", map[string]any{"session_id": s, "prompt": "two"})
	postSessionHook(t, l2, hookBase, "claude", "message_delta", map[string]any{
		"session_id": s, "turn_id": uuid.NewString(), "message_id": uuid.NewString(),
		"index": 0, "final": false, "delta": "partial",
	})
	postSessionHook(t, l2, hookBase, "claude", "session_start", map[string]any{"session_id": s, "source": "resume"})
	h.QuiesceReactors()

	var users []string
	for _, m := range readRecordedMessages(t, h, imported, chatID) {
		if m.Role == "user" {
			users = append(users, m.Text)
		}
	}
	assert.Contains(t, users, "two", "a hook on the bound session is the chat's own")

	conn := h.dial(repoBase(imported) + "/chats/ws")
	killAndAwaitExit(t, h, conn, l2)
	l3 := promptLaunch(t, h, imported, fake, chatID, "three", "")
	assert.Equal(t, s, argAfter(l3.Argv, "--resume"))
}

func TestRegression_Symptom2_OnlyAnExplicitRefusalLosesTheSession(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	chatID, l1 := createFakeChat(t, h, imported, fake)
	hookBase := h.url + repoBase(imported)
	s := argAfter(l1.Argv, "--session-id")
	require.NotEmpty(t, s, "the first launch names its session: %v", l1.Argv)
	completeTurn(t, h, l1, hookBase, "claude", s, "one")
	conn := h.dial(repoBase(imported) + "/chats/ws")

	killed := promptLaunch(t, h, imported, fake, chatID, "two", "")
	killAndAwaitExit(t, h, conn, killed)
	view := readChatSession(t, h, imported, chatID)
	require.NotNil(t, view.Conversation)
	assert.Equal(t, chatConversationView{ProviderID: "claude", SessionID: s, State: "bound"}, *view.Conversation,
		"SIGTERM with no refusal line keeps the session")

	fake.RefuseResume(t, s, "No conversation found with session ID: "+s)
	requestID := uuid.NewString()
	postPrompt(t, h, imported, chatID, "three", requestID) // the CLI may die before the spawn answers
	// One pass for both frames: their order depends on whether the CLI died before
	// the spawn answered, and readUntil drops every frame it does not match.
	var settled map[string]any
	exited := false
	readUntil(t, conn, func(m map[string]any) bool {
		if isPromptSettled(requestID)(m) {
			settled = m
		}
		exited = exited || chatExitedWith(chatID, "resume_failed")(m)
		return settled != nil && exited
	})
	assert.Contains(t, []any{nil, false}, settled["promptConsumed"], "a refused resume took no prompt")
	h.QuiesceReactors()
	view = readChatSession(t, h, imported, chatID)
	require.NotNil(t, view.Conversation)
	assert.Equal(t, chatConversationView{ProviderID: "claude", SessionID: s, State: "lost"}, *view.Conversation)

	l4 := promptLaunch(t, h, imported, fake, chatID, "four", "")
	view = readChatSession(t, h, imported, chatID)
	require.NotNil(t, view.Conversation)
	s2 := view.Conversation.SessionID
	assert.Equal(t, chatConversationView{ProviderID: "claude", SessionID: s2, State: "bound"}, *view.Conversation)
	assert.NotEqual(t, s, s2, "a lost session is replaced by a fresh one")
	assert.Equal(t, s2, argAfter(l4.Argv, "--session-id"), "the prompt rides the fresh session: %v", l4.Argv)
	assert.NotContains(t, l4.Argv, "--resume")
}

func TestRegression_Symptom2_AnAnnouncedSessionChangeStaysOnTheChat(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	chatID, l := createFakeChat(t, h, imported, fake)
	hookBase := h.url + repoBase(imported)
	s, s2 := uuid.NewString(), uuid.NewString()
	postSessionHook(t, l, hookBase, "claude", "session_start", map[string]any{"session_id": s, "source": "startup"})
	h.QuiesceReactors()
	var before []agentChatDTO
	h.get(repoBase(imported)+"/chats", &before)

	postSessionHook(t, l, hookBase, "claude", "session_start", map[string]any{"session_id": s2, "source": "clear"})
	h.QuiesceReactors()

	var after []agentChatDTO
	h.get(repoBase(imported)+"/chats", &after)
	assert.Len(t, conversationsOnly(after), len(conversationsOnly(before)), "a session change mints no chat")
	view := readChatSession(t, h, imported, chatID)
	assert.Equal(t, l.RunnerID, view.LiveRunnerID, "the CLI stays on its chat")
	ids := view.sessionIDs()
	require.GreaterOrEqual(t, len(ids), 2, "%v", ids)
	assert.Equal(t, []string{s, s2}, ids[len(ids)-2:])
}

func TestRegression_Symptom2_ASessionOwnedByAnotherChatIsNeverTaken(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	chatA, la := createFakeChat(t, h, imported, fake)
	chatB, lb := createFakeChat(t, h, imported, fake)
	hookBase := h.url + repoBase(imported)
	sa, sb := uuid.NewString(), uuid.NewString()
	postSessionHook(t, la, hookBase, "claude", "session_start", map[string]any{"session_id": sa, "source": "startup"})
	postSessionHook(t, lb, hookBase, "claude", "session_start", map[string]any{"session_id": sb, "source": "startup"})
	h.QuiesceReactors()

	postSessionHook(t, lb, hookBase, "claude", "session_start", map[string]any{"session_id": sa, "source": "resume"})
	h.QuiesceReactors()

	var listed []agentChatDTO
	h.get(repoBase(imported)+"/chats", &listed)
	ids := map[string]bool{}
	for _, c := range conversationsOnly(listed) {
		ids[c.ID] = true
	}
	assert.True(t, ids[chatA] && ids[chatB], "both chats are still listed")
	assert.Equal(t, la.RunnerID, readChatSession(t, h, imported, chatA).LiveRunnerID)
	assert.Equal(t, lb.RunnerID, readChatSession(t, h, imported, chatB).LiveRunnerID,
		"the CLI that resumed another chat's session stays on its own chat")

	conn := h.dial(repoBase(imported) + "/chats/ws")
	killAndAwaitExit(t, h, conn, lb)
	next := promptLaunch(t, h, imported, fake, chatB, "again", "")
	assert.NotContains(t, next.Argv, sa, "chat B never takes chat A's session")
}

func TestRegression_Symptom2_SwitchingBackResumesTheProvidersOwnSession(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	alt := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude", ID: "claudealt"})
	chatID, l1 := createFakeChat(t, h, imported, fake)
	hookBase := h.url + repoBase(imported)
	s := argAfter(l1.Argv, "--session-id")
	require.NotEmpty(t, s, "the first launch names its session: %v", l1.Argv)
	completeTurn(t, h, l1, hookBase, "claude", s, "one")

	l2 := promptLaunch(t, h, imported, alt, chatID, "two", alt.ID)
	a1 := argAfter(l2.Argv, "--session-id")
	require.NotEmpty(t, a1, "the other provider gets a session of its own: %v", l2.Argv)
	assert.NotEqual(t, s, a1)
	completeTurn(t, h, l2, hookBase, alt.ID, a1, "two")

	l3 := promptLaunch(t, h, imported, fake, chatID, "three", fake.ID)
	assert.Equal(t, s, argAfter(l3.Argv, "--resume"), "switching back resumes the provider's own session")
}
