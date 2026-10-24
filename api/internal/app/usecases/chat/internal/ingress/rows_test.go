package ingress

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

const testChat = "chat-1"

var at0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

// chatRows is a chat's ledger state, Map's rows committed to it as the store
// would commit them.
type chatRows struct {
	t  *testing.T
	st ledger.State
}

func newChatRows(t *testing.T, rows ...ledger.Row) *chatRows {
	t.Helper()
	c := &chatRows{t: t, st: ledger.Fold(testChat, nil)}
	c.commit(rows...)
	return c
}

// liveChatRows is a chat whose runner run-1 runs.
func liveChatRows(t *testing.T) *chatRows {
	t.Helper()
	return newChatRows(t, row(t, ledger.KindRunnerStarted, "", "", ledger.RunnerStarted{RunnerID: "run-1", Provider: "p1"}))
}

func (c *chatRows) commit(rows ...ledger.Row) {
	for _, r := range rows {
		r.ChatID, r.Seq = testChat, c.st.Head()+1
		r.At = at0.Add(time.Duration(r.Seq) * time.Second)
		c.st.Apply(r)
	}
}

// mapped maps ev from src and commits its rows.
func (c *chatRows) mapped(ev engineagents.CanonicalEvent, src Source) ([]ledger.Row, Effects) {
	c.t.Helper()
	rows, fx, err := Map(c.st, ev, src, BlobRefs{})
	require.NoError(c.t, err)
	c.commit(rows...)
	return rows, fx
}

// typed opens turn pt-1 on words typed into the CLI, and returns it.
func (c *chatRows) typed(text string) string {
	c.t.Helper()
	s := hooksSource()
	s.Prompt = Prompt{Text: text, Origin: ledger.OriginProvider}
	c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, TurnID: "pt-1"}, s)
	turn, ok := c.st.RunningTurn()
	require.True(c.t, ok)
	return turn.ID
}

func hooksSource() Source {
	return Source{RunnerID: "run-1", Provider: "p1", Held: true, Answerable: true, Channel: engineagents.ChannelHooks}
}

func row[B any](t *testing.T, kind ledger.Kind, turnID, scope string, body B) ledger.Row {
	t.Helper()
	r, err := ledger.NewRow(kind, turnID, scope, body)
	require.NoError(t, err)
	return r
}

func kinds(rows []ledger.Row) []ledger.Kind {
	out := make([]ledger.Kind, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Kind)
	}
	return out
}

func bodyOf[B any](t *testing.T, r ledger.Row) B {
	t.Helper()
	b, err := ledger.DecodeBody[B](r)
	require.NoError(t, err)
	return b
}

func crowbarPrompt(t *testing.T, requestID, text, delivery string) ledger.Row {
	t.Helper()
	return row(t, ledger.KindUserMessage, "T-"+requestID, "", ledger.UserMessage{
		RequestID: requestID, Text: text, Origin: ledger.OriginCrowbar, Delivery: delivery, RunnerID: "run-1",
	})
}

func TestMap_ATypedPromptOpensATurnWithItsWords(t *testing.T) {
	c := liveChatRows(t)
	s := hooksSource()
	s.Prompt = Prompt{Text: "hello", Origin: ledger.OriginProvider}

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, TurnID: "pt-1"}, s)

	require.Equal(t, []ledger.Kind{ledger.KindUserMessage, ledger.KindTurnStarted}, kinds(rows))
	assert.Equal(t, "hello", bodyOf[ledger.UserMessage](t, rows[0]).Text)
	started := bodyOf[ledger.TurnStarted](t, rows[1])
	assert.Equal(t, "pt-1", started.ProviderTurnID)
	assert.Equal(t, "run-1", started.RunnerID)
	assert.Equal(t, rows[0].TurnID, rows[1].TurnID)
}

func TestMap_ARepeatedUserPromptOpensNoSecondTurn(t *testing.T) {
	c := liveChatRows(t)
	c.typed("hello")
	s := hooksSource()
	s.Prompt = Prompt{Text: "hello", Origin: ledger.OriginProvider}

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, TurnID: "pt-1"}, s)

	assert.Empty(t, rows)
}

