package def_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/flow/def"
	"github.com/stretchr/testify/assert"
)

func TestFlowToolConstants(t *testing.T) {
	assert.Equal(t, def.FlowTool("crowbar.signal"), def.ToolCrowbarSignal)
	assert.Equal(t, def.FlowTool("crowbar.create_item"), def.ToolCrowbarCreateItem)
	assert.Equal(t, def.FlowTool("crowbar.update_item_status"), def.ToolCrowbarUpdateItemStatus)
	assert.Equal(t, def.FlowTool("crowbar.get_items"), def.ToolCrowbarGetItems)
	assert.Equal(t, def.FlowTool("crowbar.open_thread"), def.ToolCrowbarOpenThread)
	assert.Equal(t, def.FlowTool("crowbar.reply_thread"), def.ToolCrowbarReplyThread)
	assert.Equal(t, def.FlowTool("crowbar.get_threads"), def.ToolCrowbarGetThreads)
	assert.Equal(t, def.FlowTool("crowbar.resolve_thread"), def.ToolCrowbarResolveThread)
	assert.Equal(t, def.FlowTool("fs.read"), def.ToolFSRead)
	assert.Equal(t, def.FlowTool("fs.write"), def.ToolFSWrite)
	assert.Equal(t, def.FlowTool("terminal"), def.ToolTerminal)
}

func TestUIModeConstants(t *testing.T) {
	assert.Equal(t, def.UIMode("chat"), def.UIModeChat)
	assert.Equal(t, def.UIMode("kanban"), def.UIModeKanban)
	assert.Equal(t, def.UIMode("diff"), def.UIModeDiff)
	assert.Equal(t, def.UIMode("background"), def.UIModeBackground)
}

func TestIntelligenceLevelConstants(t *testing.T) {
	assert.Equal(t, def.IntelligenceLevel("low"), def.IntelligenceLow)
	assert.Equal(t, def.IntelligenceLevel("medium"), def.IntelligenceMedium)
	assert.Equal(t, def.IntelligenceLevel("high"), def.IntelligenceHigh)
	assert.Equal(t, def.IntelligenceLevel("ultrahigh"), def.IntelligenceUltrahigh)
}

func TestAgentDefStruct(t *testing.T) {
	a := def.AgentDef{
		Intelligence: def.IntelligenceHigh,
		Tools:        []def.FlowTool{def.ToolFSRead},
		SystemPrompt: "do stuff",
	}
	assert.Equal(t, def.IntelligenceHigh, a.Intelligence)
	assert.Equal(t, []def.FlowTool{def.ToolFSRead}, a.Tools)
	assert.Equal(t, "do stuff", a.SystemPrompt)
}

func TestTransitionDefStruct(t *testing.T) {
	tr := def.TransitionDef{To: "next", On: "event"}
	assert.Equal(t, "next", tr.To)
	assert.Equal(t, "event", tr.On)
}

func TestEmitDefStruct(t *testing.T) {
	e := def.EmitDef{Agent: "improver", On: "review_passed"}
	assert.Equal(t, "improver", e.Agent)
	assert.Equal(t, "review_passed", e.On)
}

func TestStateDefinitionStruct(t *testing.T) {
	s := def.StateDefinition{
		Name:     "mystate",
		Terminal: false,
		UI:       def.UIModeChat,
		Items:    true,
		Agent:    nil,
		Transitions: []def.TransitionDef{
			{To: "other", On: "go"},
		},
		Emits: []def.EmitDef{
			{Agent: "bot", On: ""},
		},
	}
	assert.Equal(t, "mystate", s.Name)
	assert.False(t, s.Terminal)
	assert.Equal(t, def.UIModeChat, s.UI)
	assert.True(t, s.Items)
	assert.Nil(t, s.Agent)
	assert.Len(t, s.Transitions, 1)
	assert.Len(t, s.Emits, 1)
}

func TestFlowDefinitionStruct(t *testing.T) {
	fd := def.FlowDefinition{
		Name:         "test-flow",
		Version:      "2.0",
		Description:  "desc",
		ItemStatuses: []string{"todo", "done"},
		States: []def.StateDefinition{
			{Name: "start", Terminal: true},
		},
	}
	assert.Equal(t, "test-flow", fd.Name)
	assert.Equal(t, "2.0", fd.Version)
	assert.Equal(t, "desc", fd.Description)
	assert.Equal(t, []string{"todo", "done"}, fd.ItemStatuses)
	assert.Len(t, fd.States, 1)
}
