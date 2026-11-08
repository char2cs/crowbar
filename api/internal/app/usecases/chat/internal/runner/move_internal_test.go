package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
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

// The agent asking twice in a turn asks once; a thread that already owns its branch is refused,
// so an agent cannot branch again and again.
func TestRequestBranchOut_ARepeatIsTheSameAskAndAnOwnerIsRefused(t *testing.T) {
	h := newSpawnHarness(t, engineagents.SurfaceChat)
	_, err := h.rs.RequestBranchOut(t.Context(), "chat-1", "ws-1", "fix-login", "agent")
	require.NoError(t, err)
	_, err = h.rs.RequestBranchOut(t.Context(), "chat-1", "ws-1", "other", "agent")
	require.NoError(t, err)
	st, err := h.rs.ledger.State(t.Context(), "chat-1")
	require.NoError(t, err)
	asked, _ := st.PendingMove()
	assert.Equal(t, "fix-login", asked.Branch)
	require.NoError(t, h.rs.CancelMove(t.Context(), "chat-1", "cancelled"))

	h.rs.chats = stubChatsForSpawn{chat: domain.Chat{ID: "chat-1", Surface: engineagents.SurfaceChat, OwnsWorkspace: true}}
	_, err = h.rs.RequestBranchOut(t.Context(), "chat-1", "ws-1", "again", "agent")
	require.ErrorIs(t, err, ErrOwnsItsBranch)
	st, err = h.rs.ledger.State(t.Context(), "chat-1")
	require.NoError(t, err)
	_, pending := st.PendingMove()
	assert.False(t, pending)
}

// A level whose sandbox the chat's old folder is inside cannot keep the chat out
// of it: once the chat has left a folder, it launches as its narrower values.
func TestSpawnRunner_AChatThatLeftAFolderLaunchesItsLevelsValuesAfterLeave(t *testing.T) {
	for name, tc := range map[string]struct {
		moved  bool
		want   string
		notice bool
	}{
		"never moved": {want: "thread/resume open"},
		"moved":       {moved: true, want: "thread/resume kept", notice: true},
	} {
		t.Run(name, func(t *testing.T) {
			h := helperServeSpawn(t, "answer", false)
			installDescriptor(t, h.home, strings.Replace(
				strings.Replace(fmt.Sprintf(helperServeDescriptor, os.Args[0]), `send: { threadId: "{session_id}" }`,
					`send: { threadId: "{session_id}", text: "{permission.sandbox}" }`, 1),
				"runtime:\n", leavePermissionLevels+"runtime:\n", 1))
			h.rs.conversations = selectionConversations{sel: engineagents.Selection{PermissionLevel: "full-auto"}}
			asked := filepath.Join(t.TempDir(), "asked")
			t.Setenv(helperServeAsked, asked)
			if tc.moved {
				_, err := h.rs.RequestMove(t.Context(), h.chatID, ledger.Move{RequestID: "m1", To: "ws-b"})
				require.NoError(t, err)
				_, _, err = h.rs.ExecuteMove(t.Context(), h.chatID, doneTo("/ws/branch"))
				require.NoError(t, err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()

			_, err := h.rs.spawnRunner(ctx, h.chatID, "ws-1", "carrier-test",
				spawnInput{runnerID: "runner-1", promptMessage: "hi", moveNote: "moved\n\n"})
			require.NoError(t, err)

			assert.Contains(t, askedCalls(t, asked), tc.want)
			warned := slices.ContainsFunc(dividers(t, h.rs.ledger, h.chatID), func(d string) bool {
				return strings.Contains(d, "writes only inside its new folder")
			})
			assert.Equal(t, tc.notice, warned, "the chat is told its level runs narrower")
		})
	}
}

const leavePermissionLevels = `permission_levels:
  strategy: restart_tui
  levels:
    full-auto:
      vars: { sandbox: open }
      vars_after_leave: { sandbox: kept }
`
