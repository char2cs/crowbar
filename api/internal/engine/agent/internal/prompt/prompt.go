// Package prompt assembles system prompts for ACP agent sessions.
package prompt

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/flow"
)

// AgentRunOutput holds the output from a completed prior state.
type AgentRunOutput struct {
	StateName string
	Output    string
}

// Build assembles the session prompt from a state definition, the task title,
// and prior state outputs.
//
// The task title tells the agent which feature it is working on — without it
// the agent has no concrete goal. The prior-outputs section is omitted when
// priorOutputs is empty.
func Build(
	state flow.StateDefinition,
	taskTitle string,
	priorOutputs []AgentRunOutput,
) string {
	if state.Agent == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString(state.Agent.SystemPrompt)

	if taskTitle != "" {
		b.WriteString("\n\n--- Task ---\n")
		b.WriteString(taskTitle)
		b.WriteString("\n")
	}

	// In an autonomous run there is no human available to answer clarifying
	// questions. Instruct the agent to make reasonable assumptions and drive
	// the state to completion by calling crowbar_signal, rather than ending its
	// turn waiting for a reply that will never come.
	b.WriteString("\n--- Operating instructions ---\n")
	b.WriteString("You are running autonomously. No human will reply to questions. " +
		"Make reasonable assumptions, complete the work for this state, and then " +
		"call the crowbar_signal tool with the appropriate event to advance the " +
		"flow. Do not end your turn until you have called crowbar_signal.\n")

	if len(priorOutputs) > 0 {
		b.WriteString("\n--- Prior step outputs ---\n")
		for _, o := range priorOutputs {
			fmt.Fprintf(&b, "[%s] %s\n", o.StateName, o.Output)
		}
	}

	return b.String()
}
