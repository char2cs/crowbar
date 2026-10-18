package runner

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// A session another chat holds stays that chat's: announcing it here records a
// conflict and leaves this chat's binding as it was.
func TestAnnounceSession_ASessionAnotherChatHoldsIsAConflict(t *testing.T) {
	l := turnedTestLedger(t, "claude", "S1")
	other, err := ledger.Assign(ledger.State{}, "claude", "S2")
	require.NoError(t, err)
	_, _, err = l.Append(t.Context(), "chat-2", other)
	require.NoError(t, err)
	rs := &Runners{ledger: l}

	changed, err := rs.announceSession(t.Context(), "chat-1", "claude", "S2")
	require.NoError(t, err)
	assert.False(t, changed)
	c, _ := chatState(t, l).Conversation("claude")
	assert.Equal(t, "S1", c.SessionID)
	assert.Equal(t, ledger.ConversationBound, c.State)
	rows, err := l.Read(t.Context(), "chat-1", 0, 0)
	require.NoError(t, err)
	last, err := ledger.DecodeBody[ledger.Marker](rows[len(rows)-1])
	require.NoError(t, err)
	assert.Equal(t, ledger.Marker{Kind: ledger.MarkerSessionConflict, Detail: "S2"}, last)
}

// Only the exit the descriptor recognises as the CLI's refusal loses the
// session; it settles what the runner carried and names resume_failed.
func TestLoseRefusedSession_OnlyTheCLIsRefusalLosesTheSession(t *testing.T) {
	refusal := "No conversation found with session ID: S1"
	for name, tc := range map[string]struct {
		exit engineterminal.Exit
		lost bool
	}{
		"the refusal":            {engineterminal.Exit{Code: 1, Screen: "loading\n" + refusal + "\n"}, true},
		"a kill":                 {engineterminal.Exit{Code: 143, Screen: refusal}, false},
		"a crash without a line": {engineterminal.Exit{Code: 1, Screen: "Killed"}, false},
	} {
		t.Run(name, func(t *testing.T) {
			rs := &Runners{
				ledger: boundTestLedger(t, "claude", "S1"),
				live:   liveWith(t, engineagents.Runner{ID: "runner-1", CurrentChatID: "chat-1", TerminalSession: "term-1"}),
			}
			appendPrompt(t, rs.ledger, "req-1", "runner-1")
			watch := resumeWatch{chatID: "chat-1", providerID: "claude", sessionID: "S1", agent: testAgent(t, "claude")}

			rs.loseRefusedSession(t.Context(), watch, "runner-1", tc.exit)

			st, err := rs.ledger.State(t.Context(), "chat-1")
			require.NoError(t, err)
			c, _ := st.Conversation("claude")
			p := promptReceipt(t, rs.ledger, "req-1")
			if !tc.lost {
				assert.Equal(t, ledger.ConversationBound, c.State)
				assert.Equal(t, ledger.PromptStatusPending, p.Status)
				assert.Empty(t, exitCause(t, rs.live, "runner-1"), "the runner exited on its own")
				return
			}
			assert.Equal(t, ledger.ConversationLost, c.State)
			assert.Equal(t, ledger.PromptStatusSettled, p.Status)
			assert.Equal(t, ledger.ReasonSessionLost, p.Reason)
			assert.Equal(t, domain.AgentExitResumeFailed, exitCause(t, rs.live, "runner-1"))
		})
	}
}

// A launch that resumed nothing has nothing to lose, whatever it exits with.
func TestLoseRefusedSession_AFreshLaunchLosesNothing(t *testing.T) {
	rs := &Runners{ledger: boundTestLedger(t, "claude", "S1"), live: newLiveTable()}
	rs.loseRefusedSession(t.Context(), resumeWatch{}, "runner-1",
		engineterminal.Exit{Code: 1, Screen: "No conversation found with session ID: S1"})

	st, err := rs.ledger.State(t.Context(), "chat-1")
	require.NoError(t, err)
	c, _ := st.Conversation("claude")
	assert.Equal(t, ledger.ConversationBound, c.State)
}

const apiLoseProvider = "api-lose-test"

const apiLoseTestDescriptor = `
id: api-lose-test
spawn:
  cmd: acme
  interactive_required: true
events:
  prompt:
    fresh:
      - call: thread/start
        capture: { session_id: thread.id }
    resume:
      - call: thread/resume
        send: { threadId: "{session_id}" }
    action:
      - call: turn/start
        send: { threadId: "{session_id}", text: "{text}" }
session:
  refused: { rpc_code: -32600, needles: ["no rollout found for thread id"] }
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve: [acme, serve]
    handshake: { call: initialize }
`

var refusedResume = map[string]map[string]any{
	"thread/resume": {"error": map[string]any{"code": -32600, "message": "no rollout found for thread id S1"}},
}

