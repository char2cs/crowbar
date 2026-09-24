package spawn

import (
	"fmt"
	"os"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/env"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spawn/internal/verbs"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/template"
)

var ErrForbiddenFlag = fmt.Errorf("agents: forbidden flag")

func Plan(
	d *spec.Descriptor,
	ctx models.TemplateCtx,
	baseEnv []string,
	extra []spec.InjectStep,
) (*models.SpawnPlan, error) {
	plan := &models.SpawnPlan{
		Executable: binpath.Resolve(d.Spawn.Cmd),
		Cwd:        ctx.Cwd,
		TmpDir:     ctx.Tmp,
		Env:        env.Clear(baseEnv, d.Spawn.Env.Clear),
		Cleanup:    func() { _ = os.RemoveAll(ctx.Tmp) },
	}
	PrependArgs(d, ctx, plan)

	if err := Inject(d, ctx, plan, extra); err != nil {
		plan.Cleanup()
		return nil, err
	}
	if err := checkForbidden(d, plan.Argv); err != nil {
		plan.Cleanup()
		return nil, err
	}
	return plan, nil
}

// PrependArgs puts d.Spawn.Args at the front of plan.Argv — the flags any
// interactive invocation of the provider's own executable needs (e.g.
// codex's --dangerously-bypass-hook-trust), regardless of which entry point
// is building the plan. Plan uses this for the primary spawn; the api
// package's attach path (an interactive TUI in every way that matters here)
// must use it too, or an attached CLI never gets flags the primary spawn
// always carries. The api package's serve path deliberately does NOT call
// this — it invokes a genuinely different, non-interactive subcommand that
// these flags don't apply to.
func PrependArgs(d *spec.Descriptor, ctx models.TemplateCtx, plan *models.SpawnPlan) {
	args := make([]string, len(d.Spawn.Args))
	for i, a := range d.Spawn.Args {
		args[i] = template.Expand(a, ctx)
	}
	plan.Argv = append(args, plan.Argv...)
}

// Inject applies a descriptor's MCPInject, ConfigInjection and HooksInjection
// steps, plus any caller-supplied extra, onto a PTY plan — in that order. A PTY
// reports over the hooks channel, so it is the one process that gets hooks.
func Inject(
	d *spec.Descriptor,
	ctx models.TemplateCtx,
	plan *models.SpawnPlan,
	extra []spec.InjectStep,
) error {
	return inject(d, ctx, plan, extra, true)
}

// InjectServe is Inject for an api-transport `serve` process: the same MCP
// server and session config, but no hook wiring — that process reports over
// the api channel, and one process must never report an event twice.
func InjectServe(
	d *spec.Descriptor,
	ctx models.TemplateCtx,
	plan *models.SpawnPlan,
	extra []spec.InjectStep,
) error {
	return inject(d, ctx, plan, extra, false)
}

func inject(
	d *spec.Descriptor,
	ctx models.TemplateCtx,
	plan *models.SpawnPlan,
	extra []spec.InjectStep,
	hooks bool,
) error {
	steps := make([]spec.InjectStep, 0, len(d.MCPInject)+len(d.ConfigInjection)+len(d.HooksInjection)+len(extra))
	steps = append(steps, d.MCPInject...)
	steps = append(steps, d.ConfigInjection...)
	if hooks {
		steps = append(steps, d.HooksInjection...)
	}
	steps = append(steps, extra...)

	for _, step := range steps {
		if err := verbs.Apply(step, ctx, plan); err != nil {
			return err
		}
	}
	return nil
}

func checkForbidden(d *spec.Descriptor, argv []string) error {
	optionsEnded := false
	for _, tok := range argv {
		if tok == "--" {
			optionsEnded = true
			continue
		}
		if optionsEnded {
			continue
		}
		for _, f := range d.Spawn.ForbidFlags {
			if tok == f {
				return fmt.Errorf("%w %q for provider %q", ErrForbiddenFlag, tok, d.ID)
			}
		}
	}
	return nil
}

func PromptSteps(d *spec.Descriptor, resume bool) ([]spec.InjectStep, bool) {
	if d == nil || d.Presentation.PromptSubmit == nil {
		return nil, false
	}
	steps := d.Presentation.PromptSubmit.Fresh
	if resume {
		steps = d.Presentation.PromptSubmit.Resume
	}
	return spec.CloneSteps(steps), true
}
