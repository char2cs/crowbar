package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func shippedAgent(t *testing.T, id string) engineagents.Agent {
	t.Helper()
	a, err := engineagents.New().Get(context.Background(), t.TempDir(), id)
	require.NoError(t, err)
	return a
}

func TestRegression_ACodexChatBornOnTheTerminalOpensNoAPIConnection(t *testing.T) {
	codex := shippedAgent(t, "codex")

	assert.False(t,
		apiTransportDrivesSurface(codex, surfaceForSpawn(codex, engineagents.SurfaceTerminal)),
		"a codex chat born on the terminal must be driven by its own PTY, not an api connection")
}

func TestAPITransportDrivesSurface_CodexChatSurfaceAndTheDefaultBothKeepTheConnection(t *testing.T) {
	codex := shippedAgent(t, "codex")

	assert.True(t, apiTransportDrivesSurface(codex, surfaceForSpawn(codex, engineagents.SurfaceChat)))
	assert.True(t, apiTransportDrivesSurface(codex, surfaceForSpawn(codex, "")))
}

func TestAPITransportDrivesSurface_ClaudeIsDrivenByItsPTYOnEverySurface(t *testing.T) {
	claude := shippedAgent(t, "claude")

	assert.False(t, apiTransportDrivesSurface(claude, surfaceForSpawn(claude, engineagents.SurfaceTerminal)))
}

const noTerminalSurfaceDescriptor = `
id: surface-less
display_name: Surfaceless
spawn:
  cmd: surfaceless
  interactive_required: true
events:
  session_start: { in: SessionStart, map: { session_id: session_id } }
  turn_stop:     { in: Stop, map: { message: last } }
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    delivery: stdio
    format: ndjson
    serve: [surfaceless, serve]
    handshake: { call: initialize, notify: initialized }
surfaces:
  chat: { channel: api, start_here: true }
`

func TestSurfaceForSpawn_FallsBackWhenTheProviderDeclaresNoSuchLandingSurface(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(home, "descriptors", "surface-less.yaml"), []byte(noTerminalSurfaceDescriptor), 0o600))
	a, err := engineagents.New().Get(context.Background(), home, "surface-less")
	require.NoError(t, err)

	assert.Equal(t, "", surfaceForSpawn(a, engineagents.SurfaceTerminal))
	assert.True(t, apiTransportDrivesSurface(a, surfaceForSpawn(a, engineagents.SurfaceTerminal)),
		"falling back to the default landing must keep that provider's own transport")
}

func TestRegression_ATerminalBornChatIsShowingItsNativeView(t *testing.T) {
	rs := &Runners{attached: newAttachRegistry(), surfaces: newSurfaceRegistry()}
	rs.surfaces.set("runner-1", engineagents.SurfaceTerminal)

	assert.True(t, rs.ShowingNativeView("runner-1"),
		"how the chat GOT to the terminal cannot matter; only which surface it is on now")
}

func TestShowingNativeView_AChatSurfaceRunnerIsNot(t *testing.T) {
	rs := &Runners{attached: newAttachRegistry(), surfaces: newSurfaceRegistry()}
	rs.surfaces.set("runner-1", engineagents.SurfaceChat)

	assert.False(t, rs.ShowingNativeView("runner-1"))
}

func TestShowingNativeView_TheProviderDefaultLandingIsTheChatSurface(t *testing.T) {
	rs := &Runners{attached: newAttachRegistry(), surfaces: newSurfaceRegistry()}

	assert.False(t, rs.ShowingNativeView("never-recorded"))
}

func TestShowingNativeView_ASwitchedRunnerReadsTheSurfaceItSwitchedTo(t *testing.T) {
	rs := &Runners{attached: newAttachRegistry(), surfaces: newSurfaceRegistry()}
	rs.surfaces.set("runner-1", engineagents.SurfaceChat)
	rs.surfaces.set("runner-1", engineagents.SurfaceTerminal)

	assert.True(t, rs.ShowingNativeView("runner-1"))
}

func TestRegression_ASpawnSeedsTheRunnersCurrentSurfaceFromTheChat(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceTerminal)

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex",
		spawnInput{runnerID: "runner-1"})
	require.NoError(t, err)

	assert.True(t, h.rs.ShowingNativeView("runner-1"))
}

func TestSpawnRunner_AChatSurfaceSpawnSeedsTheChatSurface(t *testing.T) {
	h := newSpawnHarness(t, "")

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "claude",
		spawnInput{runnerID: "runner-1"})
	require.NoError(t, err)

	assert.False(t, h.rs.ShowingNativeView("runner-1"))
}

func TestSpawnRunner_ASeededSurfaceDegradesWithTheProvidersOwn(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceTerminal)

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "claude",
		spawnInput{runnerID: "runner-1"})
	require.NoError(t, err)

	assert.True(t, h.rs.ShowingNativeView("runner-1"))
}

func TestRegression_ARespawnRebuildsOnTheSurfaceTheUserIsOnNow(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceTerminal)

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex",
		spawnInput{runnerID: "runner-1"})
	require.NoError(t, err)

	assert.True(t, h.rs.ShowingNativeView("runner-1"))
	assert.False(t, h.rs.HasLiveAPIConnection("runner-1"),
		"a hooks-channel surface is fed by the CLI's own PTY; a connection beside it would hide it")
	assert.Len(t, h.term.created, 1, "that PTY is the session, so it is the one thing forked")
}

