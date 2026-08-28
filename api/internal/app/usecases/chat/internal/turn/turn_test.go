package turn_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// recordingRunners is the CLI lifecycle as a hook sees it: three calls, and
// nothing else. A hook may reach the lifecycle only through this port, and never
// through one of its gated doors — a switch holds the spawn gate while parked on
// a turn only this side can release.
type recordingRunners struct {
	turn.Runners

	mu       sync.Mutex
	sessions []string
}

func (r *recordingRunners) HandleSessionStart(
	_ context.Context,
	runner engineagents.Runner,
	_ engineagents.CanonicalEvent,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions = append(r.sessions, runner.ID)
	return nil
}

func (r *recordingRunners) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.sessions...)
}

// A hook that arrives before its runner row exists must be BUFFERED, not
// ingested: ingesting it would read a runner that is not there yet and drop the
// event on the floor.
func TestIngestHook_BuffersWhileTheRunnerIsStartingAndReplaysAfter(t *testing.T) {
	t.Parallel()

	pending := inflight.NewHooks()
	turns := turn.New(turn.Deps{
		PendingHooks: pending,
		Telemetry:    telemetry.New(),
	})
	runners := &recordingRunners{}
	turns.SetRunners(runners)

	require.NoError(t, pending.Register("runner-1"))
	require.NoError(t, turns.IngestHook(t.Context(), "runner-1", "claude", "session_start", []byte(`{}`)),
		"a buffered hook is absorbed, never refused: by the time it arrives the CLI has already acted")
	assert.Empty(t, runners.seen(), "nothing may be applied while the runner row does not exist")

	var replayed []inflight.Hook
	pending.Finish("runner-1", func(h inflight.Hook) { replayed = append(replayed, h) })

	require.Len(t, replayed, 1)
	assert.Equal(t, "session_start", replayed[0].CanonicalEvent)
}

// Placement is the lifecycle's job: a /clear or /resume inside the TUI moves the
// runner and Crowbar is told after the fact. The hook side decides only that a
// move happened.
func TestReplayStartupHook_RoutesASessionStartThroughTheRunnerPort(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	turns := turn.New(turn.Deps{
		Chats:        stubChats{},
		Runners:      stubRunnerStore{},
		Agents:       engineagents.New(),
		Workspace:    stubWorkspace{home: home},
		Home:         func() (string, error) { return home, nil },
		PendingHooks: inflight.NewHooks(),
		Telemetry:    telemetry.New(),
		Work:         inflight.NewWork(),
	})
	runners := &recordingRunners{}
	turns.SetRunners(runners)

	turns.ReplayStartupHook("runner-1", inflight.Hook{
		Provider:       "claude",
		CanonicalEvent: "session_start",
		RawPayload:     []byte(`{"session_id":"s-1","transcript_path":"/tmp/t.jsonl"}`),
	})

	assert.Equal(t, []string{"runner-1"}, runners.seen())
}

// A chat nobody has reported for is UNKNOWN, not zero: the header shows a context
// gauge off this, and "0% used" is a claim.
func TestTelemetry_IsUnknownUntilAProviderReports(t *testing.T) {
	t.Parallel()

	turns := turn.New(turn.Deps{Telemetry: telemetry.New()})

	_, ok := turns.Telemetry("chat-1")

	assert.False(t, ok)
}

// The screen classifiers are consulted by a SWEEP over live terminals. A sweep
// that failed on a screen it could not classify would stop classifying the ones
// it could, so every unresolvable input is silent rather than an error.
func TestMatchTerminal_IsSilentForEveryUnresolvableInput(t *testing.T) {
	t.Parallel()

	unresolvableHome := turn.New(turn.Deps{
		Agents: engineagents.New(),
		Home:   func() (string, error) { return "", errors.New("no home") },
	})
	_, ok := unresolvableHome.MatchTerminalPrompt(t.Context(), "claude", "❯ 1. Yes, I trust this folder")
	assert.False(t, ok, "an unreadable home is silent")
	_, ok = unresolvableHome.MatchTerminalNotice(t.Context(), "codex", "usage limit reached")
	assert.False(t, ok)

	home := t.TempDir()
	unknownProvider := turn.New(turn.Deps{
		Agents: engineagents.New(),
		Home:   func() (string, error) { return home, nil },
	})
	_, ok = unknownProvider.MatchTerminalPrompt(t.Context(), "telepathy", "anything")
	assert.False(t, ok, "an unknown provider is silent")
	_, ok = unknownProvider.MatchTerminalNotice(t.Context(), "telepathy", "anything")
	assert.False(t, ok)
}

