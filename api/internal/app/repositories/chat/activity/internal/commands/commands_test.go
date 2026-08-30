package commands_test

import (
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var now = time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)

const chat = "chat-1"

func TestAppendTurn_EmitsACompletedTurnAsTheDelta(t *testing.T) {
	c := commands.AppendTurn{
		ChatID: chat, TurnID: "t1", Role: domain.TurnRoleUser,
		ProviderID: "claude", RunnerID: "r1", SessionID: "s1", Text: "hello", Now: now,
	}
	require.NoError(t, c.Validate(nil))

	got := c.EmitEvent(nil)

	require.NotNil(t, got.Last)
	assert.Equal(t, domain.DeltaClose, got.Last.Phase)
	assert.Equal(t, domain.DeltaTurn, got.Last.Kind)
	require.NotNil(t, got.Last.Turn)
	assert.Equal(t, "hello", got.Last.Turn.Text)
	assert.Equal(t, domain.TurnRoleUser, got.Last.Turn.Role)
	require.NotNil(t, got.Last.Turn.EndedAt)
	assert.Equal(t, int64(1), got.Seq)
	assert.Nil(t, got.Turn, "a completed turn leaves nothing open")
}

func TestAppendTurn_RejectsTheUnusableCases(t *testing.T) {
	testCases := []struct {
		name string
		cmd  commands.AppendTurn
	}{
		{"no chat", commands.AppendTurn{TurnID: "t", Role: domain.TurnRoleUser}},
		{"no turn id", commands.AppendTurn{ChatID: chat, Role: domain.TurnRoleUser}},
		{"unknown role", commands.AppendTurn{ChatID: chat, TurnID: "t", Role: "narrator"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.ErrorIs(t, tc.cmd.Validate(nil), asynxModels.ErrValidation)
		})
	}
}

func TestOpenTurn_StartsTheTurnToolsAttachTo(t *testing.T) {
	c := commands.OpenTurn{ChatID: chat, TurnID: "t1", ProviderID: "claude", Now: now}
	require.NoError(t, c.Validate(nil))

	got := c.EmitEvent(nil)

	require.NotNil(t, got.Turn)
	assert.Equal(t, "t1", got.Turn.ID)
	assert.Equal(t, domain.TurnRoleAssistant, got.Turn.Role)
	require.NotNil(t, got.Last)
	assert.Equal(t, domain.DeltaOpen, got.Last.Phase)
}

func TestOpenTurn_SupersedesWhateverTheLastTurnLeftInFlight(t *testing.T) {
	prior := commands.InvokeTool{ChatID: chat, ToolID: "tool-1", Name: "Bash", Now: now}.
		EmitEvent(nil)
	require.NotEmpty(t, prior.Tools)

	got := commands.OpenTurn{ChatID: chat, TurnID: "t2", Now: now}.EmitEvent(&prior)

	assert.Empty(t, got.Tools)
	assert.Empty(t, got.Subagents)
	assert.Empty(t, got.Interruptions)
	assert.Equal(t, "t2", got.Turn.ID)
}

func TestCloseTurn_CompletesTheOpenTurnWithItsText(t *testing.T) {
	opened := commands.OpenTurn{ChatID: chat, TurnID: "t1", ProviderID: "claude", Now: now}.
		EmitEvent(nil)

	c := commands.CloseTurn{ChatID: chat, TurnID: "t1", Text: "done", Effort: "high", Now: now}
	require.NoError(t, c.Validate(&opened))
	got := c.EmitEvent(&opened)

	assert.Nil(t, got.Turn)
	require.NotNil(t, got.Last)
	assert.Equal(t, domain.DeltaClose, got.Last.Phase)
	assert.Equal(t, "done", got.Last.Turn.Text)
	assert.Equal(t, "high", got.Last.Turn.Effort)
	assert.Equal(t, "claude", got.Last.Turn.ProviderID, "the opened turn's attribution survives")
	require.NotNil(t, got.Last.Turn.EndedAt)
}