// A prompt opens a turn only on the chat's live runner, held against its end,
// and never from inside a subagent.
func TestMap_APromptOpensNoTurnWhereNoneMayStart(t *testing.T) {
	prompt := engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt}
	unheld := hooksSource()
	unheld.Held = false
	other := hooksSource()
	other.RunnerID = "run-2"
	nested := prompt
	nested.Subagent = &engineagents.SubagentEvent{ID: "A"}

	for name, tc := range map[string]struct {
		ev  engineagents.CanonicalEvent
		src Source
	}{
		"unheld":        {prompt, unheld},
		"another CLI":   {prompt, other},
		"in a subagent": {nested, hooksSource()},
	} {
		t.Run(name, func(t *testing.T) {
			rows, _ := liveChatRows(t).mapped(tc.ev, tc.src)
			assert.Empty(t, rows)
		})
	}
}

func TestMap_APromptSupersedesTheRunningTurn(t *testing.T) {
	c := liveChatRows(t)
	first := c.typed("one")
	s := hooksSource()
	s.Prompt = Prompt{Text: "two", Origin: ledger.OriginProvider}

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, TurnID: "pt-2"}, s)

	require.Equal(t, []ledger.Kind{ledger.KindTurnEnded, ledger.KindUserMessage, ledger.KindTurnStarted}, kinds(rows))
	assert.Equal(t, first, rows[0].TurnID)
	assert.Equal(t, ledger.ReasonSuperseded, bodyOf[ledger.TurnEnded](t, rows[0]).Reason)
}

// Words typed that are a Crowbar prompt's start the turn that prompt queued:
// it is said once, under its request.
func TestMap_TypedWordsOfACrowbarPromptStartItsTurn(t *testing.T) {
	c := liveChatRows(t)
	c.commit(crowbarPrompt(t, "req-1", "do it", ledger.DeliveryRestart))
	s := hooksSource()
	s.Prompt = Prompt{Text: "do it", Origin: ledger.OriginProvider}

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt}, s)

	require.Equal(t, []ledger.Kind{ledger.KindTurnStarted}, kinds(rows))
	assert.Equal(t, "T-req-1", rows[0].TurnID)
	assert.Equal(t, "req-1", bodyOf[ledger.TurnStarted](t, rows[0]).RequestID)
}

// Words matching a prompt another runner carries are the live CLI's own: the
// other runner's prompt is not started here.
func TestMap_TypedWordsOfAnotherRunnersPromptAreItsOwn(t *testing.T) {
	c := liveChatRows(t)
	c.commit(row(t, ledger.KindUserMessage, "T-req-1", "", ledger.UserMessage{
		RequestID: "req-1", Text: "do it", Origin: ledger.OriginCrowbar, Delivery: ledger.DeliveryRestart, RunnerID: "run-0",
	}))
	s := hooksSource()
	s.Prompt = Prompt{Text: "do it", Origin: ledger.OriginProvider}

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt}, s)

	require.Equal(t, []ledger.Kind{ledger.KindUserMessage, ledger.KindTurnStarted}, kinds(rows))
	assert.NotEqual(t, "T-req-1", rows[1].TurnID)
	assert.Empty(t, bodyOf[ledger.TurnStarted](t, rows[1]).RequestID)
}

func TestMap_AnInjectedPromptIsHarnessNotUser(t *testing.T) {
	c := liveChatRows(t)
	s := hooksSource()
	s.Prompt = Prompt{Text: "a subagent finished", Origin: ledger.OriginHarness}

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt}, s)

	require.Equal(t, []ledger.Kind{ledger.KindUserMessage, ledger.KindTurnStarted}, kinds(rows))
	assert.Equal(t, ledger.OriginHarness, bodyOf[ledger.UserMessage](t, rows[0]).Origin)

	c2 := liveChatRows(t)
	rows, _ = c2.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt}, hooksSource())
	assert.Equal(t, []ledger.Kind{ledger.KindTurnStarted}, kinds(rows), "Crowbar's own context alone is nobody's words")
}