// ChatWorking falls back to the aggregate while the process-local mirror knows
// nothing, and the mirror wins the moment it does. Assuming idle is what killed a
// CLI still doing background work after its turn ended.
func TestChatWorking_FallsBackToTheAggregateThenPrefersTheMirror(t *testing.T) {
	t.Parallel()

	work := inflight.NewWork()
	turns := turn.New(turn.Deps{Chats: stubChats{working: true}, Work: work})

	working, err := turns.ChatWorking(t.Context(), "chat-1")
	require.NoError(t, err)
	assert.True(t, working, "the aggregate says the chat IS working and the mirror knows nothing")

	work.Set("chat-1", false)
	working, err = turns.ChatWorking(t.Context(), "chat-1")
	require.NoError(t, err)
	assert.False(t, working, "a known mirror state is newer than any aggregate read")
}

// stubChats is the chat aggregate reduced to the one field these tests decide on.
type stubChats struct {
	agentchat.EventStore
	working bool
}

func (s stubChats) GetChat(_ context.Context, id string) (domain.Chat, error) {
	return domain.Chat{ID: id, WorkspaceID: "ws-1", Working: s.working}, nil
}

// stubWorkspace roots every path under one temp home.
type stubWorkspace struct{ home string }

func (w stubWorkspace) WorktreeDir(
	context.Context, string,
) (crowbarHome, projectID, repoID, worktree string, err error) {
	return w.home, "p1", "r1", w.home, nil
}

func (w stubWorkspace) AgentChatsDir(context.Context, string) (string, error) {
	return w.home, nil
}

// stubRunnerStore answers "which runner is this" and nothing else.
type stubRunnerStore struct {
	agentrunner.EventStore
}

func (stubRunnerStore) Get(_ context.Context, id string) (engineagents.Runner, error) {
	return engineagents.Runner{
		ID:            id,
		ProviderID:    "claude",
		WorkspaceID:   "ws-1",
		CurrentChatID: "chat-1",
	}, nil
}

// codexRunnerStore is stubRunnerStore for a codex runner instead of claude's —
// TestIngestHook_DropsAHooksDeliveredCopyOfAnAPIOwnedEvent needs a provider
// whose descriptor actually declares an api transport.
type codexRunnerStore struct {
	agentrunner.EventStore
}

func (codexRunnerStore) Get(_ context.Context, id string) (engineagents.Runner, error) {
	return engineagents.Runner{
		ID:            id,
		ProviderID:    "codex",
		WorkspaceID:   "ws-1",
		CurrentChatID: "chat-1",
	}, nil
}

// liveAPIRunners answers only HasLiveAPIConnection — every other Runners
// method embeds turn.Runners and panics if reached, which is deliberate: this
// test's whole point is that a redundant hooks delivery must return before
// touching any of them.
type liveAPIRunners struct {
	turn.Runners
	live bool
}

func (r liveAPIRunners) HasLiveAPIConnection(string) bool { return r.live }

// TestIngestHook_DropsAHooksDeliveredCopyOfAnAPIOwnedEvent guards the bug
// reported live 2026-08-28: while working with codex, some turns went missing
// mid-stream and then all reappeared at once when the turn finished. Every
// api-transport spawn also forks a real, hooks-wired companion PTY on the SAME
// session (attach.go's own "known gap"), and that PTY's hooks fire the
// descriptor's full hook set regardless of what TransportFor declares — so it
// echoes turn_stop a live api connection already reported. This turn_stop hook
// carries a runner_id whose chat this fixture wires no Chats/Activity/
// Conversations for at all: if the redundant delivery is not recognized and
// dropped BEFORE closeTurnFromStop runs, the call panics on a nil port instead
// of returning cleanly.
func TestIngestHook_DropsAHooksDeliveredCopyOfAnAPIOwnedEvent(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	turns := turn.New(turn.Deps{
		Runners:      codexRunnerStore{},
		Agents:       engineagents.New(),
		Workspace:    stubWorkspace{home: home},
		Home:         func() (string, error) { return home, nil },
		PendingHooks: inflight.NewHooks(),
		Telemetry:    telemetry.New(),
		Work:         inflight.NewWork(),
	})
	turns.SetRunners(liveAPIRunners{live: true})

	err := turns.IngestHook(t.Context(), "runner-1", "codex", "turn_stop",
		[]byte(`{"session_id":"s1","last_assistant_message":"the reply"}`))

	require.NoError(t, err)
}
