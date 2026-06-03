package translator

import "github.com/char2cs/crowbar/api/internal/engine/flow/def"

func mapFlow(raw rawFlow) def.FlowDefinition {
	states := make([]def.StateDefinition, 0, len(raw.States))
	for _, s := range raw.States {
		states = append(states, mapState(s))
	}
	return def.FlowDefinition{
		Name:         raw.Name,
		Version:      raw.Version,
		Description:  raw.Description,
		ItemStatuses: raw.ItemStatuses,
		States:       states,
	}
}

func mapState(raw rawState) def.StateDefinition {
	transitions := make([]def.TransitionDef, 0, len(raw.Transitions))
	for _, t := range raw.Transitions {
		transitions = append(transitions, def.TransitionDef{
			To: t.To,
			On: t.On,
		})
	}

	emits := make([]def.EmitDef, 0, len(raw.Emits))
	for _, e := range raw.Emits {
		emits = append(emits, def.EmitDef{
			Agent: e.Agent,
			On:    e.On,
		})
	}

	var agentDef *def.AgentDef
	if raw.Agent != nil {
		agentDef = mapAgent(raw.Agent)
	}

	return def.StateDefinition{
		Name:        raw.Name,
		Terminal:    raw.Terminal,
		UI:          def.UIMode(raw.UI),
		Items:       raw.Items,
		Agent:       agentDef,
		Transitions: transitions,
		Emits:       emits,
	}
}

func mapAgent(raw *rawAgent) *def.AgentDef {
	tools := make([]def.FlowTool, len(raw.Tools))
	copy(tools, raw.Tools)
	return &def.AgentDef{
		Intelligence: def.IntelligenceLevel(raw.Intelligence),
		Tools:        tools,
		SystemPrompt: raw.SystemPrompt,
	}
}
