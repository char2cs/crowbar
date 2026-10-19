package runner

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
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
		context.Background(), domain.Chat{ID: "chat-1"}, "",
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

	// An unchanged selection must still reach dispatch (rs.ws, left nil here),
	// which panics — the same "control flow got past the guard being tested"
	// signal every other narrow stub in this package relies on.
	assert.Panics(t, func() {
		_, _, _ = rs.submitPromptOverAPI(
			context.Background(), domain.Chat{ID: "chat-1"}, "",
			live, restartOnChangeAgent{}, "", "reply with only: DONE",
		)
	}, "an unchanged selection must still reach dispatch, not stop at the restart guard")
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
	require.NoError(t, h.rs.live.add(engineagents.Runner{ID: "runner-1", ProviderID: "codex", CurrentChatID: "chat-1"}))
	require.NoError(t, h.rs.recordPrompt(t.Context(), "chat-1", "req-1", "hello", ledger.DeliveryAPI, "runner-1", nil))

	got, err := h.rs.commitPromptSpawn(t.Context(), "chat-1", "req-1", "runner-1")
	require.NoError(t, err, "a runner whose api connection IS its process has a delivery identity")
	assert.Equal(t, "runner-1", got.RunnerID)
	assert.Empty(t, got.TerminalSessionID, "there is no PTY to name, and naming one would be a lie")
}

// The refusal must SURVIVE for the case it was written for: a replacement
// spawn that produced neither a PTY nor a connection has no delivery target
// at all, and reporting success for it is how a prompt goes silently nowhere.
// The prompt is settled, so it holds the chat busy for nobody.
func TestCommitPromptSpawn_ARunnerWithNeitherPTYNorConnectionIsStillRefused(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	require.NoError(t, h.rs.live.add(engineagents.Runner{ID: "runner-1", ProviderID: "codex", CurrentChatID: "chat-1"}))
	require.NoError(t, h.rs.recordPrompt(t.Context(), "chat-1", "req-1", "hello", ledger.DeliveryRestart, "runner-1", nil))

	_, err := h.rs.commitPromptSpawn(t.Context(), "chat-1", "req-1", "runner-1")
	require.ErrorIs(t, err, ErrPromptOutcomeUnknown)
	st, err := h.rs.ledger.State(t.Context(), "chat-1")
	require.NoError(t, err)
	got, ok := st.Receipt("req-1")
	require.True(t, ok)
	assert.Equal(t, ledger.PromptStatusSettled, got.Status)
	assert.Equal(t, ledger.ReasonDeliveryFailed, got.Reason)
}

// A resend answers with its carrier's terminal session; a carrier it cannot
// read is an error, never a success that names no session at all.
func TestReplayPriorPrompt_ACarrierItCannotReadIsAnError(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	appendPrompt(t, h.rs.ledger, "req-1", "runner-gone")
	started, err := ledger.NewRow(ledger.KindTurnStarted, "T-req-1", "", ledger.TurnStarted{RequestID: "req-1"})
	require.NoError(t, err)
	_, _, err = h.rs.ledger.Append(t.Context(), "chat-1", started)
	require.NoError(t, err)

	_, done, err := h.rs.replayPriorPrompt(t.Context(), "chat-1", "req-1", "req-1")

	assert.True(t, done, "the request is a resend")
	require.ErrorIs(t, err, ErrNoLiveRunner)
}

// appendPrompt records a pending prompt on chat-1, carried by carrier, straight into l.
func appendPrompt(t *testing.T, l *ledgerstore.Store, requestID, carrier string) {
	t.Helper()
	row, err := ledger.NewRow(ledger.KindUserMessage, "T-"+requestID, "", ledger.UserMessage{
		RequestID: requestID, Text: requestID, Origin: ledger.OriginCrowbar,
		Delivery: ledger.DeliveryRestart, RunnerID: carrier,
	})
	require.NoError(t, err)
	_, _, err = l.Append(t.Context(), "chat-1", row)
	require.NoError(t, err)
}

func promptReceipt(t *testing.T, l *ledgerstore.Store, requestID string) ledger.Prompt {
	t.Helper()
	st, err := l.State(t.Context(), "chat-1")
	require.NoError(t, err)
	p, ok := st.Receipt(requestID)
	require.True(t, ok)
	return p
}

// newTestLedger is an in-memory chat ledger.
func newTestLedger(tb testing.TB) *ledgerstore.Store {
	tb.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(tb, err)
	l, err := ledgerstore.Open(db, time.Now)
	require.NoError(tb, err)
	return l
}
