package ingress

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestMap_UnsequencedDeltaWritesNoRow(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("go")

	rows, fx := c.mapped(delta(engineagents.HookMessageDelta, "m1", false, false, "hel"), hooksSource())

	assert.Empty(t, rows)
	require.NotNil(t, fx.Live)
	assert.Equal(t, LiveDelta{TurnID: turnID, ID: "m1", Text: "hel"}, *fx.Live)
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
// A subagent's words are never the chat's reply.
func TestMap_AChunkAfterTheTurnEndedJoinsItsReply(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("go")
	inA := &engineagents.SubagentEvent{ID: "A"}
	c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookSubagentPre, Subagent: inA}, hooksSource())
	c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, Message: "the answer"}, hooksSource())

	rows, _ := c.mapped(delta(engineagents.HookMessageDelta, "m1", true, true, "the answer"), hooksSource())

	require.Len(t, rows, 1)
	assert.Equal(t, ledger.ReplyID(turnID), bodyOf[ledger.AssistantText](t, rows[0]).Reply)
	sub := delta(engineagents.HookMessageDelta, "s1", true, true, "the answer")
	sub.Subagent = inA
	rows, _ = c.mapped(sub, hooksSource())
	require.Len(t, rows, 1)
	assert.Empty(t, bodyOf[ledger.AssistantText](t, rows[0]).Reply)
}

// A chunk whose message began earlier unseen joins the reply whatever it says.
func TestMap_AJoinedMessageJoinsItsReply(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("go")
	c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, Message: "the answer"}, hooksSource())
	mid := delta(engineagents.HookMessageDelta, "m1", true, false, "unrelated")
	mid.Delta.Index = 1

	rows, _ := c.mapped(mid, hooksSource())
	require.Len(t, rows, 1)
	assert.Equal(t, ledger.ReplyID(turnID), bodyOf[ledger.AssistantText](t, rows[0]).Reply)
}

// A message keeps the turn it began in; a new message naming no turn is the
// running turn's.
func TestMap_AMessageStaysInTheTurnItBeganIn(t *testing.T) {
	c := liveChatRows(t)
	first := c.typed("one")
	c.mapped(delta(engineagents.HookMessageDelta, "m1", true, false, "hel"), hooksSource())
	s := hooksSource()
	s.Prompt = Prompt{Text: "two", Origin: ledger.OriginProvider}
	c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, TurnID: "pt-2"}, s)
	second, ok := c.st.RunningTurn()
	require.True(t, ok)

	late := delta(engineagents.HookMessageDelta, "m1", true, true, "lo")
	late.Delta.Index = 1
	rows, _ := c.mapped(late, hooksSource())
	require.Len(t, rows, 1)
	assert.Equal(t, first, rows[0].TurnID)

	rows, _ = c.mapped(delta(engineagents.HookMessageDelta, "m2", true, false, "new"), hooksSource())
	require.Len(t, rows, 1)
	assert.Equal(t, second.ID, rows[0].TurnID)
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
	empty, _ := c.mapped(engineagents.CanonicalEvent{
		Kind: engineagents.HookMessageComplete, Complete: &engineagents.MessageComplete{MessageID: "m2"},
	}, hooksSource())
	assert.Empty(t, empty, "a message that says nothing writes no row")
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