func TestReconcileRunnerExit_ForgetsTheRunnersSurface(t *testing.T) {
	rs, _ := ptylessRunners(t)
	rs.surfaces.set("runner-1", engineagents.SurfaceTerminal)

	rs.reconcileRunnerExit(context.Background(), "runner-1")

	assert.False(t, rs.ShowingNativeView("runner-1"))
}

type spySurfaceChats struct {
	stubChatsForSpawn
	written chan string
}

func newSpySurfaceChats() *spySurfaceChats {
	return &spySurfaceChats{
		stubChatsForSpawn: stubChatsForSpawn{chat: domain.Chat{ID: "chat-1"}},
		written:           make(chan string, 4),
	}
}

func (s *spySurfaceChats) lastWritten(t *testing.T) string {
	t.Helper()
	select {
	case surface := <-s.written:
		return surface
	default:
		t.Fatal("nothing wrote the chat's current surface")
		return ""
	}
}

func (s *spySurfaceChats) SetSurface(
	_ context.Context, _, surface string,
) (domain.Chat, error) {
	s.written <- surface
	return s.chat, nil
}

func TestRegression_SwitchToTerminalWritesTheChatsCurrentSurface(t *testing.T) {
	rs, chats := switchSurfaceFixture(t)

	_, err := rs.SwitchToTerminal(context.Background(), "chat-1")
	require.NoError(t, err)

	require.Equal(t, domain.SurfaceTerminal, chats.lastWritten(t))
	require.True(t, rs.ShowingNativeView("runner-1"),
		"the in-process mirror moves with the durable field, never independently of it")
}

func TestRegression_SwitchToNativeWritesTheChatsCurrentSurface(t *testing.T) {
	rs, chats := switchSurfaceFixture(t)

	_, err := rs.SwitchToTerminal(context.Background(), "chat-1")
	require.NoError(t, err)
	require.Equal(t, domain.SurfaceTerminal, chats.lastWritten(t))

	require.NoError(t, rs.SwitchToNative(context.Background(), "chat-1"))

	require.Equal(t, domain.SurfaceChat, chats.lastWritten(t))
	require.False(t, rs.ShowingNativeView("runner-1"))
}

func switchSurfaceFixture(t *testing.T) (*Runners, *spySurfaceChats) {
	t.Helper()
	stdio := fakeServe(t, nil)
	agent := attachTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	apiConn, err := agent.StartAPIConn(ctx, stdio, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = apiConn.Close() })

	chats := newSpySurfaceChats()
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(),
		surfaces: newSurfaceRegistry(), chatLock: newTestLock(), parks: newParks(),
		live:  liveWith(t, engineagents.Runner{ID: "runner-1", ProviderID: "attach-test", CurrentChatID: "chat-1"}),
		turns: noopTurns{},

		term:    &fakeTermForAttach{},
		chats:   chats,
		agents:  engineagents.New(),
		answers: answerdesk.New(answerdesk.DefaultRetention, noLedger{}),
		ledger:  turnedTestLedger(t, "attach-test", "sess-1"),
	}
	rs.apiConns.set("runner-1", &apiconn{
		driver: apiConn, ctx: ctx, agent: agent,
		tctx: engineagents.TemplateCtx{Session: "sess-1", Cwd: "/work"},
	})
	return rs, chats
}

func TestHotswapChat_AsksAndCompactFollowEveryTabSwitch(t *testing.T) {
	chats := &trackedSurfaceChats{chat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}}
	rs := &Runners{
		apiConns: newAPIConnRegistry(), attached: newAttachRegistry(),
		surfaces: newSurfaceRegistry(), chatLock: newTestLock(), parks: newParks(),
		live: liveWith(t, engineagents.Runner{
			ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "claude", CurrentChatID: "chat-1", TerminalSession: "pty-1",
		}),
		chats:         chats,
		ws:            stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:        stubAgentsForInterrupt{agent: shippedAgent(t, "claude")},
		conversations: stubConversationsForCompact{providerID: "claude"},
	}
	ctx := context.Background()
	onTerminal := func() {
		t.Helper()
		session, err := rs.SwitchToTerminal(ctx, "chat-1")
		require.NoError(t, err)
		assert.Equal(t, "pty-1", session, "the same process: nothing is forked")
		assert.True(t, rs.ShowingNativeView("runner-1"), "asks go to the TUI the user is on")
		assert.ErrorIs(t, rs.Compact(ctx, "chat-1"), ErrCompactionOffSurface)
	}
	onChat := func() {
		t.Helper()
		require.NoError(t, rs.SwitchToNative(ctx, "chat-1"))
		assert.False(t, rs.ShowingNativeView("runner-1"), "asks come back to Crowbar's chat")
		assert.Equal(t, domain.SurfaceChat, chats.chat.Surface, "compact's gate reads this record")
	}

	onTerminal()
	onChat()
	onTerminal()
}

type trackedSurfaceChats struct {
	stubChatsForSpawn
	chat domain.Chat
}

func (s *trackedSurfaceChats) GetChat(context.Context, string) (domain.Chat, error) {
	return s.chat, nil
}

func (s *trackedSurfaceChats) SetSurface(_ context.Context, _, surface string) (domain.Chat, error) {
	s.chat.Surface = surface
	return s.chat, nil
}
