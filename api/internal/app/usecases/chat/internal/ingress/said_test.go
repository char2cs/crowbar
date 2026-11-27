package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestMap_ATurnStopRecordsEachThoughtWithTheMessageItNames(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("go")
	s := hooksSource()
	s.Live = Unrecorded{
		Answers: []liveText{{TurnID: turnID, ID: "a1", Text: "one"}, {TurnID: turnID, ID: "a2", Text: "two"}},
		Reasoning: []liveText{
			{TurnID: turnID, ID: "a1", Text: "first"},
			{TurnID: turnID, ID: "a2", Text: "second"},
			{TurnID: turnID, ID: "x", Text: "unnamed"},
			{TurnID: "other", ID: "a1", Text: "of another turn"},
		},
	}

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, TurnID: "pt-1", Message: "two"}, s)

	require.Equal(t, []ledger.Kind{
		ledger.KindAssistantText, ledger.KindAssistantText, ledger.KindReasoning, ledger.KindReasoning, ledger.KindTurnEnded,
	}, kinds(rows))
	assert.Equal(t, ledger.Reasoning{MessageID: "a1", Text: "first"}, bodyOf[ledger.Reasoning](t, rows[2]))
	assert.Equal(t, ledger.Reasoning{MessageID: "a2", Text: "unnamed"}, bodyOf[ledger.Reasoning](t, rows[3]),
		"a thought naming no message is the last message's, and the newest of them stands")
}

func (f *recorderFixture) dispatchOn(t *testing.T, channel engineagents.Channel, evs ...engineagents.CanonicalEvent) {
	t.Helper()
	for _, ev := range evs {
		_, _, err := f.r.Dispatch(t.Context(), f.runner, f.agent, ev, []byte(`{}`), channel)
		require.NoError(t, err)
	}
}

func transcript(t *testing.T, rows []ledger.Row) []string {
	t.Helper()
	name := map[ledger.Kind]func(ledger.Row) string{
		ledger.KindAssistantText: func(r ledger.Row) string { return "text " + bodyOf[ledger.AssistantText](t, r).MessageID },
		ledger.KindToolCall: func(r ledger.Row) string {
			c := bodyOf[ledger.ToolCall](t, r)
			return c.Phase + " " + c.ToolID
		},
		ledger.KindReasoning: func(r ledger.Row) string { return "thought " + bodyOf[ledger.Reasoning](t, r).MessageID },
		ledger.KindSubagent:  func(ledger.Row) string { return "subagent" },
		ledger.KindAsk:       func(ledger.Row) string { return "ask" },
	}
	var out []string
	for _, r := range rows {
		if f, ok := name[r.Kind]; ok {
			out = append(out, f(r))
		}
	}
	return out
}

func answer(messageID, text string) engineagents.CanonicalEvent {
	return delta(engineagents.HookMessageDelta, messageID, false, false, text)
}

func TestRecorder_TextStreamedBeforeACallIsRecordedAboveIt(t *testing.T) {
	f := newRecorderFixture(t, "hermes")
	f.dispatchOn(t, engineagents.ChannelAPI,
		engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"},
		delta(engineagents.HookReasoningDelta, "s1", false, false, "plan"),
		answer("s1", "Listing "), answer("s1", "files"),
		toolEvent(engineagents.HookToolPre, "t1", "ls"), toolEvent(engineagents.HookToolPost, "t1", "ls"),
		answer("s2", "Reading"),
		toolEvent(engineagents.HookToolPre, "t2", "cat"), toolEvent(engineagents.HookToolPost, "t2", "cat"),
		toolEvent(engineagents.HookToolPre, "t3", "wc"), toolEvent(engineagents.HookToolPost, "t3", "wc"),
		answer("s3", "50 lines"),
		engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, Message: "50 lines"},
	)

	assert.Equal(t, []string{
		"text s1", "pre t1", "post t1", "text s2", "pre t2", "post t2", "pre t3", "post t3", "text s3", "thought s1",
	}, transcript(t, f.rows(t)))
	assert.Equal(t, []string{"Listing files", "Reading", "50 lines"}, said(f.rows(t)))
}

