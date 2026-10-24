package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func delta(kind, messageID string, sequenced, final bool, text string) engineagents.CanonicalEvent {
	return engineagents.CanonicalEvent{Kind: kind, Delta: &engineagents.MessageDelta{
		MessageID: messageID, Sequenced: sequenced, Final: final, Text: text,
	}}
}

func toolEvent(kind, id, name string) engineagents.CanonicalEvent {
	return engineagents.CanonicalEvent{Kind: kind, Tool: &engineagents.ToolEvent{ID: id, Name: name}}
}

func permission(choice engineagents.ChoicePrompt) engineagents.CanonicalEvent {
	return engineagents.CanonicalEvent{Kind: engineagents.HookPermission, Choice: &choice}
}

func TestMap_UnsequencedDeltaWritesNoRow(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("go")

	rows, fx := c.mapped(delta(engineagents.HookMessageDelta, "m1", false, false, "hel"), hooksSource())

	assert.Empty(t, rows)
	require.NotNil(t, fx.Live)
	assert.Equal(t, LiveDelta{TurnID: turnID, ID: "m1", Text: "hel", ShownAs: "m1"}, *fx.Live)
}

// What is only shown while it happens is one stream per block, else per turn.
func TestMap_TheOtherStreamsAreOnlyShown(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")

	for name, tc := range map[string]struct {
		ev       engineagents.CanonicalEvent
		kind, id string
		replace  bool
	}{
		"reasoning":   {delta(engineagents.HookReasoningDelta, "r1", false, false, "hm"), KindReasoning, "r1", false},
		"tool output": {delta(engineagents.HookToolOutputDelta, "t1", false, false, "out"), KindToolOutput, "t1", false},
		"diff":        {delta(engineagents.HookDiffUpdate, "", false, false, "+x"), KindDiff, "pt-9", true},
	} {
		t.Run(name, func(t *testing.T) {
			tc.ev.Delta.TurnID = "pt-9"
			rows, fx := c.mapped(tc.ev, hooksSource())
			assert.Empty(t, rows)
			require.NotNil(t, fx.Live)
			assert.Equal(t, tc.kind, fx.Live.Kind)
			assert.Equal(t, tc.id, fx.Live.ID)
			assert.Equal(t, tc.replace, fx.Live.Replace)
		})
	}

	for _, ev := range []engineagents.CanonicalEvent{
		{Kind: engineagents.HookMessageDelta},
		delta(engineagents.HookMessageDelta, "", false, false, "no block"),
		delta(engineagents.HookReasoningDelta, "r1", false, false, ""),
	} {
		rows, fx := c.mapped(ev, hooksSource())
		assert.Empty(t, rows)
		assert.Nil(t, fx.Live)
	}
}

func TestMap_ANumberedChunkIsRecordedOnce(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("go")
	chunk := delta(engineagents.HookMessageDelta, "m1", true, false, "hel")

	rows, fx := c.mapped(chunk, hooksSource())
	require.Equal(t, []ledger.Kind{ledger.KindAssistantText}, kinds(rows))
	assert.Equal(t, turnID, rows[0].TurnID)
	require.NotNil(t, fx.Live)
	assert.True(t, fx.Live.Numbered)

	again, _ := c.mapped(chunk, hooksSource())
	assert.Empty(t, again, "a retried chunk is recorded once")
}

// A message begun after its turn closed on a reply it reads as joins that
// reply, so one answer is one message whichever of its reports landed first.
func TestMap_AChunkAfterTheTurnEndedJoinsItsReply(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("go")
	c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, Message: "the answer"}, hooksSource())

	rows, fx := c.mapped(delta(engineagents.HookMessageDelta, "m1", true, true, "the answer"), hooksSource())

	require.Len(t, rows, 1)
	assert.Equal(t, ledger.ReplyID(turnID), bodyOf[ledger.AssistantText](t, rows[0]).Reply)
	assert.Equal(t, ledger.ReplyID(turnID), fx.Live.ShownAs, "the live frame names the message it joins")
}

func TestMap_AMessageOfNoTurnIsRefused(t *testing.T) {
	c := liveChatRows(t)

	_, _, err := Map(c.st, delta(engineagents.HookMessageDelta, "m1", true, false, "x"), hooksSource(), BlobRefs{})
	require.ErrorIs(t, err, seam.ErrHookNotRecorded)
	_, _, err = Map(c.st, engineagents.CanonicalEvent{
		Kind:     engineagents.HookMessageComplete,
		Complete: &engineagents.MessageComplete{MessageID: "m1", Text: "x"},
	}, hooksSource(), BlobRefs{})
	require.ErrorIs(t, err, seam.ErrHookNotRecorded)
}