func TestCloseTurn_WithNoOpenTurnStillRecordsTheReply(t *testing.T) {
	c := commands.CloseTurn{ChatID: chat, TurnID: "t9", ProviderID: "codex", Text: "late", Now: now}

	require.NoError(t, c.Validate(nil))
	got := c.EmitEvent(nil)

	require.NotNil(t, got.Last)
	assert.Equal(t, "late", got.Last.Turn.Text)
	assert.Equal(t, "codex", got.Last.Turn.ProviderID)
}

func TestInvokeTool_RecordsTheCallAndHoldsItOpen(t *testing.T) {
	opened := commands.OpenTurn{ChatID: chat, TurnID: "t1", Now: now}.EmitEvent(nil)

	c := commands.InvokeTool{
		ChatID: chat, ToolID: "tool-1", Name: "Edit", Target: "a.go",
		RequestRef: "sha256:abc", Now: now,
	}
	require.NoError(t, c.Validate(&opened))
	got := c.EmitEvent(&opened)

	require.Contains(t, got.Tools, "tool-1")
	assert.Equal(t, domain.ToolStatusRunning, got.Tools["tool-1"].Status)
	assert.Equal(t, "t1", got.Tools["tool-1"].TurnID)
	require.NotNil(t, got.Last.Tool)
	assert.Equal(t, "sha256:abc", got.Last.Tool.RequestRef)
	assert.Equal(t, domain.DeltaOpen, got.Last.Phase)
}

func TestInvokeTool_WithNoOpenTurnOpensOneImplicitly(t *testing.T) {
	got := commands.InvokeTool{ChatID: chat, ToolID: "tool-1", Name: "Bash", Now: now}.EmitEvent(nil)

	require.NotNil(t, got.Turn)
	assert.Equal(t, domain.TurnRoleAssistant, got.Turn.Role)
	assert.Equal(t, got.Turn.ID, got.Tools["tool-1"].TurnID)
}

func TestCompleteTool_ClosesTheCallAndCarriesTheInvocationForward(t *testing.T) {
	invoked := commands.InvokeTool{
		ChatID: chat, ToolID: "tool-1", Name: "Edit", Target: "a.go",
		RequestRef: "sha256:req", Now: now,
	}.EmitEvent(nil)

	c := commands.CompleteTool{
		ChatID: chat, ToolID: "tool-1", ResultRef: "sha256:res",
		Status: domain.ToolStatusOK, DurationMS: 120, Now: now.Add(time.Second),
	}
	require.NoError(t, c.Validate(&invoked))
	got := c.EmitEvent(&invoked)

	assert.NotContains(t, got.Tools, "tool-1", "a completed call is no longer in flight")
	require.NotNil(t, got.Last.Tool)
	assert.Equal(t, "Edit", got.Last.Tool.Name, "the invocation's name survives the completion")
	assert.Equal(t, "a.go", got.Last.Tool.Target)
	assert.Equal(t, "sha256:req", got.Last.Tool.RequestRef)
	assert.Equal(t, "sha256:res", got.Last.Tool.ResultRef)
	assert.Equal(t, 120, got.Last.Tool.DurationMS)
	require.NotNil(t, got.Last.Tool.EndedAt)
}

func TestCompleteTool_DoesNotOverwriteWithWhatItDidNotReport(t *testing.T) {
	invoked := commands.InvokeTool{ChatID: chat, ToolID: "t", Name: "Edit", Target: "a.go", Now: now}.
		EmitEvent(nil)

	got := commands.CompleteTool{ChatID: chat, ToolID: "t", Now: now}.EmitEvent(&invoked)

	assert.Equal(t, "Edit", got.Last.Tool.Name)
	assert.Equal(t, "a.go", got.Last.Tool.Target)
	assert.Equal(t, domain.ToolStatusOK, got.Last.Tool.Status, "an unreported status defaults to ok")
}

func TestCompleteTool_FallsBackToObservedDurationOnlyWhenAStartWasSeen(t *testing.T) {
	invoked := commands.InvokeTool{ChatID: chat, ToolID: "t", Now: now}.EmitEvent(nil)

	seen := commands.CompleteTool{ChatID: chat, ToolID: "t", Now: now.Add(250 * time.Millisecond)}.
		EmitEvent(&invoked)
	assert.Equal(t, 250, seen.Last.Tool.DurationMS)

	unseen := commands.CompleteTool{ChatID: chat, ToolID: "ghost", Now: now}.EmitEvent(nil)
	assert.Zero(t, unseen.Last.Tool.DurationMS,
		"a duration Crowbar never observed must not be invented")
}

