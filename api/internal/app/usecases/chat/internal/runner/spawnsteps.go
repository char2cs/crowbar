package runner

import (
	"fmt"

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
//
// A bare positional ContextSteps and a bare positional finalSteps message are
// folded into ONE combined argv token rather than emitted as two — see
// mergeLeadingPositional's own doc for why.
func buildSpawnSteps(
	descriptor engineagents.Agent,
	resuming, inject bool,
	sel engineagents.Selection,
	extraSteps, finalSteps []engineagents.InjectStep,
) []engineagents.InjectStep {
	steps := append([]engineagents.InjectStep{}, extraSteps...)
	steps = append(steps, descriptor.SelectionSteps(sel)...)
	if contextStepsAllowed(resuming, inject, descriptor) {
		context := descriptor.ContextSteps(resuming)
		if merged, ok := mergeLeadingPositional(context, finalSteps); ok {
			finalSteps = merged
		} else {
			steps = append(steps, context...)
		}
	}
	return append(steps, finalSteps...)
}

// mergeLeadingPositional folds a single bare positional context step directly
// into a bare positional message step, when both are exactly that shape.
//
// Confirmed live: claude's switch-then-send resumes with the "while you were
// away" gap riding its own bare positional (resume_context_inject), and the
// user's actual message riding a SEPARATE positional right after its own "--"
// guard (prompt_submit.resume). A CLI built to take ONE positional query
// treats two distinct ones — even split across their own "--" guards — as
// unrelated, and answers only the first: the gap got acknowledged, the user's
// real question was silently dropped, never even reaching the CLI's own
// user_prompt hook. Folding them into ONE argv token ahead of the message's
// own "--" guard is the only shape a single-positional CLI can receive as one
// turn.
//
// Matched purely on InjectStep's generic Verb/Args shape — a bare
// "positional" pass_arg with no "arg" key — never on provider-specific
// string content, so any descriptor hitting this exact shape gets the same
// fix, not just claude's.
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

// argString mirrors spec.ArgString (unexported outside the engine package):
// an InjectStep's Args values are decoded from YAML as any, but every
// positional value used here is authored as a plain string.
func argString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
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
