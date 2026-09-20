package verbs

import (
	"fmt"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/template"
)

var ErrUnknownVerb = fmt.Errorf("agents: unknown inject verb")

type Args struct {
	raw map[string]any
	ctx models.TemplateCtx
}

func (a Args) String(key string) string {
	return template.Expand(spec.ArgString(a.raw[key]), a.ctx)
}

func (a Args) Present(key string) bool {
	_, ok := a.raw[key]
	return ok
}

type Handler func(args Args, plan *models.SpawnPlan) error

var registry = map[string]Handler{
	"set_env":    setEnv,
	"write_file": writeFile,
	"pass_arg":   passArg,
}

func Apply(step spec.InjectStep, ctx models.TemplateCtx, plan *models.SpawnPlan) error {
	handler, ok := registry[step.Verb]
	if !ok {
		return fmt.Errorf("%w %q", ErrUnknownVerb, step.Verb)
	}
	return handler(Args{raw: step.Args, ctx: ctx}, plan)
}