func TestCompleteTool_ForAnUnseenCallStillLeavesALegibleRecord(t *testing.T) {
	got := commands.CompleteTool{
		ChatID: chat, ToolID: "t", Name: "Read", Status: domain.ToolStatusError, Now: now,
	}.EmitEvent(nil)

	require.NotNil(t, got.Last.Tool)
	assert.Equal(t, "Read", got.Last.Tool.Name)
	assert.Equal(t, domain.ToolStatusError, got.Last.Tool.Status)
}

func TestSubagent_StopWithoutAStartIsRecordedOnItsOwnTerms(t *testing.T) {
	got := commands.StopSubagent{ChatID: chat, SubagentID: "a1", AgentType: "explore", Now: now}.
		EmitEvent(nil)

	require.NotNil(t, got.Last.Subagent)
	assert.Equal(t, "a1", got.Last.Subagent.ID)
	assert.Equal(t, "explore", got.Last.Subagent.AgentType)
	require.NotNil(t, got.Last.Subagent.EndedAt)
}

func TestSubagent_StartThenStopClosesTheSameRecord(t *testing.T) {
	started := commands.StartSubagent{ChatID: chat, SubagentID: "a1", AgentType: "explore", Now: now}.
		EmitEvent(nil)
	require.Contains(t, started.Subagents, "a1")

	stopped := commands.StopSubagent{ChatID: chat, SubagentID: "a1", Now: now.Add(time.Second)}.
		EmitEvent(&started)

	assert.NotContains(t, stopped.Subagents, "a1")
	assert.Equal(t, "explore", stopped.Last.Subagent.AgentType)
}

func TestInterrupt_OpensAndResolvesTheSameRecord(t *testing.T) {
	turn := commands.OpenTurn{ChatID: chat, TurnID: "t1", Now: now}.EmitEvent(nil)
	opened := commands.Interrupt{
		ChatID: chat, ID: "i1", Kind: "compaction", Detail: "auto", Now: now,
	}.EmitEvent(&turn)
	require.Contains(t, opened.Interruptions, "i1")
	assert.Equal(t, domain.DeltaOpen, opened.Last.Phase)

	resolved := commands.ResolveInterruption{ChatID: chat, ID: "i1", Now: now.Add(time.Second)}.
		EmitEvent(&opened)

	assert.NotContains(t, resolved.Interruptions, "i1")
	require.NotNil(t, resolved.Last.Interruption.ResolvedAt)
	assert.Equal(t, "auto", resolved.Last.Interruption.Detail)
}

func TestInterrupt_OutsideATurnIsRecordedAlreadyResolved(t *testing.T) {
	got := commands.Interrupt{
		ChatID: chat, ID: "i1", Kind: "notification",
		Detail: "Claude is waiting for your input", Now: now,
	}.EmitEvent(nil)

	assert.Empty(t, got.Interruptions, "nothing is blocked")
	assert.Nil(t, got.Turn, "and no reply was started by a notification")
	require.NotNil(t, got.Last.Interruption)
	assert.Equal(t, domain.DeltaClose, got.Last.Phase)
	assert.NotNil(t, got.Last.Interruption.ResolvedAt)
}

func TestInterrupt_ValidationRequiresIdentityAndKind(t *testing.T) {
	assert.ErrorIs(t, commands.Interrupt{ChatID: chat, ID: "i"}.Validate(nil), asynxModels.ErrValidation)
	assert.ErrorIs(t, commands.Interrupt{ChatID: chat, Kind: "k"}.Validate(nil), asynxModels.ErrValidation)
	assert.NoError(t, commands.Interrupt{ChatID: chat, ID: "i", Kind: "k"}.Validate(nil))
}

func TestResolveInterruption_ForAnUnseenIDStillRecords(t *testing.T) {
	got := commands.ResolveInterruption{
		ChatID: chat, ID: "i9", Kind: "compaction", Detail: "manual", Now: now,
	}.EmitEvent(nil)

	require.NotNil(t, got.Last.Interruption)
	assert.Equal(t, "manual", got.Last.Interruption.Detail)
	require.NotNil(t, got.Last.Interruption.ResolvedAt)
}

