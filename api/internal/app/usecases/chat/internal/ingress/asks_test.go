package ingress

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func permission(choice engineagents.ChoicePrompt) engineagents.CanonicalEvent {
	return engineagents.CanonicalEvent{Kind: engineagents.HookPermission, Choice: &choice}
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

// Held ⇔ answerable: wherever a held ask was raised, its CLI blocks on it.
func TestMap_AnAskIsOpenAndAnswerableExactlyWhenItIsHeld(t *testing.T) {
	ask := permission(engineagents.ChoicePrompt{Kind: "tool_permission", ToolName: "Bash"})
	inA := func(ev engineagents.CanonicalEvent) engineagents.CanonicalEvent {
		ev.Subagent = &engineagents.SubagentEvent{ID: "A"}
		return ev
	}
	outlived := func(t *testing.T) *chatRows {
		t.Helper()
		c := liveChatRows(t)
		c.typed("go")
		c.mapped(inA(engineagents.CanonicalEvent{Kind: engineagents.HookSubagentPre}), hooksSource())
		c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, TurnID: "pt-1"}, hooksSource())
		return c
	}
	gone := hooksSource()
	gone.RunnerID = "run-gone"
	unheld := hooksSource()
	unheld.Answerable = false
	for name, tc := range map[string]struct {
		chat func(*testing.T) *chatRows
		ev   engineagents.CanonicalEvent
		src  Source
		held bool
	}{
		"held outside a turn":         {chat: liveChatRows, ev: ask, src: hooksSource(), held: true},
		"held in an outlived scope":   {chat: outlived, ev: inA(ask), src: hooksSource(), held: true},
		"of a runner past its exit":   {chat: liveChatRows, ev: ask, src: gone},
		"unheld outside a turn":       {chat: liveChatRows, ev: ask, src: unheld},
		"unheld in an outlived scope": {chat: outlived, ev: inA(ask), src: unheld},
	} {
		t.Run(name, func(t *testing.T) {
			c := tc.chat(t)

			rows, fx := c.mapped(tc.ev, tc.src)

			a := bodyOf[ledger.Ask](t, rows[0])
			assert.Equal(t, tc.held, a.Answerable)
			assert.Equal(t, tc.held, fx.HoldAsk == a.AskID)
			assert.Equal(t, tc.held, len(c.st.PendingAsks()) == 1, "held, it stays open; unheld, nothing waits on it")
		})
	}
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

// Of two open calls of one name, the ask names the one given the same input,
// however each delivery spelled it; with no input, the newest.
func TestMap_AnAskBindsByToolNameAndCanonicalInput(t *testing.T) {
	for name, tc := range map[string]struct{ input, want string }{
		"the first call's input":  {`{ "timeout": 5, "command": "ls" }`, "t1"},
		"the second call's input": {`{"command":"pwd"}`, "t2"},
		"no input":                {"", "t2"},
	} {
		t.Run(name, func(t *testing.T) {
			c := liveChatRows(t)
			c.typed("go")
			for _, call := range []struct{ id, input string }{
				{"t1", `{"command":"ls","timeout":5}`}, {"t2", `{"command":"pwd"}`},
			} {
				ev := toolEvent(engineagents.HookToolPre, call.id, "Bash")
				ev.Tool.Input = []byte(call.input)
				c.mapped(ev, hooksSource())
			}

			rows, _ := c.mapped(permission(engineagents.ChoicePrompt{
				Kind: "tool_permission", ToolName: "Bash", ToolInput: []byte(tc.input),
			}), hooksSource())

			assert.Equal(t, tc.want, bodyOf[ledger.Ask](t, rows[0]).ToolID)
		})
	}
}

// An ask says what its call does; an input too big for the row keeps its
// leading text, marked as cut, never dropped.
func TestMap_AnAskKeepsItsCallsInputUpToABound(t *testing.T) {
	small := `{"file_path":"a.go"}`
	big := `{"content":"` + strings.Repeat("é", maxAskInput) + `"}`
	for name, tc := range map[string]struct {
		input string
		cut   bool
	}{"small": {small, false}, "too big": {big, true}} {
		t.Run(name, func(t *testing.T) {
			c := liveChatRows(t)
			c.typed("go")

			rows, _ := c.mapped(permission(engineagents.ChoicePrompt{
				Kind: "tool_permission", ToolName: "Write", ToolTarget: "a.go", MCPServer: "echo", ToolInput: []byte(tc.input),
			}), hooksSource())

			a := bodyOf[ledger.Ask](t, rows[0])
			assert.Equal(t, "a.go", a.ToolTarget)
			assert.Equal(t, "echo", a.MCPServer)
			assert.Equal(t, tc.cut, a.ToolInputTruncated)
			if !tc.cut {
				assert.JSONEq(t, small, string(a.ToolInput))
				return
			}
			var lead string
			require.NoError(t, json.Unmarshal(a.ToolInput, &lead), "a cut input is its leading text")
			assert.LessOrEqual(t, len(lead), maxAskInput)
			assert.True(t, utf8.ValidString(lead), "never cut inside a character")
			assert.True(t, strings.HasPrefix(big, lead))
		})
	}
}

// An api CLI is refused every ask Crowbar does not hold, so that is how the
// ask closes: never a pending card nobody can answer.
func TestMap_AnAPIAskCrowbarDoesNotHoldIsClosedAsRefused(t *testing.T) {
	api := func(edit func(*Source)) Source {
		s := hooksSource()
		s.Channel = engineagents.ChannelAPI
		edit(&s)
		return s
	}
	bash := permission(engineagents.ChoicePrompt{Kind: "tool_permission", ToolName: "Bash"})
	for name, tc := range map[string]struct {
		ev  engineagents.CanonicalEvent
		src Source
	}{
		"one it cannot hold":        {bash, api(func(s *Source) { s.Answerable = false })},
		"one offering no choice":    {engineagents.CanonicalEvent{Kind: engineagents.HookPermission}, api(func(*Source) {})},
		"of a runner past its exit": {bash, api(func(s *Source) { s.RunnerID = "run-gone" })},
	} {
		t.Run(name, func(t *testing.T) {
			c := liveChatRows(t)
			c.typed("go")

			rows, fx := c.mapped(tc.ev, tc.src)

			require.Equal(t, []ledger.Kind{ledger.KindAsk, ledger.KindAskResolved}, kinds(rows))
			assert.Equal(t, ledger.ResolutionRefused, bodyOf[ledger.AskResolved](t, rows[1]).Resolution)
			assert.Empty(t, fx.HoldAsk)
			assert.Empty(t, c.st.PendingAsks())
		})
	}
}
