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

// TestRegression_ACodexChatBornOnTheTerminalOpensNoAPIConnection is the bug
// the user reported: a brand-new codex chat landed on the terminal surface and
// found nothing there ("This agent has no terminal view attached right now"),
// because every codex spawn opened an app-server connection — which forks a
// thread of its own, makes the PTY beside it a companion nobody is looking at,
// and has the DTO hide that PTY outright (HasLiveAPIConnection, dto/agent.go).
// The only way back to the terminal was SwitchToTerminal, whose `codex resume
// {id}` needs a flushed rollout codex does not write until a turn completes.
//
// codex's terminal surface is declared on the HOOKS channel, so a chat living
// there is fed by that PTY and needs no connection at all.
func TestRegression_ACodexChatBornOnTheTerminalOpensNoAPIConnection(t *testing.T) {
	codex := shippedAgent(t, "codex")

	assert.False(t,
		apiTransportDrivesSurface(codex, surfaceForSpawn(codex, engineagents.SurfaceTerminal)),
		"a codex chat born on the terminal must be driven by its own PTY, not an api connection")
}

// The chat surface is codex's api-channel one, and is still the default: a
// chat that named no surface must fork byte-identically to one created before
// surfaces existed.
func TestAPITransportDrivesSurface_CodexChatSurfaceAndTheDefaultBothKeepTheConnection(t *testing.T) {
	codex := shippedAgent(t, "codex")

	assert.True(t, apiTransportDrivesSurface(codex, surfaceForSpawn(codex, engineagents.SurfaceChat)))
	assert.True(t, apiTransportDrivesSurface(codex, surfaceForSpawn(codex, "")))
}

// claude speaks no api transport at all, so the question is moot for it either
// way — pinned so a future hooks provider that grows one inherits the same
// answer from the surface rather than from a provider name.
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

// A surface the chosen provider does not declare launchable DEGRADES to that
// provider's default rather than refusing: domain.Chat.Surface outlives a
// provider switch, and a chat born on one provider's terminal must still come
// up after being switched to a provider that has no such landing surface.
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

// TestRegression_ATerminalBornChatIsShowingItsNativeView is the collision the
// surface-birth work left behind: THREE notions of "which surface" that could
// disagree. surfaceGated (ingress/ingest.go) derives the surface from
// ShowingNativeView, which was true only for a chat that SWITCHED to the TUI
// — so a chat BORN there read `chat` while the user was looking at its
// terminal, and every event the descriptor gates to [chat] would have been
// ingested from a view nobody was watching.
//
// Inert only while the gated events are api-only, which a terminal-surface
// chat has none of. It stops being inert the moment codex declares a
// hooks-side delta.
func TestRegression_ATerminalBornChatIsShowingItsNativeView(t *testing.T) {
	rs := &Runners{attached: newAttachRegistry(), surfaces: newSurfaceRegistry()}
	rs.surfaces.set("runner-1", engineagents.SurfaceTerminal)

	assert.True(t, rs.ShowingNativeView("runner-1"),
		"how the chat GOT to the terminal cannot matter; only which surface it is on now")
}

// A chat on the api-channel chat surface is not showing a native view, born
// there or switched back to it.
func TestShowingNativeView_AChatSurfaceRunnerIsNot(t *testing.T) {
	rs := &Runners{attached: newAttachRegistry(), surfaces: newSurfaceRegistry()}
	rs.surfaces.set("runner-1", engineagents.SurfaceChat)

	assert.False(t, rs.ShowingNativeView("runner-1"))
}

// The provider's DEFAULT landing ("") is Crowbar's own chat — it is what
// every spawn that predates surfaces resolves to, and reading it as the
// terminal would gate every [chat] event off for all of them.
func TestShowingNativeView_TheProviderDefaultLandingIsTheChatSurface(t *testing.T) {
	rs := &Runners{attached: newAttachRegistry(), surfaces: newSurfaceRegistry()}

	assert.False(t, rs.ShowingNativeView("never-recorded"))
}

// SwitchToTerminal's attach registry stays the single answer to "which
// terminal session IS the native view", but it is no longer an independent
// answer to which SURFACE: the surface it moved to is what this reports.
func TestShowingNativeView_ASwitchedRunnerReadsTheSurfaceItSwitchedTo(t *testing.T) {
	rs := &Runners{attached: newAttachRegistry(), surfaces: newSurfaceRegistry()}
	rs.surfaces.set("runner-1", engineagents.SurfaceChat)
	rs.surfaces.set("runner-1", engineagents.SurfaceTerminal)

	assert.True(t, rs.ShowingNativeView("runner-1"))
}

// A spawn SEEDS the current surface from the chat's own durable one, so a
// terminal-born chat is on the terminal from its very first event — with no
// switch ever having happened, which is the whole point.
func TestRegression_ASpawnSeedsTheRunnersCurrentSurfaceFromTheChat(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceTerminal)

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex",
		spawnInput{runnerID: "runner-1"}, false)
	require.NoError(t, err)

	assert.True(t, h.rs.ShowingNativeView("runner-1"))
}