func TestAbandon_ClosesWhatWasOpenWithoutInventingAReply(t *testing.T) {
	opened := commands.OpenTurn{ChatID: chat, TurnID: "t1", ProviderID: "claude", Now: now}.
		EmitEvent(nil)
	withTool := commands.InvokeTool{ChatID: chat, ToolID: "tool-1", Now: now}.EmitEvent(&opened)

	got := commands.Abandon{ChatID: chat, Now: now.Add(time.Minute)}.EmitEvent(&withTool)

	assert.Nil(t, got.Turn)
	assert.Empty(t, got.Tools)
	require.NotNil(t, got.Last.Turn)
	assert.Empty(t, got.Last.Turn.Text, "an abandoned turn has no reply to record")
	require.NotNil(t, got.Last.Turn.EndedAt)
}

func TestAbandon_WithNothingOpenTouchesNothing(t *testing.T) {
	got := commands.Abandon{ChatID: chat, Now: now}.EmitEvent(nil)

	assert.Nil(t, got.Last, "the reconcile is still recorded, but it changed nothing")
	assert.Equal(t, int64(1), got.Seq)
}

func TestEmitEvent_NeverMutatesThePreviousAggregate(t *testing.T) {
	prior := commands.InvokeTool{ChatID: chat, ToolID: "keep", Now: now}.EmitEvent(nil)
	before := len(prior.Tools)

	_ = commands.InvokeTool{ChatID: chat, ToolID: "another", Now: now}.EmitEvent(&prior)
	_ = commands.CompleteTool{ChatID: chat, ToolID: "keep", Now: now}.EmitEvent(&prior)
	_ = commands.StartSubagent{ChatID: chat, SubagentID: "a", Now: now}.EmitEvent(&prior)
	_ = commands.Interrupt{ChatID: chat, ID: "i", Kind: "k", Now: now}.EmitEvent(&prior)

	assert.Len(t, prior.Tools, before)
	assert.Contains(t, prior.Tools, "keep")
	assert.Empty(t, prior.Subagents)
	assert.Empty(t, prior.Interruptions)
}

func TestOpenState_IsCappedSoALeakyProviderCannotGrowTheAggregate(t *testing.T) {
	state := commands.OpenTurn{ChatID: chat, TurnID: "t1", Now: now}.EmitEvent(nil)

	for i := range domain.MaxOpenPerTurn * 3 {
		next := commands.InvokeTool{
			ChatID: chat, ToolID: "tool-" + itoa(i), Name: "Bash", Now: now,
		}.EmitEvent(&state)
		state = next
	}

	assert.LessOrEqual(t, state.OpenCount(), domain.MaxOpenPerTurn)
	require.NotNil(t, state.Last, "every call is still REPORTED even past the cap")
}

func TestEveryCommand_NamesItsAggregateAndEvent(t *testing.T) {
	cmds := []asynxModels.Command[domain.ChatActivity]{
		commands.AppendTurn{ChatID: chat},
		commands.OpenTurn{ChatID: chat},
		commands.CloseTurn{ChatID: chat},
		commands.Abandon{ChatID: chat},
		commands.InvokeTool{ChatID: chat},
		commands.CompleteTool{ChatID: chat},
		commands.StartSubagent{ChatID: chat},
		commands.StopSubagent{ChatID: chat},
		commands.Interrupt{ChatID: chat},
		commands.ResolveInterruption{ChatID: chat},
	}
	seen := map[string]struct{}{}
	for _, c := range cmds {
		assert.Equal(t, chat, c.AggregateID())
		name := c.EventName()
		assert.Contains(t, name, "agentactivity.")
		assert.Contains(t, name, chat)
		_, dup := seen[name]
		assert.False(t, dup, "event names must be distinct: %s", name)
		seen[name] = struct{}{}
	}
}