func TestMap_AWholeMessageIsRecordedOnceInItsTurn(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("go")
	c.mapped(delta(engineagents.HookMessageDelta, "m1", true, false, "hel"), hooksSource())
	complete := engineagents.CanonicalEvent{
		Kind:     engineagents.HookMessageComplete,
		Complete: &engineagents.MessageComplete{MessageID: "m1", Text: "hello", Phase: "final"},
	}

	rows, _ := c.mapped(complete, hooksSource())
	require.Len(t, rows, 1)
	assert.Equal(t, turnID, rows[0].TurnID)
	assert.Equal(t, ledger.IndexComplete, bodyOf[ledger.AssistantText](t, rows[0]).Index)

	again, _ := c.mapped(complete, hooksSource())
	assert.Empty(t, again)
	none, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookMessageComplete}, hooksSource())
	assert.Empty(t, none)
}

func TestMap_ARepeatedToolPhaseWritesNoRow(t *testing.T) {
	for _, channel := range []engineagents.Channel{engineagents.ChannelHooks, engineagents.ChannelAPI} {
		t.Run(string(channel), func(t *testing.T) {
			c := liveChatRows(t)
			c.typed("go")
			s := hooksSource()
			s.Channel = channel

			first, _ := c.mapped(toolEvent(engineagents.HookToolPre, "t1", "Bash"), s)
			again, _ := c.mapped(toolEvent(engineagents.HookToolPre, "t1", "Bash"), s)

			assert.Len(t, first, 1)
			assert.Empty(t, again, "a relay's retry records its fact once")
		})
	}
}

func TestMap_AToolCallRecordsWhatItsPhasesSay(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")
	pre := toolEvent(engineagents.HookToolPre, "t1", "Edit")
	pre.Tool.Kind, pre.Tool.Target = "edit", "a.go"
	pre.Tool.Locations = []engineagents.ToolLocation{{Path: "a.go", Line: 2}}
	pre.Tool.EditBefore, pre.Tool.EditAfter = "a\n", "b\n"

	rows, _ := c.mapped(pre, hooksSource())
	body := bodyOf[ledger.ToolCall](t, rows[0])
	assert.Equal(t, ledger.ToolPre, body.Phase)
	assert.Equal(t, []ledger.ToolLocation{{Path: "a.go", Line: 2}}, body.Locations)
	assert.NotEmpty(t, body.Diff, "an edit's change is shown from its start")

	post := engineagents.CanonicalEvent{Kind: engineagents.HookToolPost, Tool: &engineagents.ToolEvent{Name: "Edit"}}
	rows, _ = c.mapped(post, hooksSource())
	body = bodyOf[ledger.ToolCall](t, rows[0])
	assert.Equal(t, "t1", body.ToolID, "an id-less end closes the open call of its name")
	assert.Equal(t, domain.ToolStatusOK, body.Status)
}

func TestMap_AFailedToolSaysWhy(t *testing.T) {
	for status, want := range map[string]string{"": domain.ToolStatusError, domain.ToolStatusDeclined: domain.ToolStatusDeclined} {
		c := liveChatRows(t)
		c.typed("go")
		ev := toolEvent(engineagents.HookToolFail, "t1", "Bash")
		ev.Tool.Status, ev.Tool.Error = status, "no"

		rows, _ := c.mapped(ev, hooksSource())

		body := bodyOf[ledger.ToolCall](t, rows[0])
		assert.Equal(t, want, body.Status)
		assert.Equal(t, "no", body.Error)
	}
}

func TestMap_AnIDLessToolStartGetsAnIDOfItsOwn(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")

	rows, _ := c.mapped(toolEvent(engineagents.HookToolPre, "", "Bash"), hooksSource())
	none, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookToolPre}, hooksSource())

	assert.Contains(t, bodyOf[ledger.ToolCall](t, rows[0]).ToolID, "tool-")
	assert.Empty(t, none)
}

func TestMap_SubagentToolCallsAreRecordedUnderTheirScope(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")
	ev := toolEvent(engineagents.HookToolPre, "t1", "Read")
	ev.Subagent = &engineagents.SubagentEvent{ID: "A"}

	rows, _ := c.mapped(ev, hooksSource())

	require.Len(t, rows, 1)
	assert.Equal(t, "A", rows[0].Scope)
}