func TestMap_ATurnStopRecordsItsReplyWithItsEnd(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("go")
	s := hooksSource()
	s.Live = Unrecorded{
		Answers: []liveText{
			{TurnID: turnID, ID: "a1", Text: "streamed"},
			{TurnID: "other", ID: "a2", Text: "of another turn"},
			{TurnID: turnID, Scope: "A", ID: "a3", Text: "a subagent's"},
		},
		Reasoning: []liveText{{TurnID: turnID, ID: "r1", Text: "thinking"}},
		Diff:      []liveText{{TurnID: turnID, ID: turnID, Text: "+x"}},
	}

	rows, fx := c.mapped(engineagents.CanonicalEvent{
		Kind: engineagents.HookTurnStop, TurnID: "pt-1", Message: "streamed, finished", Effort: "high",
	}, s)

	require.Equal(t, []ledger.Kind{
		ledger.KindAssistantText, ledger.KindAssistantText, ledger.KindReasoning, ledger.KindTurnEnded,
	}, kinds(rows))
	assert.Equal(t, "a1", bodyOf[ledger.AssistantText](t, rows[0]).MessageID)
	reply := bodyOf[ledger.AssistantText](t, rows[1])
	assert.Equal(t, "a1", reply.MessageID, "the reply is the turn's last message")
	assert.Equal(t, "streamed, finished", reply.Text)
	assert.Equal(t, "thinking", bodyOf[ledger.Reasoning](t, rows[2]).Text)
	end := bodyOf[ledger.TurnEnded](t, rows[3])
	assert.Equal(t, "+x", end.Diff)
	assert.Equal(t, "high", end.Effort)
	assert.Equal(t, turnID, fx.Ended)
	_, running := c.st.RunningTurn()
	assert.False(t, running)
}

func TestMap_AReplyWithNothingStreamedIsTheTurnsReply(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("go")

	rows, _ := c.mapped(engineagents.CanonicalEvent{
		Kind: engineagents.HookTurnStop, Message: "done", TurnStatus: "interrupted",
	}, hooksSource())

	assert.Equal(t, ledger.ReplyID(turnID), bodyOf[ledger.AssistantText](t, rows[0]).MessageID)
	assert.Equal(t, string(ledger.TurnStatusInterrupted), bodyOf[ledger.TurnEnded](t, rows[1]).Status)
}

func TestMap_ATurnsEndAbandonsItsOpenAsks(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")
	c.mapped(engineagents.CanonicalEvent{
		Kind:   engineagents.HookPermission,
		Choice: &engineagents.ChoicePrompt{Kind: "tool_permission", ToolName: "Bash"},
	}, hooksSource())
	require.Len(t, c.st.PendingAsks(), 1)

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop}, hooksSource())

	require.Equal(t, []ledger.Kind{ledger.KindAskResolved, ledger.KindTurnEnded}, kinds(rows))
	assert.Equal(t, ledger.ResolutionAbandoned, bodyOf[ledger.AskResolved](t, rows[0]).Resolution)
}

// A compaction's own turn wraps it like any turn; its end ends the compaction,
// never a turn of the chat's.
func TestMap_ACompactionTurnStopEndsTheCompactionNotTheTurn(t *testing.T) {
	for _, kind := range []string{engineagents.HookTurnStop, engineagents.HookTurnFailed} {
		t.Run(kind, func(t *testing.T) {
			c := liveChatRows(t)
			c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookCompactPre, TurnID: "compact-1"}, hooksSource())

			rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: kind, TurnID: "compact-1", Message: "x"}, hooksSource())

			require.Equal(t, []ledger.Kind{ledger.KindMarker}, kinds(rows))
			assert.Equal(t, ledger.MarkerCompactionEnded, bodyOf[ledger.Marker](t, rows[0]).Kind)
		})
	}
}