func TestSnapshotPolicy_FollowsEventFrequency(t *testing.T) {
	boundaries := []asynxModels.Command[domain.ChatActivity]{
		commands.AppendTurn{}, commands.OpenTurn{}, commands.CloseTurn{}, commands.Abandon{},
	}
	for _, c := range boundaries {
		assert.True(t, c.ShouldSnapshot(), "%T", c)
	}
	hot := []asynxModels.Command[domain.ChatActivity]{
		commands.InvokeTool{},
		commands.CompleteTool{},
		commands.StartSubagent{},
		commands.StopSubagent{},
		commands.Interrupt{},
		commands.ResolveInterruption{},
	}
	for _, c := range hot {
		assert.False(t, c.ShouldSnapshot(), "%T", c)
	}
}

func TestValidate_EveryCommandRequiresAChat(t *testing.T) {
	assert.ErrorIs(t, commands.OpenTurn{TurnID: "t"}.Validate(nil), asynxModels.ErrValidation)
	assert.ErrorIs(t, commands.CloseTurn{TurnID: "t"}.Validate(nil), asynxModels.ErrValidation)
	assert.ErrorIs(t, commands.Abandon{}.Validate(nil), asynxModels.ErrValidation)
	assert.ErrorIs(t, commands.InvokeTool{ToolID: "t"}.Validate(nil), asynxModels.ErrValidation)
	assert.ErrorIs(t, commands.CompleteTool{ToolID: "t"}.Validate(nil), asynxModels.ErrValidation)
	assert.ErrorIs(t, commands.StartSubagent{SubagentID: "a"}.Validate(nil), asynxModels.ErrValidation)
	assert.ErrorIs(t, commands.StopSubagent{SubagentID: "a"}.Validate(nil), asynxModels.ErrValidation)
	assert.ErrorIs(t, commands.ResolveInterruption{ID: "i"}.Validate(nil), asynxModels.ErrValidation)

	assert.ErrorIs(t, commands.OpenTurn{ChatID: chat}.Validate(nil), asynxModels.ErrValidation)
	assert.ErrorIs(t, commands.CloseTurn{ChatID: chat}.Validate(nil), asynxModels.ErrValidation)
	assert.ErrorIs(t, commands.InvokeTool{ChatID: chat}.Validate(nil), asynxModels.ErrValidation)
	assert.ErrorIs(t, commands.CompleteTool{ChatID: chat}.Validate(nil), asynxModels.ErrValidation)
	assert.ErrorIs(t, commands.StartSubagent{ChatID: chat}.Validate(nil), asynxModels.ErrValidation)
	assert.ErrorIs(t, commands.StopSubagent{ChatID: chat}.Validate(nil), asynxModels.ErrValidation)
	assert.ErrorIs(t, commands.ResolveInterruption{ChatID: chat}.Validate(nil), asynxModels.ErrValidation)
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// Regression: reproduces the user's exact report. Claude's turn is
// dispatched first, interrupted (a graceful stop request, NOT a kill — see
// runner/lifecycle.go), and the user switches to Codex. Codex, dispatched
// SECOND, finishes FIRST. Claude, still gracefully finishing in the
// background, closes LATE — after Codex. Before this fix, CloseTurn minted
// a fresh Seq at close time for BOTH, so Claude's reply — recorded after
// Codex's — got a HIGHER value and sorted AFTER it despite being sent
// first. DisplayOrder must stay tied to DISPATCH order regardless of which
// one finishes first.
func TestRegression_AnInterruptedTurnThatFinishesLateStillDisplaysBeforeALaterTurnThatFinishedFirst(t *testing.T) {
	claudeOpened := commands.OpenTurn{
		ChatID: chat, TurnID: "open-claude", RunnerID: "claude-runner",
		ProviderID: "claude", Now: now,
	}.EmitEvent(nil)
	claudeDisplayOrder := claudeOpened.Turn.DisplayOrder

	// The user interrupts Claude and switches to Codex — a second runner
	// opens a turn on the SAME chat while Claude's is still technically
	// open. This replaces ChatActivity.Turn wholesale (the pre-existing,
	// separately-tracked single-pointer limitation), which is exactly why
	// DisplayOrder cannot be recovered from Turn positionally at close time.
	codexOpened := commands.OpenTurn{
		ChatID: chat, TurnID: "open-codex", RunnerID: "codex-runner",
		ProviderID: "codex", Now: now.Add(time.Second),
	}.EmitEvent(&claudeOpened)
	codexDisplayOrder := codexOpened.Turn.DisplayOrder
	require.Greater(t, codexDisplayOrder, claudeDisplayOrder,
		"dispatched second, so reserved a later order")

	// Codex, dispatched second, finishes FIRST.
	codexClosed := commands.CloseTurn{
		ChatID: chat, TurnID: "msg-codex-1", RunnerID: "codex-runner",
		Text: "codex reply", Now: now.Add(2 * time.Second),
	}.EmitEvent(&codexOpened)
	require.NotNil(t, codexClosed.Last.Turn)
	assert.Equal(t, codexDisplayOrder, codexClosed.Last.Turn.DisplayOrder,
		"inherits the order reserved at its own dispatch")

	// Claude, dispatched first, finishes LATE — after Codex already closed.
	claudeClosed := commands.CloseTurn{
		ChatID: chat, TurnID: "msg-claude-1", RunnerID: "claude-runner",
		Text: "claude reply", Now: now.Add(3 * time.Second),
	}.EmitEvent(&codexClosed)
	require.NotNil(t, claudeClosed.Last.Turn)

	assert.Equal(t, claudeDisplayOrder, claudeClosed.Last.Turn.DisplayOrder,
		"still recovers ITS OWN dispatch-time order despite closing last")
	assert.Less(t, claudeClosed.Last.Turn.DisplayOrder, codexClosed.Last.Turn.DisplayOrder,
		"THE FIX: displays before Codex, matching dispatch order")
	assert.Greater(t, claudeClosed.Last.Turn.Seq, codexClosed.Last.Turn.Seq,
		"while Seq — persist order — is, as ever, the opposite: proof the two are genuinely decoupled")
}

func TestOpenTurn_ReservesDisplayOrderPerRunner_NotClobberedByADifferentRunnerOpening(t *testing.T) {
	a := commands.OpenTurn{ChatID: chat, TurnID: "open-a", RunnerID: "runner-a", Now: now}.
		EmitEvent(nil)
	b := commands.OpenTurn{ChatID: chat, TurnID: "open-b", RunnerID: "runner-b", Now: now}.
		EmitEvent(&a)

	assert.Contains(t, b.OpenTurnOrders, "runner-a", "runner-a's reservation survives runner-b opening")
	assert.Contains(t, b.OpenTurnOrders, "runner-b")
	assert.Equal(t, a.Turn.DisplayOrder, b.OpenTurnOrders["runner-a"])
}

func TestCloseTurn_ConsumesAndRemovesItsReservation(t *testing.T) {
	opened := commands.OpenTurn{ChatID: chat, TurnID: "open-a", RunnerID: "runner-a", Now: now}.
		EmitEvent(nil)
	require.Contains(t, opened.OpenTurnOrders, "runner-a")

	closed := commands.CloseTurn{ChatID: chat, TurnID: "t1", RunnerID: "runner-a", Text: "done", Now: now}.
		EmitEvent(&opened)

	assert.NotContains(t, closed.OpenTurnOrders, "runner-a", "consumed, not left to leak forever")
}

// TestRegression_AnInterruptionRecordedLateStillDisplaysAtItsTurnsOwnPosition is
// ActivityInterruption's own version of the turn-DisplayOrder regression above:
// RecordStop (the "Interrupted" divider's source, turn.go) can now fire well
// after its turn opened — a provider switch that waits out a stuck outgoing
// turn's whole grace period before forcing it (runner/switch.go's
// forceOutgoingTurn) — and the stuck turn's OWN hooks (tool calls, other
// interruptions) keep advancing Seq for the entire wait. Without an inherited
// DisplayOrder, the eventual Interrupt would mint a fresh, much-later Seq and
// the divider would render after activity it logically preceded — the
// "interrupt signal moved away" symptom.
func TestRegression_AnInterruptionRecordedLateStillDisplaysAtItsTurnsOwnPosition(t *testing.T) {
	opened := commands.OpenTurn{
		ChatID: chat, TurnID: "open-claude", RunnerID: "claude-runner",
		ProviderID: "claude", Now: now,
	}.EmitEvent(nil)
	turnDisplayOrder := opened.Turn.DisplayOrder

	// The turn stays open and stuck, but its own activity keeps consuming Seq —
	// an unrelated permission prompt, opened and resolved, while nobody has
	// stopped anything yet.
	permissionOpened := commands.Interrupt{
		ChatID: chat, ID: "interrupt-permission", Kind: "permission", Now: now.Add(time.Second),
	}.EmitEvent(&opened)
	permissionResolved := commands.ResolveInterruption{
		ChatID: chat, ID: "interrupt-permission", Kind: "permission", Now: now.Add(2 * time.Second),
	}.EmitEvent(&permissionOpened)

	// Only NOW, well after Seq has moved on, does the switch's grace period
	// elapse and force the stuck turn — RecordStop's Interrupt+Resolve pair.
	stopOpened := commands.Interrupt{
		ChatID: chat, ID: "interrupt-stop", Kind: "stopped", Now: now.Add(3 * time.Second),
	}.EmitEvent(&permissionResolved)
	stopResolved := commands.ResolveInterruption{
		ChatID: chat, ID: "interrupt-stop", Kind: "stopped", Now: now.Add(3 * time.Second),
	}.EmitEvent(&stopOpened)

	require.NotNil(t, stopResolved.Last.Interruption)
	assert.Equal(t, turnDisplayOrder, stopResolved.Last.Interruption.DisplayOrder,
		"THE FIX: anchored to the turn it interrupted, not a fresh late Seq")
	assert.Greater(t, stopResolved.Last.Interruption.Seq, stopResolved.Last.Interruption.DisplayOrder,
		"while Seq — persist order — has moved on with everything that happened while it waited: "+
			"proof the two are genuinely decoupled")
}

func TestRegression_ACloseSideEventNeverConjuresATurn(t *testing.T) {
	opened := commands.OpenTurn{ChatID: chat, TurnID: "t1", Now: now}.EmitEvent(nil)
	closed := commands.CloseTurn{ChatID: chat, TurnID: "reply", Text: "done", Now: now}.
		EmitEvent(&opened)
	require.Nil(t, closed.Turn)

	testCases := []struct {
		name string
		emit func(*domain.ChatActivity) domain.ChatActivity
	}{
		{"subagent stop", func(s *domain.ChatActivity) domain.ChatActivity {
			return commands.StopSubagent{ChatID: chat, SubagentID: "a1", Now: now}.EmitEvent(s)
		}},
		{"tool completion", func(s *domain.ChatActivity) domain.ChatActivity {
			return commands.CompleteTool{ChatID: chat, ToolID: "t9", Now: now}.EmitEvent(s)
		}},
		{"interruption resolution", func(s *domain.ChatActivity) domain.ChatActivity {
			return commands.ResolveInterruption{ChatID: chat, ID: "i9", Kind: "k", Now: now}.EmitEvent(s)
		}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.emit(&closed)

			assert.Nil(t, got.Turn, "closing something never opens a turn")
			require.NotNil(t, got.Last, "and the event is still recorded")
		})
	}
}

func TestRegression_ALateSubagentStopDoesNotMakeAnIdleNotificationLookBlocking(t *testing.T) {
	opened := commands.OpenTurn{ChatID: chat, TurnID: "t1", Now: now}.EmitEvent(nil)
	closed := commands.CloseTurn{ChatID: chat, TurnID: "reply", Text: "done", Now: now}.
		EmitEvent(&opened)
	late := commands.StopSubagent{ChatID: chat, SubagentID: "anon", Now: now.Add(4 * time.Second)}.
		EmitEvent(&closed)

	idle := commands.Interrupt{
		ChatID: chat, ID: "i1", Kind: "notification",
		Detail: "Claude is waiting for your input", Now: now.Add(time.Minute),
	}.EmitEvent(&late)

	require.NotNil(t, idle.Last.Interruption)
	assert.NotNil(t, idle.Last.Interruption.ResolvedAt,
		"an idle notification must never render as the agent being blocked")
	assert.Empty(t, idle.Interruptions)
}