func TestMap_AToolsEndClosesTheAsksItRaised(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")
	c.mapped(toolEvent(engineagents.HookToolPre, "t1", "Bash"), hooksSource())
	c.mapped(permission(engineagents.ChoicePrompt{Kind: "tool_permission", ToolName: "Bash"}), hooksSource())

	rows, _ := c.mapped(toolEvent(engineagents.HookToolPost, "t1", "Bash"), hooksSource())

	require.Equal(t, []ledger.Kind{ledger.KindToolCall, ledger.KindAskResolved}, kinds(rows))
	assert.Equal(t, ledger.ResolutionProceeded, bodyOf[ledger.AskResolved](t, rows[1]).Resolution)
}

func TestMap_ASubagentsPhasesAreRecordedOnce(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")
	start := engineagents.CanonicalEvent{
		Kind:     engineagents.HookSubagentPre,
		Subagent: &engineagents.SubagentEvent{ID: "A", AgentType: "explorer"},
	}
	stop := engineagents.CanonicalEvent{Kind: engineagents.HookSubagentPost, Subagent: &engineagents.SubagentEvent{ID: "A"}}

	rows, _ := c.mapped(start, hooksSource())
	require.Len(t, rows, 1)
	assert.Equal(t, ledger.Subagent{Phase: ledger.SubagentStart, Type: "explorer"}, bodyOf[ledger.Subagent](t, rows[0]))
	again, _ := c.mapped(start, hooksSource())
	assert.Empty(t, again)
	rows, _ = c.mapped(stop, hooksSource())
	assert.Equal(t, ledger.SubagentStop, bodyOf[ledger.Subagent](t, rows[0]).Phase)
	none, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookSubagentPre}, hooksSource())
	assert.Empty(t, none)
}

func TestMap_AnAskBindsToTheOpenToolOfItsScope(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")
	c.mapped(toolEvent(engineagents.HookToolPre, "t1", "Bash"), hooksSource())

	rows, _ := c.mapped(permission(engineagents.ChoicePrompt{Kind: "tool_permission", ToolName: "Bash"}), hooksSource())

	assert.Equal(t, "t1", bodyOf[ledger.Ask](t, rows[0]).ToolID)
}

func TestMap_AHeldAskIsAnswerable(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")

	rows, fx := c.mapped(permission(engineagents.ChoicePrompt{
		Kind: "question", PromptID: "p1", Title: "Pick", Question: "which?", Mode: "form", Multi: true,
		Options:   []engineagents.ChoiceOption{{ID: "a", Kind: "answer", Label: "A"}},
		Questions: []engineagents.PromptQuestion{{ID: "q1", Text: "and?", Options: []engineagents.ChoiceOption{{ID: "b"}}}},
		Schema:    []byte(`{"type":"object"}`),
	}), hooksSource())

	ask := bodyOf[ledger.Ask](t, rows[0])
	assert.True(t, ask.Answerable)
	assert.Equal(t, ask.AskID, fx.HoldAsk)
	assert.Equal(t, "Pick", ask.Title)
	assert.True(t, ask.Multi)
	assert.JSONEq(t, `[{"id":"a","kind":"answer","label":"A"}]`, string(ask.Options))
	assert.JSONEq(t, `[{"id":"q1","text":"and?","options":[{"id":"b","kind":""}]}]`, string(ask.Questions))
	assert.JSONEq(t, `{"type":"object"}`, string(ask.Schema))
}

func TestMap_AnAskCrowbarCannotHoldIsNotAnswerable(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")
	s := hooksSource()
	s.Answerable = false

	rows, fx := c.mapped(permission(engineagents.ChoicePrompt{Kind: "tool_permission", ToolName: "Bash"}), s)

	assert.False(t, bodyOf[ledger.Ask](t, rows[0]).Answerable)
	assert.Empty(t, fx.HoldAsk)
	assert.Len(t, c.st.PendingAsks(), 1, "it is still asked, of the CLI's own surface")
}

// An ask nothing waits on is recorded closed: one of a runner past its exit,
// or raised outside any turn.
func TestMap_AnAskNothingWaitsOnIsRecordedClosed(t *testing.T) {
	outside := liveChatRows(t)
	rows, fx := outside.mapped(permission(engineagents.ChoicePrompt{Kind: "tool_permission", ToolName: "Bash"}), hooksSource())
	assert.Equal(t, []ledger.Kind{ledger.KindAsk, ledger.KindAskResolved}, kinds(rows))
	assert.NotEmpty(t, fx.HoldAsk, "its CLI is held as any is")

	exited := liveChatRows(t)
	exited.typed("go")
	s := hooksSource()
	s.RunnerID = "run-gone"
	rows, fx = exited.mapped(permission(engineagents.ChoicePrompt{Kind: "tool_permission", ToolName: "Bash"}), s)
	assert.Equal(t, []ledger.Kind{ledger.KindAsk, ledger.KindAskResolved}, kinds(rows))
	assert.Empty(t, fx.HoldAsk)
}

