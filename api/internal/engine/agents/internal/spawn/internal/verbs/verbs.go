// Package verbs implements the descriptor injection verbs.
//
// One file per verb. A verb receives the step's already-parsed arguments, the
// expansion context and the plan being built, and may only add to that plan —
// there is no verb that can read a provider's files, run a command, or reach the
// network, and adding one would be a visible new file rather than another arm on
// a switch.
package verbs

import (
	"fmt"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/template"
)

// ErrUnknownVerb reports a descriptor asking for an injection the engine does not
// implement. It is an error rather than a skip: a silently ignored verb spawns a
// CLI missing the config the descriptor author believed it had.
var ErrUnknownVerb = fmt.Errorf("agents: unknown inject verb")

// Args resolves a step's arguments against the expansion context.
type Args struct {
	raw map[string]any
	ctx models.TemplateCtx
}

// String returns an expanded argument.
func (a Args) String(key string) string {
	return template.Expand(spec.ArgString(a.raw[key]), a.ctx)
}

// Present reports whether a key was supplied at all, which is different from it
// being supplied empty.
func (a Args) Present(key string) bool {
	_, ok := a.raw[key]
	return ok
}

// Handler applies one verb to the plan under construction.
type Handler func(args Args, plan *models.SpawnPlan) error

var registry = map[string]Handler{
	"set_env":    setEnv,
	"write_file": writeFile,
	"pass_arg":   passArg,
}

// Apply runs one injection step.
func Apply(step spec.InjectStep, ctx models.TemplateCtx, plan *models.SpawnPlan) error {
	handler, ok := registry[step.Verb]
	if !ok {
		return fmt.Errorf("%w %q", ErrUnknownVerb, step.Verb)
	}
	return handler(Args{raw: step.Args, ctx: ctx}, plan)
}