// apiLoseConn places on runner-1 a connection whose server answers each call
// from replies, and returns it with every method the server is asked; that
// channel closes once the connection is dropped.
func apiLoseConn(t *testing.T, rs *Runners, replies map[string]map[string]any) (*apiconn, <-chan string) {
	t.Helper()
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "descriptors", apiLoseProvider+".yaml"),
		[]byte(apiLoseTestDescriptor), 0o600))
	agent, err := engineagents.New().Get(t.Context(), home, apiLoseProvider)
	require.NoError(t, err)

	asked := make(chan string, 16)
	sock := fakeWSServer(t, func(conn *websocket.Conn) {
		defer close(asked)
		for {
			var req struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if conn.ReadJSON(&req) != nil {
				return
			}
			asked <- req.Method
			resp := map[string]any{"id": req.ID, "result": map[string]any{}}
			maps.Copy(resp, replies[req.Method])
			if resp["error"] != nil {
				delete(resp, "result")
			}
			if conn.WriteJSON(resp) != nil {
				return
			}
		}
	})
	driver, err := agent.StartAPIConn(t.Context(), sock, rs.sessionOrigin("chat-1", apiLoseProvider))
	require.NoError(t, err)
	conn := &apiconn{driver: driver, ctx: t.Context()}
	rs.apiConns.set("runner-1", conn)
	t.Cleanup(func() { rs.apiConns.drop("runner-1") })
	return conn, asked
}

// apiLoseLive holds runner-1 on chat-1 with no PTY: its connection is its
// only channel.
func apiLoseLive(t *testing.T) *liveTable {
	t.Helper()
	return liveWith(t, engineagents.Runner{ID: "runner-1", ProviderID: apiLoseProvider, CurrentChatID: "chat-1"})
}

// assertSessionLost: the chat's session is lost, the prompt runner-1 carried is
// settled as lost, runner-1 ends resume_failed, and no new thread was opened.
func assertSessionLost(t *testing.T, rs *Runners, asked <-chan string) {
	t.Helper()
	c, _ := chatState(t, rs.ledger).Conversation(apiLoseProvider)
	assert.Equal(t, ledger.ConversationLost, c.State)
	p := promptReceipt(t, rs.ledger, "req-1")
	assert.Equal(t, ledger.PromptStatusSettled, p.Status)
	assert.Equal(t, ledger.ReasonSessionLost, p.Reason)
	assert.Equal(t, domain.AgentExitResumeFailed, exitCause(t, rs.live, "runner-1"))
	_, live := rs.apiConns.get("runner-1")
	require.False(t, live, "the connection is dropped")
	var methods []string
	for m := range asked {
		methods = append(methods, m)
	}
	assert.Equal(t, []string{"thread/resume"}, methods)
}

func TestSessionOrigin_TheSessionAConnectionOpensIsBound(t *testing.T) {
	rs := &Runners{ledger: newTestLedger(t), apiConns: newAPIConnRegistry(), live: newLiveTable()}
	conn, _ := apiLoseConn(t, rs, map[string]map[string]any{
		"thread/start": {"result": map[string]any{"thread": map[string]any{"id": "thr-1"}}},
	})

	session, ok := rs.establishAPISession(t.Context(), conn, "runner-1", apiLoseProvider,
		engineagents.TemplateCtx{ChatID: "chat-1"})
	require.True(t, ok)
	assert.Equal(t, "thr-1", session)
	c, _ := chatState(t, rs.ledger).Conversation(apiLoseProvider)
	assert.Equal(t, ledger.ConversationBound, c.State)
	assert.Equal(t, "thr-1", c.SessionID)
	assert.Equal(t, ledger.CauseAnnounced, c.Cause)
}

func TestEstablishAPISession_ARefusedResumeLosesTheSession(t *testing.T) {
	rs := &Runners{ledger: turnedTestLedger(t, apiLoseProvider, "S1"), apiConns: newAPIConnRegistry(), live: apiLoseLive(t)}
	appendPrompt(t, rs.ledger, "req-1", "runner-1")
	conn, asked := apiLoseConn(t, rs, refusedResume)

	_, ok := rs.establishAPISession(t.Context(), conn, "runner-1", apiLoseProvider,
		engineagents.TemplateCtx{ChatID: "chat-1", Session: "S1"})
	require.False(t, ok)
	assertSessionLost(t, rs, asked)
}

func TestSubmitPromptOverAPI_ARefusedSessionIsLost(t *testing.T) {
	rs := &Runners{
		ledger: turnedTestLedger(t, apiLoseProvider, "S1"), apiConns: newAPIConnRegistry(), live: apiLoseLive(t),
		ws: fakeWSReader{chatsDir: t.TempDir()}, conversations: stubConversationsForPromptRestart{},
	}
	_, asked := apiLoseConn(t, rs, refusedResume)

	_, handled, err := rs.submitPromptOverAPI(t.Context(), domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}, "req-1",
		engineagents.Runner{ID: "runner-1", ProviderID: apiLoseProvider}, restartOnChangeAgent{}, t.TempDir(), "hi")
	assert.True(t, handled)
	require.ErrorIs(t, err, ErrPromptOutcomeUnknown)
	assertSessionLost(t, rs, asked)
}
