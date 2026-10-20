package runner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// stubWorkspaceForSpawn answers the two calls spawnPaths makes — WorktreeDir
// and AgentChatsDir — from fixed dirs under one temp home.
type stubWorkspaceForSpawn struct {
	seam.WorkspaceReader
	home, worktree, chatsDir string
}

func (s stubWorkspaceForSpawn) WorktreeDir(
	context.Context, string,
) (string, string, string, string, error) {
	return s.home, "p1", "r1", s.worktree, nil
}

func (s stubWorkspaceForSpawn) AgentChatsDir(
	context.Context, string,
) (string, error) {
	return s.chatsDir, nil
}

// stubProvidersForSpawn always enables the provider and never turns its MCP
// surface on — this test never touches tool injection.
type stubProvidersForSpawn struct{}

func (stubProvidersForSpawn) RequireProviderEnabled(context.Context, string) error { return nil }

func (stubProvidersForSpawn) ProviderMCPEnabled(context.Context, string) (bool, error) {
	return false, nil
}

// stubConversationsForSpawn answers only the two calls a create=true spawn's
// spawnPreflight makes — ThreadContext and ChatSelection — panicking on
// anything else via the embedded nil interface.
type stubConversationsForSpawn struct {
	Conversations
}

func (stubConversationsForSpawn) ThreadContext(context.Context, string, bool) (string, error) {
	return "", nil
}

func (stubConversationsForSpawn) ChatSelection(
	context.Context, string, bool,
) (engineagents.Selection, error) {
	return engineagents.Selection{}, nil
}

// boomSpawnPlanDescriptor is apiTransportTestDescriptor's own shape (see
// apiconn_internal_test.go) plus a spawn.args flag that also names itself in
// forbid_flags. Descriptor validation never cross-checks spawn.args against
// forbid_flags (only catalog/telemetry commands — see spawn_command.go and
// catalog_command.go), so this loads cleanly and fails only later, inside
// spawn.Plan's own checkForbidden — the one reachable way to make a REAL
// descriptor's SpawnPlan fail without hand-faking the whole Agent interface.
const boomSpawnPlanDescriptor = `
id: boom-test
spawn:
  cmd: acme
  interactive_required: true
  args: ["--boom"]
  forbid_flags: ["--boom"]
events:
  session_start:
    in: thread/started
    map: { session_id: thread.id }
  turn_stop:
    in: turn/completed
    map:
      session_id: threadId
      message: "turn.items[type=agentMessage].text"
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    delivery: stdio
    format: ndjson
    serve: [acme, serve]
    handshake: { call: initialize }
`

func TestSpawnRunner_SpawnPlanFailure_ReturnsTheBuildError(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(home, "descriptors", "boom-test.yaml"), []byte(boomSpawnPlanDescriptor), 0o600))

	worktree := filepath.Join(home, "worktree")
	chatsDir := filepath.Join(home, "chats")

	rs := &Runners{
		agents:        engineagents.New(),
		ws:            stubWorkspaceForSpawn{home: home, worktree: worktree, chatsDir: chatsDir},
		providers:     stubProvidersForSpawn{},
		conversations: stubConversationsForSpawn{},
		ledger:        newTestLedger(t),
		live:          newLiveTable(),
		chatLock:      newTestLock(), parks: newParks(),
	}

	_, err := rs.spawnRunner(context.Background(), "chat-1", "ws-1", "boom-test",
		spawnInput{runnerID: "runner-1", promptMessage: "hello"}, true)

	require.Error(t, err)
	assert.ErrorContains(t, err, "build spawn plan")
}

func TestSpawnRunner_DescriptorResolveFailure_ReturnsTheResolveError(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	// Deliberately no descriptor file for "missing-test" — rs.agents.Get must
	// fail to resolve it.

	worktree := filepath.Join(home, "worktree")
	chatsDir := filepath.Join(home, "chats")

	rs := &Runners{
		agents:        engineagents.New(),
		ws:            stubWorkspaceForSpawn{home: home, worktree: worktree, chatsDir: chatsDir},
		providers:     stubProvidersForSpawn{},
		conversations: stubConversationsForSpawn{},
		ledger:        newTestLedger(t),
		live:          newLiveTable(),
	}

	_, err := rs.spawnRunner(context.Background(), "chat-1", "ws-1", "missing-test",
		spawnInput{runnerID: "runner-1", promptMessage: "hello"}, true)

	require.Error(t, err)
	assert.ErrorContains(t, err, "resolve descriptor")
}