func TestMap_ARepeatedAskJoinsTheFirst(t *testing.T) {
	for name, choice := range map[string]engineagents.ChoicePrompt{
		"by tool":      {Kind: "tool_permission", ToolName: "Bash"},
		"by prompt id": {Kind: "tool_permission", PromptID: "p1", ToolName: "Edit"},
		"by question":  {Kind: "question", Question: "which?"},
	} {
		for _, channel := range []engineagents.Channel{engineagents.ChannelHooks, engineagents.ChannelAPI} {
			t.Run(name+"/"+string(channel), func(t *testing.T) {
				c := liveChatRows(t)
				c.typed("go")
				c.mapped(toolEvent(engineagents.HookToolPre, "t1", "Bash"), hooksSource())
				s := hooksSource()
				s.Channel = channel

				_, first := c.mapped(permission(choice), s)
				rows, again := c.mapped(permission(choice), s)

				assert.Empty(t, rows)
				assert.Equal(t, first.HoldAsk, again.HoldAsk, "the retry is held on the ask it repeats")
				assert.Len(t, c.st.PendingAsks(), 1)
			})
		}
	}
}

func TestMap_ARepeatOfAnAskNotHeldIsNotHeld(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")
	s := hooksSource()
	s.Answerable = false
	c.mapped(permission(engineagents.ChoicePrompt{Kind: "tool_permission", ToolName: "Bash"}), s)

	_, fx := c.mapped(permission(engineagents.ChoicePrompt{Kind: "tool_permission", ToolName: "Bash"}), hooksSource())

	assert.Empty(t, fx.HoldAsk)
}

func TestMap_AnElicitationCarriesWhatItSaid(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")

	rows, _ := c.mapped(engineagents.CanonicalEvent{
		Kind:      engineagents.HookElicitation,
		Interrupt: &engineagents.InterruptEvent{Detail: "a server asks"},
	}, hooksSource())

	ask := bodyOf[ledger.Ask](t, rows[0])
	assert.Equal(t, "a server asks", ask.Detail)
	assert.Empty(t, ask.Kind, "offering no choice, it is no card")
}

func TestMap_APlanIsRestatedWhole(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("go")

	rows, _ := c.mapped(engineagents.CanonicalEvent{
		Kind: engineagents.HookPlanUpdate,
		Plan: []engineagents.PlanStep{{Text: "read", Status: "done"}},
	}, hooksSource())

	assert.Equal(t, turnID, rows[0].TurnID)
	assert.Equal(t, ledger.Plan{Op: ledger.PlanReplace, Items: []ledger.PlanItem{{Text: "read", Status: "done"}}},
		bodyOf[ledger.Plan](t, rows[0]))
}

func TestMap_ACompactionIsMarkedOnce(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("go")
	s := hooksSource()
	s.Manual = true

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookCompactPre}, s)
	require.Len(t, rows, 1)
	assert.Equal(t, turnID, rows[0].TurnID)
	assert.Equal(t, ledger.Marker{Kind: ledger.MarkerCompactionStarted, Detail: "manual"}, bodyOf[ledger.Marker](t, rows[0]))
	again, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookCompactPre}, s)
	assert.Empty(t, again)

	rows, _ = c.mapped(engineagents.CanonicalEvent{
		Kind:      engineagents.HookCompactPost,
		Interrupt: &engineagents.InterruptEvent{Detail: "auto"},
	}, hooksSource())
	assert.Equal(t, ledger.Marker{Kind: ledger.MarkerCompactionEnded, Detail: "auto"}, bodyOf[ledger.Marker](t, rows[0]))
	none, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookCompactPost}, hooksSource())
	assert.Empty(t, none, "an end of nothing open repeats one recorded")
}

// A compaction asked for with a prompt spends that prompt: no turn will.
func TestMap_ACompactionAskedByPromptSpendsIt(t *testing.T) {
	c := liveChatRows(t)
	c.commit(crowbarPrompt(t, "req-1", "/compact", ledger.DeliveryRestart))
	s := hooksSource()
	s.CompactsByPrompt = true

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookCompactPre}, s)

	require.Equal(t, []ledger.Kind{ledger.KindMarker, ledger.KindPromptSettled}, kinds(rows))
	settled := bodyOf[ledger.PromptSettled](t, rows[1])
	assert.True(t, settled.Consumed)
	assert.Equal(t, ledger.ReasonProviderBuiltin, settled.Reason)
}
