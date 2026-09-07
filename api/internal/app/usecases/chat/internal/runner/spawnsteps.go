package runner

import (
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// buildSpawnSteps assembles the ordered InjectStep list a spawn's SpawnPlan
// renders against: extraSteps first, then the descriptor's own selection and
// context steps, then finalSteps — positional user prompts are final by
// contract (Claude's variadic --mcp-config must already be terminated by
// later options, and codex's resume subcommand/id must precede the message).
//
// descriptor.SelectionSteps contributes an EMPTY slice for a chat with no
// model/effort choice, or a provider declaring no such block — so this costs
// nothing on a spawn not using the feature, and the argv is byte-identical to
// one rendered before it existed.
func buildSpawnSteps(
	descriptor engineagents.Agent,
	resuming, inject bool,
	sel engineagents.Selection,
	extraSteps, finalSteps []engineagents.InjectStep,
) []engineagents.InjectStep {
	steps := append([]engineagents.InjectStep{}, extraSteps...)
	steps = append(steps, descriptor.SelectionSteps(sel)...)
	if contextStepsAllowed(resuming, inject, descriptor) {
		steps = append(steps, descriptor.ContextSteps(resuming)...)
	}
	return append(steps, finalSteps...)
}

// contextStepsAllowed is whether ContextSteps — a CLI argv, a POSITIONAL
// PROMPT on the resume path — may be rendered at all. False exactly when the
// redundant hooks-only PTY this same spawn's applyAPITransport call has
// already resumed over the api connection would otherwise answer it as its
// own genuine first turn (a provider whose only resume channel is a user
// message, e.g. codex — see apiOwnsResume). Never suppressed for a FRESH
// inject: an unresumed spawn's ContextSteps is silent config, nothing for the
// PTY to act on. resumeContextFor, just below, is this same routing decision
// for the OTHER channel — InjectAt over the api connection itself.
func contextStepsAllowed(resuming, inject bool, descriptor engineagents.Agent) bool {
	return inject && (!resuming || !apiOwnsResume(descriptor))
}

// resumeContextFor is the gap document a resumed api-transport connection's
// applyAPITransport call hands to InjectAt("context") — see that function's own
// comment for why this rides a separate channel from EstablishSession's own
// "context" value. Empty whenever inject's own gate says there is nothing to
// hand over, or this spawn isn't a resume at all: a fresh establish already
// carries tctx.Context as thread/start's developerInstructions.
func resumeContextFor(resuming, inject bool, tctx engineagents.TemplateCtx) string {
	if resuming && inject {
		return tctx.Context
	}
	return ""
}
