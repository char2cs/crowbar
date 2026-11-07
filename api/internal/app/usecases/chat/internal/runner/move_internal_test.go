package runner

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func doneTo(dir string) MoveApplier {
	return func(_ context.Context, m ledger.Move) (ledger.Move, error) {
		return ledger.Move{From: "ws-a", To: m.To, FromDir: "/old", ToDir: dir}, nil
	}
}

func TestExecuteMove_AnIdleChatMovesAtOnceAndItsNextPromptCarriesTheNote(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	ready, err := h.rs.RequestMove(t.Context(), "chat-1", ledger.Move{RequestID: "m1", To: "ws-b", Label: "b2"})
	require.NoError(t, err)
	require.True(t, ready)

	_, err = h.rs.recordPrompt(t.Context(), "chat-1", "req-0", "hi", "", ledger.DeliveryRestart, "runner-1", nil)
	require.ErrorIs(t, err, ErrChatMoving, "a prompt never starts a CLI in the folder the chat is leaving")

	done, ran, err := h.rs.ExecuteMove(t.Context(), "chat-1", doneTo("/new"))
	require.NoError(t, err)
	require.True(t, ran)
	assert.Equal(t, ledger.MoveDone, done.Phase)
	assert.Equal(t, "b2", done.Label)

	note, err := h.rs.recordPrompt(t.Context(), "chat-1", "req-1", "where are you?", "", ledger.DeliveryRestart, "runner-1", nil)
	require.NoError(t, err)
	assert.Contains(t, note, "/new")
	p := promptReceipt(t, h.rs.ledger, "req-1")
	assert.Equal(t, "where are you?", p.Text, "the person's words are kept as they typed them")
	assert.Empty(t, p.Dispatch, "the note is the launch's, not the person's prompt")
}

// The CLI echoes the note with the prompt; the echo is the person's words, never
// a second turn showing Crowbar's note as typed by them.
func TestSpawnRunner_TheMoveNotesEchoIsCrowbarsNotAPrompt(t *testing.T) {
	h := newSpawnHarness(t, "")
	note := "<system-reminder>moved</system-reminder>\n\n"
	_, err := h.rs.spawnRunner(context.Background(), h.chatID, "ws-1", "claude",
		spawnInput{runnerID: "runner-1", promptMessage: "hello", moveNote: note})
	require.NoError(t, err)
	<-h.lifecycle.started

	rest, injected := h.rs.agents.ConsumeInjectedPrefix("runner-1", note+"hello")
	assert.True(t, injected)
	assert.Equal(t, "hello", rest)
}

func TestExecuteMove_WaitsForTheRunningTurn(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	_, err := h.rs.recordPrompt(t.Context(), "chat-1", "req-1", "work", "", ledger.DeliveryRestart, "runner-1", nil)
	require.NoError(t, err)
	started, err := ledger.NewRow(ledger.KindTurnStarted, promptReceipt(t, h.rs.ledger, "req-1").TurnID, "",
		ledger.TurnStarted{RequestID: "req-1", RunnerID: "runner-1"})
	require.NoError(t, err)
	_, _, err = h.rs.ledger.Append(t.Context(), "chat-1", started)
	require.NoError(t, err)

	ready, err := h.rs.RequestMove(t.Context(), "chat-1", ledger.Move{RequestID: "m1", To: "ws-b"})
	require.NoError(t, err)
	assert.False(t, ready)
	_, ran, err := h.rs.ExecuteMove(t.Context(), "chat-1", doneTo("/new"))
	require.NoError(t, err)
	assert.False(t, ran, "a CLI is never moved mid-turn")
}

func TestExecuteMove_AFailureIsRecordedWithItsReason(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	_, err := h.rs.RequestMove(t.Context(), "chat-1", ledger.Move{RequestID: "m1", To: "ws-b"})
	require.NoError(t, err)
	done, ran, err := h.rs.ExecuteMove(t.Context(), "chat-1", func(context.Context, ledger.Move) (ledger.Move, error) {
		return ledger.Move{}, errors.New("the worktree could not be made")
	})
	require.Error(t, err)
	require.True(t, ran)
	assert.Equal(t, ledger.MoveFailed, done.Phase)
	assert.Contains(t, done.Reason, "the worktree could not be made")
	assert.Equal(t, "ws-b", done.To, "a failed row says where it was going, for its Retry")
	st, err := h.rs.ledger.State(t.Context(), "chat-1")
	require.NoError(t, err)
	_, pending := st.PendingMove()
	assert.False(t, pending)
}

func TestCancelMove_ClosesThePendingMove(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	_, err := h.rs.RequestMove(t.Context(), "chat-1", ledger.Move{RequestID: "m1", To: "ws-b", Label: "b2"})
	require.NoError(t, err)
	require.NoError(t, h.rs.CancelMove(t.Context(), "chat-1", "cancelled"))
	rows, err := h.rs.ledger.Read(t.Context(), "chat-1", 0, 0)
	require.NoError(t, err)
	cancelled, err := ledger.DecodeBody[ledger.Move](rows[len(rows)-1])
	require.NoError(t, err)
	assert.Equal(t, ledger.Move{Phase: ledger.MoveCancelled, RequestID: "m1", To: "ws-b", Label: "b2", Reason: "cancelled"},
		cancelled, "a cancelled row names the move it withdrew")
	_, ran, err := h.rs.ExecuteMove(t.Context(), "chat-1", doneTo("/new"))
	require.NoError(t, err)
	assert.False(t, ran)
}