func TestRecorder_ACallLeavesOpenAStreamNotDeclaredToEndAtIt(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	f.dispatchOn(t, engineagents.ChannelAPI,
		engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "hi"},
		answer("m1", "hel"),
		toolEvent(engineagents.HookToolPre, "t1", "Bash"), toolEvent(engineagents.HookToolPost, "t1", "Bash"),
		engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, Message: "hello"},
	)

	assert.Equal(t, []string{"pre t1", "post t1", "text m1"}, transcript(t, f.rows(t)))
	assert.Equal(t, []string{"hello"}, said(f.rows(t)))
}

func TestRecorder_ACallBetweenNumberedChunksRecordsTheMessageOnce(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	first := delta(engineagents.HookMessageDelta, "m1", true, false, "Running ")
	last := delta(engineagents.HookMessageDelta, "m1", true, true, "ls")
	last.Delta.Index = 1
	f.dispatchOn(t, engineagents.ChannelHooks,
		engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"},
		first, toolEvent(engineagents.HookToolPre, "t1", "ls"), last, toolEvent(engineagents.HookToolPost, "t1", "ls"),
		engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, Message: "Running ls"},
	)

	assert.Equal(t, []string{"text m1", "pre t1", "text m1", "post t1", "text m1"}, transcript(t, f.rows(t)))
	assert.Equal(t, []string{"Running ls"}, said(f.rows(t)))
}

func TestRecorder_TextStreamedBeforeASubagentOrAnAskIsRecordedAboveIt(t *testing.T) {
	f := newRecorderFixture(t, "hermes")
	f.dispatchOn(t, engineagents.ChannelAPI,
		engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"},
		answer("m1", "Delegating"),
		engineagents.CanonicalEvent{Kind: engineagents.HookSubagentPre, Subagent: &engineagents.SubagentEvent{ID: "A"}},
		answer("m2", "May I?"),
		permission(engineagents.ChoicePrompt{Kind: "tool_permission", ToolName: "Bash"}),
	)

	assert.Equal(t, []string{"text m1", "subagent", "text m2", "ask"}, transcript(t, f.rows(t)))
}

func TestRecorder_AReplyThatNeverStreamedIsRecordedAfterTheCall(t *testing.T) {
	for name, tc := range map[string]struct {
		reply string
		said  []string
	}{
		"new words":         {"Done", []string{"Listing", "Done"}},
		"the words it said": {"Listing", []string{"Listing"}},
		"more of them":      {"Listing all", []string{"Listing", "all"}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newRecorderFixture(t, "hermes")
			f.dispatchOn(t, engineagents.ChannelAPI,
				engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"},
				answer("s1", "Listing"),
				toolEvent(engineagents.HookToolPre, "t1", "ls"), toolEvent(engineagents.HookToolPost, "t1", "ls"),
				engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, Message: tc.reply},
			)

			rows := f.rows(t)
			assert.Equal(t, tc.said, said(rows))
			assert.Equal(t, []string{"text s1", "pre t1", "post t1"}, transcript(t, rows)[:3])
		})
	}
}

func TestRecorder_TextAfterAnEarlyCallStillJoinsItsMessage(t *testing.T) {
	for name, tc := range map[string]struct {
		then []engineagents.CanonicalEvent
		said []string
	}{
		"at the next call": {
			then: []engineagents.CanonicalEvent{
				toolEvent(engineagents.HookToolPre, "t1", "ls"), toolEvent(engineagents.HookToolPost, "t1", "ls"),
				answer("s2", "Done"),
				{Kind: engineagents.HookTurnStop, Message: "Done"},
			},
			said: []string{"Listing files", "Done"},
		},
		"at the turn's end": {
			then: []engineagents.CanonicalEvent{{Kind: engineagents.HookTurnStop}},
			said: []string{"Listing files"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newRecorderFixture(t, "hermes")
			f.dispatchOn(t, engineagents.ChannelAPI,
				engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"},
				answer("s1", "Listing "), toolEvent(engineagents.HookToolPre, "c1", "child"), answer("s1", "files"),
			)
			f.dispatchOn(t, engineagents.ChannelAPI, tc.then...)

			assert.Equal(t, tc.said, said(f.rows(t)))
		})
	}
}