func TestMap_AToolRowCarriesItsTitleInputExitCodeAndInterrupt(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")
	pre := toolEvent(engineagents.HookToolPre, "t1", "Bash")
	pre.Tool.Kind, pre.Tool.Title, pre.Tool.Input = "execute", "List the files", []byte(`{"command":"ls"}`)
	post := toolEvent(engineagents.HookToolPost, "t1", "Bash")
	post.Tool.ExitCode, post.Tool.Input = new(0), pre.Tool.Input
	stopped := toolEvent(engineagents.HookToolFail, "t2", "Bash")
	stopped.Tool.ExitCode, stopped.Tool.Interrupted = new(130), true

	rows, _ := c.mapped(pre, hooksSource())
	started := bodyOf[ledger.ToolCall](t, rows[0])
	rows, _ = c.mapped(post, hooksSource())
	ran := bodyOf[ledger.ToolCall](t, rows[0])
	rows, _ = c.mapped(stopped, hooksSource())
	failed := bodyOf[ledger.ToolCall](t, rows[0])

	assert.Equal(t, "List the files", started.Title)
	assert.Equal(t, ledger.InputKey([]byte(`{"command":"ls"}`)), started.InputKey)
	assert.Empty(t, ran.InputKey, "a call is keyed once, by its first phase")
	assert.Nil(t, started.ExitCode, "a call still running has no exit code")
	require.NotNil(t, ran.ExitCode)
	assert.Equal(t, 0, *ran.ExitCode, "a zero exit is an answer")
	assert.False(t, ran.Interrupted)
	require.NotNil(t, failed.ExitCode)
	assert.Equal(t, 130, *failed.ExitCode)
	assert.True(t, failed.Interrupted)
}

func TestMap_ACallNamesAFileInTheSessionsDirectoryFromThere(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")
	pre := toolEvent(engineagents.HookToolPre, "t1", "Read")
	pre.Tool.Cwd = "/ws/repo"
	pre.Tool.Locations = []engineagents.ToolLocation{{Path: "/ws/repo/src/a.go", Line: 3}, {Path: "/etc/hosts"}}

	rows, _ := c.mapped(pre, hooksSource())

	want := []ledger.ToolLocation{{Path: "src/a.go", Line: 3}, {Path: "/etc/hosts"}}
	assert.Equal(t, want, bodyOf[ledger.ToolCall](t, rows[0]).Locations)
}

func TestMap_AFinishedCallCarriesItsOutputOnlyWhenItIsShort(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")
	short := toolEvent(engineagents.HookToolPost, "t1", "Bash")
	short.Tool.Output = "one\ntwo\n"
	long := toolEvent(engineagents.HookToolPost, "t2", "Bash")
	long.Tool.Output = strings.Repeat("line\n", 6)

	rows, _ := c.mapped(short, hooksSource())
	assert.Equal(t, "one\ntwo", bodyOf[ledger.ToolCall](t, rows[0]).Output)
	rows, _ = c.mapped(long, hooksSource())
	assert.Empty(t, bodyOf[ledger.ToolCall](t, rows[0]).Output, "a long output stays in its blob")
}

// Only a call of the MCP kind names an MCP server and tool: the server is
// optional, and a tool name is given to other kinds of call too.
func TestMap_OnlyACallIntoAnMCPServerNamesOne(t *testing.T) {
	for name, tc := range map[string]struct {
		kind, server, tool  string
		wantServer, wantMCP string
	}{
		"with its server":           {ledger.ToolKindMCP, "echo", "shout", "echo", "shout"},
		"on a channel naming none":  {ledger.ToolKindMCP, "", "shout", "", "shout"},
		"another kind naming tools": {"subagent", "", "spawnAgent", "", ""},
	} {
		t.Run(name, func(t *testing.T) {
			c := liveChatRows(t)
			c.typed("go")
			ev := toolEvent(engineagents.HookToolPre, "t1", "call")
			ev.Tool.Kind, ev.Tool.MCPServer, ev.Tool.MCPTool = tc.kind, tc.server, tc.tool

			rows, _ := c.mapped(ev, hooksSource())

			body := bodyOf[ledger.ToolCall](t, rows[0])
			assert.Equal(t, tc.wantServer, body.MCPServer)
			assert.Equal(t, tc.wantMCP, body.MCPTool)
		})
	}
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
	assert.Equal(t, ledger.SubagentBody{Phase: ledger.SubagentStart, Type: "explorer"}, bodyOf[ledger.SubagentBody](t, rows[0]))
	again, _ := c.mapped(start, hooksSource())
	assert.Empty(t, again)
	rows, _ = c.mapped(stop, hooksSource())
	assert.Equal(t, ledger.SubagentStop, bodyOf[ledger.SubagentBody](t, rows[0]).Phase)
	none, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookSubagentPre}, hooksSource())
	assert.Empty(t, none)
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

