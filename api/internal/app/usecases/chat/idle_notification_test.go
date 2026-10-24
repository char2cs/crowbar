package chat_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// The two Notification messages claude actually sends, both verbatim. The same
// hook carries both, so only the idle one may read as claude saying it is idle.
const (
	idleNotice       = "Claude is waiting for your input"
	permissionNotice = "Claude needs your permission to use Bash"
)

// claudeTurnWithAToolOpen is a claude turn running a tool call that never
// reports its end.
func claudeTurnWithAToolOpen(t *testing.T, f testFixture) (chatID, runnerID string) {
	t.Helper()
	chatID, runnerID = f.spawn(t, "claude")
	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	hook(t, f, runnerID, "claude", engineagents.HookToolPre, map[string]any{
		"tool_use_id": "toolu_01FeKyxrB8sfmuTiHZYUj2VA",
		"tool_name":   "ScheduleWakeup",
		"tool_input":  map[string]any{},
	})
	require.True(t, f.busy(t, chatID), "precondition: the turn is running")
	return chatID, runnerID
}

// claude's Notification hook is its authoritative "I am waiting on the human":
// it ends the running turn, however much of it never reported an end.
func TestRegression_ClaudesIdleNotificationEndsItsTurn(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := claudeTurnWithAToolOpen(t, f)

	// claude.yaml wires two relay commands to the one Notification hook.
	notice := map[string]any{"message": idleNotice}
	hook(t, f, runnerID, "claude", engineagents.HookNotification, notice)
	hook(t, f, runnerID, "claude", engineagents.HookIdle, notice)

	assert.False(t, f.busy(t, chatID), "claude said it is waiting for the user")
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "next", uuid.NewString(), "", nil)
	assert.NotErrorIs(t, err, agentusecase.ErrPromptBusy)
}

// A permission notice is claude waiting on a decision inside a live turn: it
// ends nothing, and is still recorded as the interruption it is.
func TestRegression_APermissionNotificationNeverEndsClaudesTurn(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := claudeTurnWithAToolOpen(t, f)

	notice := map[string]any{"message": permissionNotice}
	hook(t, f, runnerID, "claude", engineagents.HookNotification, notice)
	hook(t, f, runnerID, "claude", engineagents.HookIdle, notice)

	assert.True(t, f.busy(t, chatID), "the turn waiting on the user's decision is still running")
	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1)
	assert.Equal(t, ledger.MarkerNotification, ints[0].Kind)
	assert.Equal(t, permissionNotice, ints[0].Detail)
}

// TestRegression_TheIdleNoticeIsStillRecordedAsAnInterruption pins the other
// side of "one hook, two canonical events": adding the idle reading must not
// cost the interruption row the activity ledger already showed for it.
func TestRegression_TheIdleNoticeIsStillRecordedAsAnInterruption(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	notice := map[string]any{"message": idleNotice}
	hook(t, f, runnerID, "claude", engineagents.HookNotification, notice)
	hook(t, f, runnerID, "claude", engineagents.HookIdle, notice)

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1, "the idle reading must not duplicate or replace the notification row")
	assert.Equal(t, ledger.MarkerNotification, ints[0].Kind)
	assert.Equal(t, idleNotice, ints[0].Detail)
}
