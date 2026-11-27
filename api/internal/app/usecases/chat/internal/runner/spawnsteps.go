package runner

import (
	"fmt"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func buildSpawnSteps(
	descriptor engineagents.Agent,
	resuming, inject bool,
	sel engineagents.Selection,
	resumeSteps, finalSteps []engineagents.InjectStep,
) []engineagents.InjectStep {
	steps := append([]engineagents.InjectStep{}, resumeSteps...)
	steps = append(steps, descriptor.SelectionSteps(sel)...)
	if !inject {
		return append(steps, finalSteps...)
	}
	context := descriptor.ContextSteps(resuming)
	if merged, ok := mergeLeadingPositional(context, finalSteps); ok {
		return append(steps, merged...)
	}
	steps = append(steps, context...)
	return append(steps, finalSteps...)
}

func mergeLeadingPositional(
	context, final []engineagents.InjectStep,
) ([]engineagents.InjectStep, bool) {
	if len(context) != 1 || !isBarePositional(context[0]) {
		return nil, false
	}
	if len(final) == 0 || !isBarePositional(final[len(final)-1]) {
		return nil, false
	}
	merged := append([]engineagents.InjectStep{}, final...)
	last := merged[len(merged)-1]
	merged[len(merged)-1] = engineagents.InjectStep{
		Verb: last.Verb,
		Args: map[string]any{
			"positional": argString(context[0].Args["positional"]) + "\n\n" + argString(last.Args["positional"]),
		},
	}
	return merged, true
}

func isBarePositional(step engineagents.InjectStep) bool {
	if step.Verb != "pass_arg" {
		return false
	}
	_, hasPositional := step.Args["positional"]
	_, hasArg := step.Args["arg"]
	return hasPositional && !hasArg
}

func argString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func resumeContextFor(resuming, inject bool, tctx engineagents.TemplateCtx) string {
	if resuming && inject {
		return tctx.Context
	}
	return ""
}
