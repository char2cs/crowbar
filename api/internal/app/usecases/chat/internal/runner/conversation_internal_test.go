package runner

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/testutil"
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
    delivery: stdio
    format: ndjson
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
	stdio := fakeServe(t, func(conn *testutil.LinePeer) {
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
	driver, err := agent.StartAPIConn(t.Context(), stdio, rs.sessionOrigin("chat-1", apiLoseProvider))
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

// helperServeDescriptor is a provider whose `serve` is this test binary
// running TestHelperServe.
const helperServeDescriptor = `
id: carrier-test
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
  resume: { arg: "resume {id}" }
  refused: { rpc_code: -32600, needles: ["no rollout found for thread id"] }
  persists: turn_start
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    delivery: stdio
    format: ndjson
    serve: [%q, "-test.run=^TestHelperServe$", "--", "serve"]
    handshake: { call: initialize }
`

// TestHelperServe is not a test: it is the `serve` a helperServeDescriptor spawn
// forks, and returns at once anywhere else. Mode "refuse" refuses every resume;
// "drop-after <method>" hangs up once it has answered that call, and lives on;
// "exit" ends unanswered; any other mode answers every call until stdin closes.
func TestHelperServe(t *testing.T) {
	mode := os.Getenv(helperServeMode)
	if mode == "" || flag.NArg() != 1 {
		return
	}
	if mode == "exit" {
		os.Exit(0)
	}
	awaitServeGate(t)
	serve := testutil.NewLinePeer(os.Stdin, os.Stdout)
	dropAfter, drops := strings.CutPrefix(mode, "drop-after ")
	for {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		// Exits rather than returns: the test binary's own verdict would land
		// on stdout, which is the connection.
		if serve.ReadJSON(&req) != nil {
			os.Exit(0)
		}
		if len(req.ID) == 0 {
			continue
		}
		resp := map[string]any{"id": req.ID, "result": map[string]any{"thread": map[string]any{"id": "T1"}}}
		if mode == "refuse" && req.Method == "thread/resume" {
			resp = map[string]any{"id": req.ID, "error": refusedResume["thread/resume"]["error"]}
		}
		require.NoError(t, serve.WriteJSON(resp))
		if drops && req.Method == dropAfter {
			require.NoError(t, os.Stdout.Close())
			_, _ = io.Copy(io.Discard, os.Stdin)
			os.Exit(0)
		}
	}
}

// refusedResumeSpawn spawns a runner whose api resume the provider refuses:
// the session is lost and the spawn goes on over its PTY.
func refusedResumeSpawn(t *testing.T) (*spawnHarness, string) {
	t.Helper()
	h := helperServeSpawn(t, "refuse", false)

	runnerID, err := h.rs.spawnRunner(t.Context(), h.chatID, "ws-1", "carrier-test",
		spawnInput{runnerID: "runner-refused-resume"}, false)
	require.NoError(t, err)
	c, _ := chatState(t, h.rs.ledger).Conversation("carrier-test")
	require.Equal(t, ledger.ConversationLost, c.State, "the provider refused the resume")
	require.Len(t, h.term.created, 1, "the spawn went on over its PTY")
	return h, runnerID
}

const helperServeMode = "CROWBAR_TEST_SERVE"

// helperServeSpawn readies h to spawn a runner on a turned carrier-test chat
// whose serve runs as mode; attach makes the provider hotswap, so a PTY forks.
func helperServeSpawn(t *testing.T, mode string, attach bool) *spawnHarness {
	t.Helper()
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	t.Setenv("CROWBAR_DISABLE_API_TRANSPORT", "")
	t.Setenv(helperServeMode, mode)
	doc := fmt.Sprintf(helperServeDescriptor, os.Args[0])
	if attach {
		doc = strings.Replace(doc, "  transport: api\n", "  transport: api\n  hotswap: true\n", 1) +
			"    attach: [acme, resume, \"{session_id}\"]\n"
	}
	installDescriptor(t, h.home, doc)
	h.rs.ledger = turnedTestLedger(t, "carrier-test", "S1")
	h.rs.spawns = inflight.NewGate()
	return h
}

// endedAs is how chat-1's one recorded runner exit names its end.
func endedAs(t *testing.T, h *spawnHarness) string {
	t.Helper()
	rows, err := h.rs.ledger.Read(t.Context(), h.chatID, 0, 0)
	require.NoError(t, err)
	require.Equal(t, 1, countKind(rows, ledger.KindRunnerExited))
	exit, _ := chatState(t, h.rs.ledger).LastExit()
	return exit.Reason
}

// The runner a refused resume leaves is healthy: nothing waits it out, and
// the refusal is not why it ends when the user stops it.
func TestSpawnRunner_ARefusedAPIResumeLeavesAHealthyRunner(t *testing.T) {
	h, runnerID := refusedResumeSpawn(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	resumed, err := h.rs.ResumeChat(ctx, h.chatID)
	require.NoError(t, err, "a healthy runner is not waited out")
	assert.Equal(t, runnerID, resumed)
	v, _ := h.rs.live.view(h.chatID)
	assert.False(t, v.exiting)

	require.NoError(t, h.rs.StopChat(ctx, h.chatID))
	assert.Equal(t, domain.AgentExitStopped, endedAs(t, h))
}

// A daemon shutdown ends the runner a refused resume left as a daemon_restart.
func TestSpawnRunner_ARefusedAPIResumesRunnerEndsAtShutdownAsADaemonRestart(t *testing.T) {
	h, _ := refusedResumeSpawn(t)

	h.rs.Shutdown()
	require.NoError(t, h.term.TerminateGraceful(t.Context(), h.term.lastCall().termSessID))

	assert.Equal(t, domain.AgentExitDaemonRestart, endedAs(t, h))
}

// touches counts the snapshot republishes a chat is given.
type touches struct {
	*spyLifecycle
	n map[string]int
}

func (s touches) Touch(_ context.Context, chatID string) { s.n[chatID]++ }

// What a CLI reports it resolved to is kept beside what Crowbar launched it
// with, half by half: an unreported half keeps its value, and a report of
// nothing changes nothing.
func TestHandleSessionStart_KeepsWhatTheCLIReportsItLaunchedAs(t *testing.T) {
	snaps := touches{spyLifecycle: newSpyLifecycle(), n: map[string]int{}}
	rs := &Runners{
		ledger: newTestLedger(t), snapshots: snaps,
		live: liveWith(t, engineagents.Runner{
			ID: "runner-1", ProviderID: "claude", CurrentChatID: "chat-1",
			LaunchModel: "opus", LaunchEffort: "low",
		}),
	}
	runner, _ := rs.RunnerByID("runner-1")
	for _, report := range []engineagents.CanonicalEvent{
		{SessionID: "S1", Model: "claude-opus-4"},
		{SessionID: "S1", Effort: "high"},
		{SessionID: "S1"},
	} {
		require.NoError(t, rs.HandleSessionStart(t.Context(), runner, report))
	}

	got, _ := rs.RunnerByID("runner-1")
	assert.Equal(t, "claude-opus-4", got.ReportedModel)
	assert.Equal(t, "high", got.ReportedEffort)
	assert.Equal(t, "opus", got.LaunchModel, "what Crowbar launched with is never overwritten")
	assert.Equal(t, "low", got.LaunchEffort)
	assert.Equal(t, 2, snaps.n["chat-1"], "each report republishes the chat; an empty one does not")
}
