package chat_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// The two Notification messages claude actually sends, both verbatim.
//
// idleNotice is a MEASUREMENT, not a guess: it is the value recorded six times
// in this worktree's own event store as an interruption's `detail`
// (kind: "notification"), and inbound.build sets Interrupt.Detail =
// ev.Message while claude.yaml's notification: maps `message: message`, so the
// string is verbatim provider payload with nothing of Crowbar's in it. On the
// wedged chat it landed 60.0s after the turn close — claude's own idle-
// notification delay.
//
// permissionNotice is the other thing the SAME hook carries, and the whole
// reason a discriminator is mandatory: arming idle on any Notification would
// hand AbandonMessage a genuinely live turn five seconds later.
const (
	idleNotice       = "Claude is waiting for your input"
	permissionNotice = "Claude needs your permission to use Bash"
)

// wedgeAClaudeTurn reproduces, hook for hook, the state measured on claude chat
// 7745f69f (2026-09-23):
//
//	22:50:07.533  turn_started                 working = true
//	22:50:17.302  turn_stopped                 working = false — Stop DID arrive
//	22:50:19.691  PreToolUse "ScheduleWakeup"  2.4s after the close, no turn open
//	              -> ingest mints a synthetic turn, restateAsyncWork relights
//	                 working = TRUE
//	(no PostToolUse, ever — claude fires none on that path)
//
// Nothing could recover it: providerSaysItIsIdle needed an `idle` event claude
// declared not_emitted, abandonedMessage is gated on OpenWork which that one
// running row holds true forever, and stalled needs a terminal_notices entry
// claude declares none of.
func wedgeAClaudeTurn(t *testing.T, f testFixture) (chatID, runnerID string) {
	t.Helper()
	chatID, runnerID = f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	hook(t, f, runnerID, "claude", engineagents.HookTurnStop, map[string]any{
		"last_assistant_message": "done",
		"effort":                 map[string]any{"level": "high"},
	})
	hook(t, f, runnerID, "claude", engineagents.HookToolPre, map[string]any{
		"tool_use_id": "toolu_01FeKyxrB8sfmuTiHZYUj2VA",
		"tool_name":   "ScheduleWakeup",
		"tool_input":  map[string]any{},
	})

	require.True(t, f.chat(t, chatID).Working,
		"precondition: the measured wedge — a tool call opened after the turn "+
			"closed has relit the spinner")
	open, err := f.usecase.OpenWork(f.ctx, chatID)
	require.NoError(t, err)
	require.True(t, open,
		"precondition: the tool row is still open, which is what holds every "+
			"OpenWork-gated recovery off")
	_, armed := agentusecase.ProviderIdleLatch(f.usecase.TurnUsecase, chatID)
	require.False(t, armed, "precondition: nothing has reported idle yet")

	return chatID, runnerID
}

// TestRegression_AWedgedClaudeTurnIsClearedByClaudesOwnIdleNotification is the
// fix for the showstopper the user hit twice: a claude chat spinning forever
// after the turn visibly finished, composer reading "Queue a message…" with a
// Stop button, the CLI's own TUI showing the turn done.
//
// claude DOES send an authoritative "I am waiting on the human" — the
// Notification hook, 60s after the close. It was only ever recorded as an
// interruption row; nothing consumed it as an idle report, because a wire hook
// could mean exactly one canonical event. It now ALSO means `idle`, gated on
// the message itself, which arms the latch the terminal-wait sweep already
// knows how to act on.
func TestRegression_AWedgedClaudeTurnIsClearedByClaudesOwnIdleNotification(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := wedgeAClaudeTurn(t, f)

	// 60s after the close. The SAME payload reaches Crowbar twice, under two
	// canonical names, because claude.yaml wires two relay commands to the one
	// Notification hook.
	notice := map[string]any{"message": idleNotice}
	hook(t, f, runnerID, "claude", engineagents.HookNotification, notice)
	hook(t, f, runnerID, "claude", engineagents.HookIdle, notice)

	at, armed := agentusecase.ProviderIdleLatch(f.usecase.TurnUsecase, chatID)
	require.True(t, armed,
		"claude said it is waiting for the user; that is the idle report the "+
			"provider-idle fuse needs")
	assert.False(t, at.IsZero())

	// ...and the close that fuse performs actually clears the wedge. Not a
	// second timer: the fuse's own 5s wait is driven off a fake clock by
	// termwait's TestProviderIdle_ClosesATurnWhoseCloseNeverCame; what is
	// proven here is that running it against THIS state — a synthetic turn with
	// asyncWork=1 and a tool row that will never close — puts the chat back to
	// rest.
	closed, err := agentusecase.CloseIdleTurn(f.ctx, f.usecase.TurnUsecase, chatID)
	require.NoError(t, err)
	require.True(t, closed)
	f.wait()

	assert.False(t, f.chat(t, chatID).Working,
		"the chat must stop reporting working, ~65s after the close and not in two hours")
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "next", uuid.NewString(), "", nil)
	assert.NotErrorIs(t, err, agentusecase.ErrPromptBusy,
		"a closed turn must not stay in flight: the next message is refused as busy forever")
}

// TestRegression_APermissionNotificationNeverArmsClaudesIdleLatch is the safety
// half, and it is the whole argument for the discriminator living in the
// descriptor rather than nowhere.
//
// Notification is not only claude's idle ping — it is also how claude announces
// that it needs permission. Arming the latch on ANY notification would hand
// AbandonMessage a genuinely live turn five seconds later: the chat would go
// dark, the partial answer would be salvaged as though the turn had ended, and
// the user's permission prompt would be answered into a closed turn. So a
// permission notice must arm nothing at all, while still being RECORDED as the
// interruption it is.
func TestRegression_APermissionNotificationNeverArmsClaudesIdleLatch(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := wedgeAClaudeTurn(t, f)

	notice := map[string]any{"message": permissionNotice}
	hook(t, f, runnerID, "claude", engineagents.HookNotification, notice)
	hook(t, f, runnerID, "claude", engineagents.HookIdle, notice)

	_, armed := agentusecase.ProviderIdleLatch(f.usecase.TurnUsecase, chatID)
	assert.False(t, armed,
		"a permission notice is claude waiting on a DECISION, not claude idle — "+
			"arming here would kill a live turn 5s later")

	// The interruption is still recorded: the discriminator selects one variant
	// IN, it never filters the other out.
	ints, err := f.usecase.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1)
	assert.Equal(t, engineagents.InterruptNotification, ints[0].Kind)
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

	ints, err := f.usecase.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1, "the idle reading must not duplicate or replace the notification row")
	assert.Equal(t, engineagents.InterruptNotification, ints[0].Kind)
	assert.Equal(t, idleNotice, ints[0].Detail)
}