// And a chat-surface spawn seeds the chat surface, so nothing that predates
// surfaces changes behaviour.
func TestSpawnRunner_AChatSurfaceSpawnSeedsTheChatSurface(t *testing.T) {
	h := newSpawnHarness(t, "")

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "claude",
		spawnInput{runnerID: "runner-1"}, false)
	require.NoError(t, err)

	assert.False(t, h.rs.ShowingNativeView("runner-1"))
}

// A surface the provider cannot launch onto DEGRADES at spawn
// (surfaceForSpawn), and the seeded current surface has to degrade with it —
// otherwise a chat switched to a provider with no terminal surface would
// report a native view that provider cannot show.
func TestSpawnRunner_ASeededSurfaceDegradesWithTheProvidersOwn(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceTerminal)

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "claude",
		spawnInput{runnerID: "runner-1"}, false)
	require.NoError(t, err)

	// claude declares surfaces.terminal.start_here, so this one does NOT
	// degrade — it is a real landing for it too, and must read as such.
	assert.True(t, h.rs.ShowingNativeView("runner-1"))
}

// TestRegression_ARespawnRebuildsOnTheSurfaceTheUserIsOnNow is what the
// whole "current, not birth" change buys: every respawn (a restart_tui
// prompt, a model change, a resume after the daemon restarted) reads the
// chat's surface again — so a chat the user SWITCHED to the terminal comes
// back on the terminal, with no api connection opened beside the PTY that is
// now its session.
func TestRegression_ARespawnRebuildsOnTheSurfaceTheUserIsOnNow(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceTerminal)

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex",
		spawnInput{runnerID: "runner-1"}, false)
	require.NoError(t, err)

	assert.True(t, h.rs.ShowingNativeView("runner-1"))
	assert.False(t, h.rs.HasLiveAPIConnection("runner-1"),
		"a hooks-channel surface is fed by the CLI's own PTY; a connection beside it would hide it")
	assert.Len(t, h.term.created, 1, "that PTY is the session, so it is the one thing forked")
}

// A runner's surface is forgotten when the runner is, beside the echo guard
// it sits next to — otherwise the mirror grows for the life of the daemon and
// a recycled id could read a dead runner's surface.
func TestReconcileRunnerExit_ForgetsTheRunnersSurface(t *testing.T) {
	rs, _ := ptylessRunners(t)
	rs.surfaces.set("runner-1", engineagents.SurfaceTerminal)

	rs.reconcileRunnerExit(context.Background(), "runner-1")

	assert.False(t, rs.ShowingNativeView("runner-1"))
}

// spySurfaceChats records every SetSurface write, so a test can assert on the
// DURABLE surface rather than only on the in-process mirror of it.
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

// lastWritten is a NON-BLOCKING read: the write happens inside the call under
// test and has already returned by the time a test asks, so an empty channel
// means the write never happened — never that it has not happened yet.
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

// TestRegression_SwitchToTerminalWritesTheChatsCurrentSurface: the switch
// endpoints are the two moments Crowbar is actually TOLD the user moved, so
// they are what makes domain.Chat.Surface a current fact instead of a birth
// record. Without this the chat came back from a daemon restart (or any
// respawn) believing it was still on the surface it was created on, and
// rebuilt the wrong transport underneath the view the user was looking at.
func TestRegression_SwitchToTerminalWritesTheChatsCurrentSurface(t *testing.T) {
	rs, chats := switchSurfaceFixture(t)

	_, err := rs.SwitchToTerminal(context.Background(), "chat-1")
	require.NoError(t, err)

	require.Equal(t, domain.SurfaceTerminal, chats.lastWritten(t))
	require.True(t, rs.ShowingNativeView("runner-1"),
		"the in-process mirror moves with the durable field, never independently of it")
}

// TestRegression_SwitchToNativeWritesTheChatsCurrentSurface is the reverse:
// a chat handed back to Crowbar's own chat must stop reporting a native view.
func TestRegression_SwitchToNativeWritesTheChatsCurrentSurface(t *testing.T) {
	rs, chats := switchSurfaceFixture(t)

	_, err := rs.SwitchToTerminal(context.Background(), "chat-1")
	require.NoError(t, err)
	require.Equal(t, domain.SurfaceTerminal, chats.lastWritten(t))

	require.NoError(t, rs.SwitchToNative(context.Background(), "chat-1"))

	require.Equal(t, domain.SurfaceChat, chats.lastWritten(t))
	require.False(t, rs.ShowingNativeView("runner-1"))
}

// switchSurfaceFixture stands up the smallest Runners that can complete a
// real SwitchToTerminal: a live api connection over this file's in-process
// server, a completed turn on the runner's own session, and an idle chat.
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

// A hot-swap provider's one process serves both views, so the pane's tab IS the
// surface: each switch moves where asks are answered and whether compact is offered.
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

// trackedSurfaceChats keeps the last surface written, as the chat row does, so
// Compact's gate reads what the switches recorded.
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
