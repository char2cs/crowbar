package runner

import (
	"strings"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// resumeInjectionSteps turns a native session id into the argv/inject steps
// that ask a freshly-spawned CLI to resume it — used by both a prompt's
// restart-to-deliver path (prompts.go) and a provider switch (switch.go).
func resumeInjectionSteps(d engineagents.Agent, sessionID string) []engineagents.InjectStep {
	arg, ok := d.ResumeArg()
	if sessionID == "" || !ok {
		return nil
	}
	return sessionArgSteps(arg, sessionID)
}

// sessionArgSteps splits a session arg template ("<flag> {id}") into one
// pass_arg per token: exec.Command never splits a string on whitespace.
func sessionArgSteps(arg, sessionID string) []engineagents.InjectStep {
	ctx := engineagents.TemplateCtx{ID: sessionID}
	parts := strings.Fields(engineagents.Expand(arg, ctx))
	steps := make([]engineagents.InjectStep, 0, len(parts))
	for _, part := range parts {
		steps = append(steps, engineagents.InjectStep{
			Verb: "pass_arg",
			Args: map[string]any{"positional": part},
		})
	}
	return steps
}