func TestMap_AFailureKeepsItsReasonAndItsReply(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")

	rows, fx := c.mapped(engineagents.CanonicalEvent{
		Kind: engineagents.HookTurnFailed, Message: "I could not",
		Failure: &engineagents.TurnFailure{Reason: "usage_limit", Detail: "out of credit"},
	}, hooksSource())

	require.Equal(t, []ledger.Kind{ledger.KindAssistantText, ledger.KindTurnFailed}, kinds(rows))
	assert.Equal(t, "I could not", bodyOf[ledger.AssistantText](t, rows[0]).Text)
	failed := bodyOf[ledger.TurnFailed](t, rows[1])
	assert.Equal(t, "usage_limit", failed.Reason)
	assert.Equal(t, "out of credit", failed.Detail)
	assert.NotEmpty(t, fx.Ended)

	c2 := liveChatRows(t)
	c2.typed("go")
	rows, _ = c2.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnFailed, Message: "boom"}, hooksSource())
	require.Equal(t, []ledger.Kind{ledger.KindTurnFailed}, kinds(rows), "with no detail, the message is the detail")
	assert.Equal(t, ledger.TurnFailed{Reason: ledger.ReasonTurnFailed, Detail: "boom"}, bodyOf[ledger.TurnFailed](t, rows[0]))
}

func TestMap_AFailureSettlesTheSteerItsTurnCarried(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")
	c.commit(crowbarPrompt(t, "req-1", "and also", ledger.DeliverySteer))

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnFailed}, hooksSource())

	require.Equal(t, []ledger.Kind{ledger.KindTurnFailed, ledger.KindPromptSettled}, kinds(rows))
	assert.Equal(t, ledger.ReasonTurnFailed, bodyOf[ledger.PromptSettled](t, rows[1]).Reason)
}

func TestMap_AnIdleEndsTheRunningTurnAndSettlesItsSteer(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")
	c.commit(crowbarPrompt(t, "req-1", "and also", ledger.DeliverySteer))

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookIdle}, hooksSource())

	require.Equal(t, []ledger.Kind{ledger.KindTurnEnded, ledger.KindPromptSettled}, kinds(rows))
	assert.Equal(t, ledger.ReasonProviderIdle, bodyOf[ledger.TurnEnded](t, rows[0]).Reason)
	assert.Equal(t, ledger.ReasonDeliveryFailed, bodyOf[ledger.PromptSettled](t, rows[1]).Reason)
}

// An idle that ends the turn records what the turn said, as any end does.
func TestMap_AnIdleThatEndsTheTurnKeepsWhatItSaid(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("go")
	c.mapped(permission(engineagents.ChoicePrompt{Kind: "tool_permission", ToolName: "Bash"}), hooksSource())
	s := hooksSource()
	s.Live = Unrecorded{Answers: []liveText{{TurnID: turnID, ID: "a1", Text: "streamed"}}}

	rows, fx := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookIdle}, s)

	require.Equal(t, []ledger.Kind{ledger.KindAssistantText, ledger.KindAskResolved, ledger.KindTurnEnded}, kinds(rows))
	assert.Equal(t, "streamed", bodyOf[ledger.AssistantText](t, rows[0]).Text)
	assert.Equal(t, turnID, fx.Ended)
	assert.Empty(t, c.st.PendingAsks())
}

// An idle with nothing running is recorded late: on hooks it also says the
// prompts the CLI carried were never taken; on an api connection, nothing more.
func TestMap_AnIdleWithNothingRunningIsRecordedLate(t *testing.T) {
	c := liveChatRows(t)
	c.commit(crowbarPrompt(t, "req-1", "hello", ledger.DeliveryRestart))
	api := hooksSource()
	api.Channel = engineagents.ChannelAPI

	rows, fx := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookIdle}, api)
	assert.Equal(t, []ledger.Kind{ledger.KindTurnEnded}, kinds(rows))
	assert.Empty(t, fx.Ended)

	rows, _ = c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookIdle}, hooksSource())
	require.Equal(t, []ledger.Kind{ledger.KindTurnEnded, ledger.KindPromptSettled}, kinds(rows))
	assert.Equal(t, ledger.ReasonNotAcknowledged, bodyOf[ledger.PromptSettled](t, rows[1]).Reason)
}

