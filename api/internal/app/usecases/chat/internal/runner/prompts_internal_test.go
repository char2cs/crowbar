package runner

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// stubConversationsForPromptRestart answers only ChatSelection — the one
// call selectionRequiresRestart makes — panicking on anything else via the
// embedded nil interface, same pattern stubConversationsForSpawn
// (spawn_failure_internal_test.go) already uses.
type stubConversationsForPromptRestart struct {
	Conversations
	desired engineagents.Selection
}

func (s stubConversationsForPromptRestart) ChatSelection(
	context.Context, string, bool,
) (engineagents.Selection, error) {
	return s.desired, nil
}

// restartOnChangeAgent is a minimal engineagents.Agent whose SelectionRestart
// answers exactly like a real restart_tui model/effort block would
// (selection.RestartRequired): true whenever launched != desired. Standing
// one up this way, rather than resolving the real codex descriptor, keeps the
// test from depending on model discovery ever running.
type restartOnChangeAgent struct {
	engineagents.Agent
}

func (restartOnChangeAgent) SelectionRestart(launched, desired engineagents.Selection) bool {
	return launched != desired
}

// TestRegression_SubmitPromptOverAPI_ChangedSelectionRefusesTheLiveConnection
// reproduces the showstopper reported live: a codex chat with a LIVE api
// connection (the common case once session_start has fired — every prompt
// after the first) whose user picks a new model or effort and sends a
// message. Before this fix, submitPromptOverAPI pushed the text straight
// over the stale connection whenever one was live, never once comparing the
// launched process's model/effort against the chat's newly staged selection
// — confirmed live against a running codex process polled every 3s for 36s
// after each send: it never changed and never restarted, silently keeping
// the OLD model and effort for two full turns.
//
// handled MUST be false here: that is what makes submitPromptLocked
// (prompts.go) fall through to the restart_tui path instead, which tears the
// stale connection down (quitOutgoingCLI) and respawns with the new
// selection (spawn.go's resolveSelectionForSpawn + buildSpawnSteps render
// the new model/effort into the fresh process's argv — see
// TestBuildSpawnSteps_IncludesSelectionSteps, spawnsteps_internal_test.go,
// for that half of the property). Nothing here is silently dropped: a
// refused shortcut is a real restart, never a swallowed selection.
func TestRegression_SubmitPromptOverAPI_ChangedSelectionRefusesTheLiveConnection(t *testing.T) {
	live := engineagents.Runner{
		ID: "runner-1", ProviderID: "codex",
		LaunchModel: "gpt-5.6-luna", LaunchEffort: "xhigh",
	}
	desired := engineagents.Selection{Model: "gpt-5.6-sol", Effort: "medium"}

	rs := &Runners{
		apiConns:      newAPIConnRegistry(),
		conversations: stubConversationsForPromptRestart{desired: desired},
	}
	rs.apiConns.set(live.ID, &apiconn{})

	_, handled, err := rs.submitPromptOverAPI(
		context.Background(), domain.Chat{ID: "chat-1"}, "", "", "",
		live, restartOnChangeAgent{}, "", "reply with only: TWO",
	)

	require.NoError(t, err)
	assert.False(t, handled,
		"a selection change the descriptor declares restart_tui for must refuse the live-connection shortcut, never silently push it to the stale process")
}

// TestSubmitPromptOverAPI_UnchangedSelectionStillUsesTheLiveConnection is the
// companion property the fix must not break: an ordinary resend with nothing
// staged must keep using the live connection, never pay for a restart it
// never asked for.
func TestSubmitPromptOverAPI_UnchangedSelectionStillUsesTheLiveConnection(t *testing.T) {
	live := engineagents.Runner{
		ID: "runner-1", ProviderID: "codex",
		LaunchModel: "gpt-5.6-luna", LaunchEffort: "xhigh",
	}
	same := engineagents.Selection{Model: "gpt-5.6-luna", Effort: "xhigh"}

	rs := &Runners{
		apiConns:      newAPIConnRegistry(),
		conversations: stubConversationsForPromptRestart{desired: same},
	}
	rs.apiConns.set(live.ID, &apiconn{})

	// An unchanged selection must still reach durable dispatch (rs.prompts,
	// left nil here), which panics on Begin — the same "control flow got
	// past the guard being tested" signal every other narrow stub in this
	// package relies on, rather than mocking the full journal for a
	// property this test does not otherwise care about.
	assert.Panics(t, func() {
		_, _, _ = rs.submitPromptOverAPI(
			context.Background(), domain.Chat{ID: "chat-1"}, "", "", "",
			live, restartOnChangeAgent{}, "", "reply with only: DONE",
		)
	}, "an unchanged selection must still reach durable dispatch (rs.prompts), not stop at the restart guard")
}

// TestRegression_CommitPromptSpawn_APTYLessRunnerIsNotAMissingIdentity is the
// hazard the orphan-PTY fix exposed (apirunner.go): once an api-driven spawn
// forks no PTY at all, the runner row's TerminalSession is legitimately "" —
// and commitPromptSpawn refused exactly that as "replacement terminal
// identity is missing", which is the SUCCESS path of every api-dispatched
// prompt (submitPromptOverAPI's own tail). Unfixed, every single message to a
// codex chat comes back ErrPromptOutcomeUnknown.
//
// The identity that matters is the DELIVERY TARGET, and for this runner that
// is its live connection, not a terminal session it was never given.
func TestRegression_CommitPromptSpawn_APTYLessRunnerIsNotAMissingIdentity(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	h.seedLiveAPIConn(t, "runner-1")
	h.store.runner = engineagents.Runner{ID: "runner-1", ProviderID: "codex", TerminalSession: ""}

	dir := t.TempDir()
	_, _, err := h.rs.prompts.Begin(dir, "req-1", "hello", "hash-1", "codex", "", "runner-1", time.Now())
	require.NoError(t, err)

	got, err := h.rs.commitPromptSpawn(context.Background(), dir, "req-1", "hash-1", "runner-1")
	require.NoError(t, err, "a runner whose api connection IS its process has a delivery identity")
	assert.Equal(t, "runner-1", got.RunnerID)
	assert.Empty(t, got.TerminalSessionID, "there is no PTY to name, and naming one would be a lie")
}

// The refusal must SURVIVE for the case it was written for: a replacement
// spawn that produced neither a PTY nor a connection has no delivery target
// at all, and reporting success for it is how a prompt goes silently nowhere.
func TestCommitPromptSpawn_ARunnerWithNeitherPTYNorConnectionIsStillRefused(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	h.store.runner = engineagents.Runner{ID: "runner-1", ProviderID: "codex", TerminalSession: ""}

	dir := t.TempDir()
	_, _, err := h.rs.prompts.Begin(dir, "req-1", "hello", "hash-1", "codex", "", "runner-1", time.Now())
	require.NoError(t, err)

	_, err = h.rs.commitPromptSpawn(context.Background(), dir, "req-1", "hash-1", "runner-1")
	require.ErrorIs(t, err, ErrPromptOutcomeUnknown)
}