// recordedRows are the rows of a feed captured off a real daemon run, one
// `<time> <frame>` per line (testdata/live3: real claude and codex CLIs).
func recordedRows(t *testing.T, name string) []ledger.Row {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "live3", name))
	require.NoError(t, err)
	var rows []ledger.Row
	for line := range strings.Lines(string(raw)) {
		_, frame, _ := strings.Cut(line, " ")
		var f struct {
			Type string     `json:"type"`
			Row  ledger.Row `json:"row"`
		}
		if json.Unmarshal([]byte(frame), &f) == nil && f.Type == "row" {
			rows = append(rows, f.Row)
		}
	}
	return rows
}

// replay re-decides a recording's provider reports through Map: each whole
// message, and each provider's turn end with what the recording wrote at it
// (its reply, streamed answers, thought). Other rows are committed as recorded.
func replay(t *testing.T, recorded []ledger.Row) []ledger.Row {
	t.Helper()
	c := newChatRows(t)
	var out, spoken []ledger.Row
	decide := func(ev engineagents.CanonicalEvent, src Source) {
		rows, _, err := Map(c.st, ev, src, BlobRefs{})
		require.NoError(t, err)
		c.commit(rows...)
		out, spoken = append(out, rows...), nil
	}
	for _, r := range recorded {
		turn, _ := c.st.LastTurn()
		text, _ := ledger.DecodeBody[ledger.AssistantText](r)
		end, _ := ledger.DecodeBody[ledger.TurnEnded](r)
		switch {
		case r.Kind == ledger.KindAssistantText && text.Index == ledger.IndexComplete && text.Phase != "":
			decide(engineagents.CanonicalEvent{Kind: engineagents.HookMessageComplete, Complete: &engineagents.MessageComplete{
				TurnID: turn.ProviderTurnID, MessageID: text.MessageID, Text: text.Text, Phase: text.Phase,
			}}, Source{RunnerID: turn.RunnerID})
		case r.Kind == ledger.KindReasoning || r.Kind == ledger.KindAssistantText && text.Index == ledger.IndexComplete:
			spoken = append(spoken, r)
		case r.Kind == ledger.KindTurnEnded && (end.Reason == "" || end.Reason == ledger.ReasonProviderIdle):
			decide(recordedEnd(t, c.st, r.TurnID, end, spoken))
		default:
			c.commit(append(spoken, r)...)
			out, spoken = append(out, append(spoken, r)...), nil
		}
	}
	return out
}

// recordedEnd is the provider's end of turnID as the rows written at it say.
func recordedEnd(t *testing.T, st ledger.State, turnID string, end ledger.TurnEnded, spoken []ledger.Row) (
	engineagents.CanonicalEvent, Source,
) {
	t.Helper()
	turn, _ := st.LastTurn()
	last, _ := st.LastMessage(turnID)
	status := map[string]string{"done": "completed", "interrupted": "interrupted"}[end.Status]
	ev := engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, TurnID: turn.ProviderTurnID, Effort: end.Effort, TurnStatus: status}
	if end.Reason == ledger.ReasonProviderIdle {
		ev = engineagents.CanonicalEvent{Kind: engineagents.HookIdle}
	}
	src := Source{RunnerID: end.RunnerID}
	for _, r := range spoken {
		text := bodyOf[ledger.AssistantText](t, r)
		switch {
		case r.Kind == ledger.KindReasoning:
			src.Live.Reasoning = append(src.Live.Reasoning, liveText{TurnID: turnID, Text: text.Text})
		case text.MessageID == last || text.MessageID == ledger.ReplyID(turnID):
			ev.Message = text.Text
		default:
			src.Live.Answers = append(src.Live.Answers, liveText{TurnID: turnID, ID: text.MessageID, Text: text.Text})
		}
	}
	return ev, src
}
