package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
    serve: [surfaceless, serve]
    handshake: { call: initialize }
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
// disagree. surfaceGated (turn/ingest.go) derives the surface from
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

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex", "runner-1",
		nil, nil, "", 0, false, "", false, "")
	require.NoError(t, err)

	assert.True(t, h.rs.ShowingNativeView("runner-1"))
}

// And a chat-surface spawn seeds the chat surface, so nothing that predates
// surfaces changes behaviour.
func TestSpawnRunner_AChatSurfaceSpawnSeedsTheChatSurface(t *testing.T) {
	h := newSpawnHarness(t, "")

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "claude", "runner-1",
		nil, nil, "", 0, false, "", false, "")
	require.NoError(t, err)

	assert.False(t, h.rs.ShowingNativeView("runner-1"))
}

// A surface the provider cannot launch onto DEGRADES at spawn
// (surfaceForSpawn), and the seeded current surface has to degrade with it —
// otherwise a chat switched to a provider with no terminal surface would
// report a native view that provider cannot show.
func TestSpawnRunner_ASeededSurfaceDegradesWithTheProvidersOwn(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceTerminal)

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "claude", "runner-1",
		nil, nil, "", 0, false, "", false, "")
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

	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "codex", "runner-1",
		nil, nil, "", 0, false, "", false, "")
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
	store := newSpyRunnerStoreForSpawn()
	rs := ptylessRunners(store)
	rs.surfaces.set("runner-1", engineagents.SurfaceTerminal)

	rs.reconcileRunnerExit(context.Background(), "runner-1")

	assert.False(t, rs.ShowingNativeView("runner-1"))
}