// A late end of a turn already over is written to it, restating the work its
// CLI still runs in the background.
func TestMap_ALateTurnStopRestatesTheBackgroundWork(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("go")
	c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, TurnID: "pt-1"}, hooksSource())

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, TurnID: "pt-1", AsyncWork: 2}, hooksSource())

	require.Equal(t, []ledger.Kind{ledger.KindTurnEnded}, kinds(rows))
	assert.Equal(t, turnID, rows[0].TurnID)
	require.NotNil(t, bodyOf[ledger.TurnEnded](t, rows[0]).AsyncWork)
	assert.Equal(t, 2, *bodyOf[ledger.TurnEnded](t, rows[0]).AsyncWork)
}

func TestMap_ANotificationIsAMarker(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("go")

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookNotification, Message: "look"}, hooksSource())

	require.Equal(t, []ledger.Kind{ledger.KindMarker}, kinds(rows))
	assert.Equal(t, turnID, rows[0].TurnID)
	assert.Equal(t, ledger.Marker{Kind: ledger.MarkerNotification, Detail: "look"}, bodyOf[ledger.Marker](t, rows[0]))
}

func TestMap_AnEventItDoesNotRecordWritesNothing(t *testing.T) {
	rows, fx := liveChatRows(t).mapped(engineagents.CanonicalEvent{Kind: engineagents.HookSessionEnd}, hooksSource())
	assert.Empty(t, rows)
	assert.Equal(t, Effects{}, fx)
}

func TestSalvage_RecordsWhatNoRowHolds(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("go")
	c.commit(row(t, ledger.KindAssistantText, turnID, "", ledger.AssistantText{MessageID: "done", Index: ledger.IndexComplete, Text: "x"}))

	rows, err := Salvage(c.st, []liveText{
		{TurnID: turnID, ID: "kept", Text: "said"},
		{TurnID: turnID, ID: "done", Text: "already recorded"},
		{ID: "orphan", Text: "of no turn"},
	})

	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "kept", bodyOf[ledger.AssistantText](t, rows[0]).MessageID)
}

func TestNoticeRows_FailTheRunningTurnOrTheCarriedPrompts(t *testing.T) {
	notice := engineagents.TerminalNotice{Kind: "usage_limit", Text: "out of credit"}

	c := liveChatRows(t)
	turnID := c.typed("go")
	c.commit(crowbarPrompt(t, "req-1", "and also", ledger.DeliverySteer))
	rows, err := noticeRows(c.st, "run-1", notice, Unrecorded{Answers: []liveText{{TurnID: turnID, ID: "a1", Text: "partial"}}})
	require.NoError(t, err)
	assert.Equal(t, []ledger.Kind{ledger.KindAssistantText, ledger.KindTurnFailed, ledger.KindPromptSettled}, kinds(rows))

	rows, err = noticeRows(c.st, "run-2", notice, Unrecorded{})
	require.NoError(t, err)
	assert.Empty(t, rows, "another runner's notice ends nothing of this turn")

	idle := liveChatRows(t)
	idle.commit(crowbarPrompt(t, "req-2", "hello", ledger.DeliveryRestart))
	rows, err = noticeRows(idle.st, "run-1", notice, Unrecorded{})
	require.NoError(t, err)
	require.Equal(t, []ledger.Kind{ledger.KindTurnFailed, ledger.KindPromptSettled}, kinds(rows))
	assert.Equal(t, "out of credit", bodyOf[ledger.TurnFailed](t, rows[0]).Detail)
	assert.Equal(t, "usage_limit", bodyOf[ledger.PromptSettled](t, rows[1]).Reason)
}
