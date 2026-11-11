package ingress

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

const usageLimitScreen = "■ You've hit your usage limit. Upgrade to Pro, visit\nthe settings page to purchase more credits.\n"

var usageLimit = engineagents.TerminalNotice{Kind: engineagents.TerminalNoticeUsageLimit, Text: "You've hit your usage limit", EndsTurn: true}

func TestRecorder_ANoticeFailsTheTurnItsRunnerRuns(t *testing.T) {
	f := newRecorderFixture(t, "codex")
	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"})
	require.NoError(t, err)
	_, _, err = f.dispatch(t, delta(engineagents.HookMessageDelta, "m1", false, false, "partly"))
	require.NoError(t, err)

	f.r.FailTurnOnNotice(t.Context(), f.runner, usageLimit)

	rows := f.rows(t)
	require.Equal(t, []ledger.Kind{ledger.KindAssistantText, ledger.KindTurnFailed}, kinds(rows[len(rows)-2:]))
	assert.Equal(t, "partly", bodyOf[ledger.AssistantText](t, rows[len(rows)-2]).Text, "what it streamed is kept")
	assert.Equal(t, usageLimit.Kind, bodyOf[ledger.TurnFailed](t, rows[len(rows)-1]).Reason)
	exit, err := f.r.ExitRows(f.state(t), testChat, "run-1")
	require.NoError(t, err)
	assert.Empty(t, exit)
}

func TestRecorder_ANoticeEndsNothingNotItsRunners(t *testing.T) {
	f := newRecorderFixture(t, "codex")
	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"})
	require.NoError(t, err)
	before := len(f.rows(t))

	other := f.runner
	other.ID = "run-2"
	f.r.FailTurnOnNotice(t.Context(), other, usageLimit)
	gone := f.runner
	gone.CurrentChatID = "chat-deleted"
	f.r.FailTurnOnNotice(t.Context(), gone, usageLimit)

	assert.Len(t, f.rows(t), before)
	_, running := f.state(t).RunningTurn()
	assert.True(t, running)
}

func TestRecorder_ANoticeWithNothingRunningSettlesTheCarriedPrompts(t *testing.T) {
	f := newRecorderFixture(t, "codex")
	_, _, err := f.ledger.Append(t.Context(), testChat, crowbarPrompt(t, "req-1", "hello", ledger.DeliveryRestart))
	require.NoError(t, err)

	f.r.FailTurnOnNotice(t.Context(), f.runner, usageLimit)

	assert.Empty(t, f.state(t).PendingPrompts())
}

func TestRecorder_AScreenIsMatchedAgainstItsProvidersDescriptor(t *testing.T) {
	f := newRecorderFixture(t, "codex")

	notice, ok := f.r.MatchTerminalNotice(t.Context(), "codex", usageLimitScreen)
	require.True(t, ok)
	assert.Equal(t, engineagents.TerminalNoticeUsageLimit, notice.Kind)
	_, ok = f.r.MatchTerminalNotice(t.Context(), "claude", usageLimitScreen)
	assert.False(t, ok, "a descriptor declaring no notice matches none")

	prompt, ok := f.r.MatchTerminalPrompt(t.Context(), "claude", "Do you trust the files here?\n❯ 1. Yes, I trust this folder\n")
	require.True(t, ok)
	assert.Equal(t, "workspace_trust", prompt.Kind)

	_, ok = f.r.MatchTerminalNotice(t.Context(), "no-such-provider", usageLimitScreen)
	assert.False(t, ok)
	_, ok = f.r.MatchTerminalPrompt(t.Context(), "no-such-provider", "x")
	assert.False(t, ok)
	f.r.home = func() (string, error) { return "", errors.New("no home") }
	_, ok = f.r.MatchTerminalPrompt(t.Context(), "claude", "I trust this folder")
	assert.False(t, ok)
}
