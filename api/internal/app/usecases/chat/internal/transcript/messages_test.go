package transcript_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// A prompt Crowbar delivered is said once its turn takes it, under the id the
// client queued it with.
func TestProjector_ACrowbarPromptIsListedOnceItsTurnStarts(t *testing.T) {
	b := newBook(t)
	b.add(mk(t, ledger.KindUserMessage, "T1", "", ledger.UserMessage{
		RequestID: "req-1", Text: "do it", Origin: ledger.OriginCrowbar, Delivery: ledger.DeliveryRestart,
	}))
	assert.Empty(t, b.messages(), "a queued prompt is not said yet")

	b.add(mk(t, ledger.KindTurnStarted, "T1", "", ledger.TurnStarted{Provider: "p1", RunnerID: "run-1", RequestID: "req-1"}))

	got := b.messages()
	require.Len(t, got, 1)
	assert.Equal(t, "req-1", got[0].ID)
	assert.Equal(t, domain.TurnRoleUser, got[0].Role)
	assert.Equal(t, "p1", got[0].Provider)
	assert.Equal(t, int64(got[0].Sequence), got[0].DisplayOrder, "it is said where its turn started")
}

func TestProjector_AHarnessMessageKeepsItsRole(t *testing.T) {
	b := newBook(t)
	b.add(
		mk(t, ledger.KindUserMessage, "T1", "", ledger.UserMessage{Text: "a subagent handed back", Origin: ledger.OriginHarness}),
		mk(t, ledger.KindTurnStarted, "T1", "", ledger.TurnStarted{Provider: "p1"}),
	)

	got := b.messages()
	require.Len(t, got, 1)
	assert.Equal(t, domain.TurnRoleHarness, got[0].Role)
}

func TestProjector_ANoteIsCrowbarsOwn(t *testing.T) {
	b := newBook(t)
	b.add(mk(t, ledger.KindUserMessage, "N1", "", ledger.UserMessage{Text: "switched", Origin: ledger.OriginNote}))

	got := b.messages()
	require.Len(t, got, 1)
	assert.Equal(t, "crowbar", got[0].Provider)
}

func TestProjector_AFailedTurnReadsAsANotice(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.add(mk(t, ledger.KindTurnFailed, "T1", "", ledger.TurnFailed{Reason: "rate_limited", Detail: "slow down"}))
	b.add(mk(t, ledger.KindTurnFailed, "T1", "", ledger.TurnFailed{Reason: "again"}))
	b.open("T2", "go on")
	b.add(mk(t, ledger.KindTurnFailed, "T2", "", ledger.TurnFailed{Reason: "rate_limited"}))

	got := b.messages()
	require.Len(t, got, 4, "one notice per failed turn")
	assert.Equal(t, domain.TurnRoleNotice, got[1].Role)
	assert.Equal(t, "notice-T1", got[1].ID)
	assert.Equal(t, "slow down", got[1].Text, "the detail is what the person reads")
	assert.Equal(t, "rate_limited", got[3].Text, "with none, the reason")
	turns := b.activity().Turns
	assert.Equal(t, "failed", turns[1].Status)
}

func TestProjector_AReplyCarriesItsTurnsEnd(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.say("", "m1", "partial")
	b.add(mk(t, ledger.KindTurnEnded, "T1", "", ledger.TurnEnded{
		Status: string(ledger.TurnStatusInterrupted), RunnerID: "run-1", Effort: "high", Diff: "+x",
	}))
	b.add(mk(t, ledger.KindTurnEnded, "T1", "", ledger.TurnEnded{Status: "done", RunnerID: "run-1"}))

	reply := b.activity().Turns[1]
	assert.Equal(t, "interrupted", reply.Status, "the first end is the turn's")
	assert.Equal(t, "high", reply.Effort)
	assert.Equal(t, "+x", reply.Diff)
	assert.Equal(t, "high", b.messages()[1].Effort)
}

// A turn whose start never came is its ending runner's, and so of the provider
// that runner ran.
func TestProjector_AReplyWithNoStartIsItsRunnersProvider(t *testing.T) {
	b := newBook(t)
	b.add(mk(t, ledger.KindRunnerStarted, "", "", ledger.RunnerStarted{RunnerID: "run-1", Provider: "p1"}))
	b.say("", "m1", "done thing")
	b.end()

	got := b.messages()
	require.Len(t, got, 1)
	assert.Equal(t, "p1", got[0].Provider)
	assert.Equal(t, "run-1", got[0].RunnerID)
}

func TestProjector_TheTurnsPlanRidesItsLastMessage(t *testing.T) {
	b := newBook(t)
	b.open("T1", "plan it")
	b.add(mk(t, ledger.KindPlan, "T1", "", ledger.Plan{Op: ledger.PlanReplace, Items: []ledger.PlanItem{
		{ID: "a", Text: "read", Status: "done"}, {ID: "b", Text: "write", Status: "pending"},
	}}))
	b.add(mk(t, ledger.KindPlan, "T1", "", ledger.Plan{Op: ledger.PlanUpsert, Items: []ledger.PlanItem{
		{ID: "b", Text: "write", Status: "done"}, {ID: "c", Text: "ship"},
	}}))
	b.add(mk(t, ledger.KindPlan, "T1", "A", ledger.Plan{Op: ledger.PlanReplace, Items: []ledger.PlanItem{{Text: "nested"}}}))
	b.say("", "m1", "first")
	b.say("", "m2", "last")

	turns := b.activity().Turns
	require.Len(t, turns, 3)
	assert.Empty(t, turns[1].Plan)
	assert.Equal(t, []domain.ActivityPlanStep{
		{Text: "read", Status: "done"}, {Text: "write", Status: "done"}, {Text: "ship"},
	}, turns[2].Plan, "an upsert replaces by id and appends the new")
	require.NotNil(t, turns[2].PlanUpdatedAt)
}

// A late chunk that extends a settled reply folds into it: the reply keeps its
// place, its start and its turn's plan, and carries the full text.
func TestProjector_ALateExtensionKeepsTheRepliesPlaceAndPlan(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.add(mk(t, ledger.KindPlan, "T1", "", ledger.Plan{Op: ledger.PlanReplace, Items: []ledger.PlanItem{{Text: "step"}}}))
	b.add(mk(t, ledger.KindAssistantText, "T1", "", ledger.AssistantText{MessageID: "m1", Index: 0, Text: "the ans"}))
	b.end()
	before := b.activity().Turns[1]

	b.add(mk(t, ledger.KindAssistantText, "T1", "", ledger.AssistantText{MessageID: "m1", Index: 1, Final: true, Text: "wer"}))

	after := b.activity().Turns
	require.Len(t, after, 2, "it joins its reply")
	assert.Equal(t, "the answer", after[1].Text)
	assert.Equal(t, before.DisplayOrder, after[1].DisplayOrder)
	assert.Equal(t, before.StartedAt, after[1].StartedAt)
	assert.Equal(t, before.Plan, after[1].Plan)
	b.mu.Lock()
	defer b.mu.Unlock()
	require.NotEmpty(t, b.changed, "the settled change is published")
	assert.Equal(t, "the answer", b.changed[len(b.changed)-1].Text)
}

// A subagent's words are its own: they never enter the chat's messages.
func TestProjector_AScopesWordsAreNotTheChats(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.say("A", "s1", "from a subagent")

	assert.Equal(t, []string{"go"}, texts(b.messages()))
}
