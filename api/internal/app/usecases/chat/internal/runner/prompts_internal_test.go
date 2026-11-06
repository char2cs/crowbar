package runner

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
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
	context.Context, string,
) (engineagents.Selection, error) {
	return s.desired, nil
}

// restartOnChangeAgent answers SelectionRestart like a restart_tui model/effort
// block (true whenever launched != desired), so the test never runs model discovery.
type restartOnChangeAgent struct {
	engineagents.Agent
}

func (restartOnChangeAgent) SelectionRestart(launched, desired engineagents.Selection) bool {
	return launched != desired
}

// A live api connection must not carry a prompt after the user changed model or
// effort: handled is false so submitPromptLocked restarts the CLI with the new
// selection instead of silently keeping the old one.
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

// An api-driven runner forks no PTY, so its empty TerminalSession is no missing
// identity: its delivery target is the live connection, and refusing it would
// fail every api-dispatched prompt.
func TestRegression_CommitPromptSpawn_APTYLessRunnerIsNotAMissingIdentity(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	h.seedLiveAPIConn(t, "runner-1")
	require.NoError(t, h.rs.live.add(engineagents.Runner{ID: "runner-1", ProviderID: "codex", CurrentChatID: "chat-1"}))
	require.NoError(t, h.rs.recordPrompt(t.Context(), "chat-1", "req-1", "hello", "", ledger.DeliveryAPI, "runner-1", nil))

	got, err := h.rs.commitPromptSpawn(t.Context(), "chat-1", "req-1", "runner-1")
	require.NoError(t, err, "a runner whose api connection IS its process has a delivery identity")
	assert.Equal(t, "runner-1", got.RunnerID)
	assert.Empty(t, got.TerminalSessionID, "there is no PTY to name, and naming one would be a lie")
}

// A prompt failed for its lost session is settled as lost, not as a generic
// failed delivery, whoever settles it.
func TestFailPrompt_ALostSessionSettlesThePromptAsLost(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	require.NoError(t, h.rs.recordPrompt(t.Context(), "chat-1", "req-1", "hello", "", ledger.DeliveryRestart, "runner-1", nil))

	err := h.rs.failPrompt(t.Context(), "chat-1", "req-1", fmt.Errorf("spawn: %w", ErrPromptSessionLost))
	require.ErrorIs(t, err, ErrPromptSessionLost)
	assert.Equal(t, ledger.ReasonSessionLost, promptReceipt(t, h.rs.ledger, "req-1").Reason)

	require.NoError(t, h.rs.recordPrompt(t.Context(), "chat-1", "req-2", "again", "", ledger.DeliveryRestart, "runner-2", nil))
	require.ErrorIs(t, h.rs.failPrompt(t.Context(), "chat-1", "req-2", ErrPromptOutcomeUnknown), ErrPromptOutcomeUnknown)
	assert.Equal(t, ledger.ReasonDeliveryFailed, promptReceipt(t, h.rs.ledger, "req-2").Reason)
}

// The refusal must SURVIVE for the case it was written for: a replacement
// spawn that produced neither a PTY nor a connection has no delivery target
// at all, and reporting success for it is how a prompt goes silently nowhere.
// The prompt is settled, so it holds the chat busy for nobody.
func TestCommitPromptSpawn_ARunnerWithNeitherPTYNorConnectionIsStillRefused(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	require.NoError(t, h.rs.live.add(engineagents.Runner{ID: "runner-1", ProviderID: "codex", CurrentChatID: "chat-1"}))
	require.NoError(t, h.rs.recordPrompt(t.Context(), "chat-1", "req-1", "hello", "", ledger.DeliveryRestart, "runner-1", nil))

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

// A prompt whose carrier is still being launched by its own submission is not
// orphaned: a second send in that window waits behind it, never displaces it.
func TestReplayPriorPrompt_APromptWhoseCarrierIsStillLaunchingKeepsTheChatBusy(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	h.rs.live.launching.Store("runner-launching", true)
	appendPrompt(t, h.rs.ledger, "req-1", "runner-launching")

	_, done, err := h.rs.replayPriorPrompt(t.Context(), "chat-1", "req-2", "next")

	assert.True(t, done)
	require.ErrorIs(t, err, ErrPromptBusy)
	assert.Equal(t, ledger.PromptStatusPending, promptReceipt(t, h.rs.ledger, "req-1").Status,
		"the first prompt is still owed its delivery")
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
	l, err := ledgerstore.Open(tb.Context(), db, time.Now)
	require.NoError(tb, err)
	return l
}

// A spawn refused for its lost session says so; a spawn that may have forked
// still cannot tell whether the provider took the prompt.
func TestSpawnFailure_ALostSessionIsSessionLost(t *testing.T) {
	refused := fmt.Errorf("%w: %w", ErrSpawnSessionRefused, engineagents.ErrAPISessionLost)

	err := spawnFailure(t.Context(), refused)

	require.ErrorIs(t, err, ErrPromptSessionLost)
	assert.Equal(t, problem.SessionLost, problem.CodeOf(err))
	assert.Equal(t, http.StatusConflict, problem.CodeOf(err).Status())
	assert.ErrorIs(t, spawnFailure(t.Context(), errors.New("fork failed")), ErrPromptOutcomeUnknown)
}