// okHooksDescriptor is a minimal, entirely valid hooks-transport descriptor —
// unlike boomSpawnPlanDescriptor, its SpawnPlan succeeds — used by the test
// below that needs to reach forkCLI with a real, working descriptor. hooks
// (not api) transport so this never touches applyAPITransport's serve-process
// machinery at all.
const okHooksDescriptor = `
id: ok-test
spawn:
  cmd: acme
  interactive_required: true
events:
  session_start:
    in: SessionStart
    map: { session_id: session_id }
  turn_stop:
    in: Stop
    map:
      session_id: session_id
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
    delivery: http
`

// unforkable is a terminal no CLI forks on.
type unforkable struct{ blindTerminal }

func (unforkable) CreateCommand(context.Context, string, string, []string, []string, func(engineterminal.Exit)) (string, error) {
	return "", errors.New("boom: the CLI would not fork")
}

// A respawn that puts no CLI on an existing chat is recorded as spawn_failed,
// and the prompt it was to carry is settled rather than left waiting on it.
func TestSpawnRunner_AFailedRespawnRecordsSpawnFailedAndSettlesWhatItCarried(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(home, "descriptors", "ok-test.yaml"), []byte(okHooksDescriptor), 0o600))
	rs := &Runners{
		agents:        engineagents.New(),
		ws:            stubWorkspaceForSpawn{home: home, worktree: filepath.Join(home, "worktree"), chatsDir: filepath.Join(home, "chats")},
		providers:     stubProvidersForSpawn{},
		conversations: stubConversationsForSpawn{},
		chats:         stubChatsForSpawn{chat: domain.Chat{ID: "chat-1"}},
		ledger:        newTestLedger(t),
		live:          newLiveTable(),
		term:          unforkable{},
	}
	appendPrompt(t, rs.ledger, "req-1", "runner-1")

	_, err := rs.spawnRunner(context.Background(), "chat-1", "ws-1", "ok-test",
		spawnInput{runnerID: "runner-1"}, false)

	require.Error(t, err)
	exit, ok := chatState(t, rs.ledger).LastExit()
	require.True(t, ok)
	assert.Equal(t, ledger.RunnerExit{RunnerID: "runner-1", Reason: domain.AgentExitSpawnFailed, At: exit.At}, exit)
	p := promptReceipt(t, rs.ledger, "req-1")
	assert.Equal(t, ledger.PromptStatusSettled, p.Status)
	assert.Equal(t, ledger.ReasonDeliveryFailed, p.Reason)
	_, live := rs.RunnerByID("runner-1")
	assert.False(t, live, "a failed spawn leaves nothing in the live table")
}

// A carried prompt whose push finds the session gone loses the session, as a
// refused establish does: the spawn ends resume_failed, never spawn_failed.
func TestSpawnRunner_ACarriedPushThatFindsTheSessionGoneLosesIt(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	require.NoError(t, os.MkdirAll(filepath.Join(h.home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(h.home, "descriptors", apiLoseProvider+".yaml"),
		[]byte(apiLoseTestDescriptor), 0o600))
	h.rs.ledger = turnedTestLedger(t, apiLoseProvider, "S1")
	appendPrompt(t, h.rs.ledger, "req-1", "runner-1")
	conn, _ := apiLoseConn(t, h.rs, refusedResume)
	serve := exec.CommandContext(t.Context(), "sleep", "30")
	require.NoError(t, serve.Start())
	conn.serve, conn.tctx = reapServe(serve), engineagents.TemplateCtx{Session: "S1"}

	_, err := h.rs.spawnRunner(t.Context(), h.chatID, "ws-1", apiLoseProvider,
		spawnInput{runnerID: "runner-1", promptMessage: "hi"}, false)

	require.ErrorIs(t, err, ErrSpawnPromptUndeliverable)
	st := chatState(t, h.rs.ledger)
	c, _ := st.Conversation(apiLoseProvider)
	assert.Equal(t, ledger.ConversationLost, c.State)
	assert.Equal(t, ledger.ReasonSessionLost, promptReceipt(t, h.rs.ledger, "req-1").Reason)
	exit, _ := st.LastExit()
	assert.Equal(t, domain.AgentExitResumeFailed, exit.Reason)
	rows, err := h.rs.ledger.Read(t.Context(), h.chatID, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, countKind(rows, ledger.KindSessionLost))
	assert.False(t, h.rs.HasLiveAPIConnection("runner-1"), "the connection is dropped")
}

// A send whose launch finds its session refused by the connection forks
// nothing: its plan resumes that session, and the prompt it carried is settled.
func TestSubmitPrompt_ALaunchWhoseSessionTheConnectionRefusesForksNothing(t *testing.T) {
	h := helperServeSpawn(t, "refuse", false)
	asked := filepath.Join(t.TempDir(), "asked")
	t.Setenv(helperServeAsked, asked)
	h.rs.chats = stubChatsForSpawn{chat: domain.Chat{
		ID: h.chatID, WorkspaceID: "ws-1", ProviderID: "carrier-test", Surface: engineagents.SurfaceChat,
	}}
	h.rs.conversations, h.rs.activity = reviveConversations{provider: "carrier-test"}, reviveActivity{}
	requestID := uuid.NewString()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	_, err := h.rs.SubmitPromptWithSwitch(ctx, h.chatID, "hello", requestID, "", nil)

	require.ErrorIs(t, err, ErrPromptOutcomeUnknown)
	assert.Empty(t, h.term.created, "no PTY forks to resume the refused session")
	assert.Equal(t, []string{"initialize", "thread/resume"}, askedCalls(t, asked), "one CLI was launched")
	assert.Equal(t, ledger.ReasonSessionLost, promptReceipt(t, h.rs.ledger, requestID).Reason)
	assert.Equal(t, domain.AgentExitResumeFailed, endedAs(t, h))
	rows, err := h.rs.ledger.Read(t.Context(), h.chatID, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, countKind(rows, ledger.KindSessionLost))
}

// An attaching spawn whose view cannot fork leaves no connection behind.
func TestSpawnRunner_AnAttachingSpawnThatCannotForkDropsItsConnection(t *testing.T) {
	h := helperServeSpawn(t, "answer", true)
	h.rs.term = unforkable{}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	_, err := h.rs.spawnRunner(ctx, h.chatID, "ws-1", "carrier-test", spawnInput{runnerID: "runner-1"}, false)

	require.Error(t, err)
	assert.False(t, h.rs.HasLiveAPIConnection("runner-1"), "the serve it would have attached to is dropped")
}

// A chat holds one CLI: a spawn onto a chat whose runner is live is refused
// before anything forks, and the live runner keeps the chat.
func TestSpawnRunner_ASecondRunnerOnALiveChatIsRefusedBeforeItForks(t *testing.T) {
	h := newSpawnHarness(t, "")
	h.rs.live = liveWith(t, engineagents.Runner{ID: "runner-0", ProviderID: "claude", CurrentChatID: "chat-1"})

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "claude", spawnInput{runnerID: "runner-1"}, false)

	require.ErrorIs(t, err, ErrChatHasLiveRunner)
	assert.Empty(t, h.term.created, "nothing forked")
	live, err := h.rs.LiveRunnerForChat(t.Context(), "chat-1")
	require.NoError(t, err)
	assert.Equal(t, "runner-0", live.ID)
	_, recorded := chatState(t, h.rs.ledger).LastExit()
	assert.False(t, recorded, "a refused spawn records nothing over the live runner")
}

// A turn a hook opened on a spawn that then failed is closed with it, and none
// opens on the runner after: the chat is free, and nothing waits on that turn.
func TestRecordSpawnFailure_ClosesATurnOpenedInTheWindow(t *testing.T) {
	runner := engineagents.Runner{ID: "runner-1", ProviderID: "claude", CurrentChatID: "chat-1"}
	rs := &Runners{
		ledger: newTestLedger(t),
		live:   newLiveTable(),
	}
	require.NoError(t, rs.live.add(runner))
	started, err := ledger.NewRow(ledger.KindRunnerStarted, "", "", ledger.RunnerStarted{RunnerID: runner.ID, Provider: "claude"})
	require.NoError(t, err)
	turn, err := ledger.NewRow(ledger.KindTurnStarted, "T-1", "", ledger.TurnStarted{RunnerID: runner.ID})
	require.NoError(t, err)
	_, _, err = rs.ledger.Append(t.Context(), runner.CurrentChatID, started, turn)
	require.NoError(t, err)
	require.True(t, chatState(t, rs.ledger).Busy(), "precondition: a turn runs on the spawning runner")

	spawnErr := errors.New("spawn failed")
	rs.recordSpawnFailure(t.Context(), runner.CurrentChatID, runner.ID, false, &spawnErr)

	assert.False(t, chatState(t, rs.ledger).Busy(), "the failed spawn left its turn running")
	opened, err := rs.TurnRunner(runner.ID, func(engineagents.Runner) error { return nil })
	require.NoError(t, err)
	assert.False(t, opened, "a turn opened on a runner whose spawn failed")
}
