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
	for _, a := range d.Spawn.Args {
		plan.Argv = append(plan.Argv, template.Expand(a, ctx))
	}

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

// Inject applies a descriptor's MCPInject and ConfigInjection steps, plus any
// caller-supplied extra, onto plan — in that order, always, regardless of
// which process plan describes.
//
// What a provider needs injected (its own MCP server, its settings/hooks
// file) is a fact about the PROVIDER, not about which of its processes is
// doing the talking right now — a hooks-attached CLI and an api-transport
// serve process are still the same provider, and both need the same crowbar
// MCP server registered to reach Crowbar's tools at all. Plan calls this for
// the former; the api-transport package calls it again, on a plan of its own,
// for the latter — same steps, same verbs, the only difference is which argv
// they land on.
func Inject(
	d *spec.Descriptor,
	ctx models.TemplateCtx,
	plan *models.SpawnPlan,
	extra []spec.InjectStep,
) error {
	steps := make([]spec.InjectStep, 0, len(d.MCPInject)+len(d.ConfigInjection)+len(extra))
	steps = append(steps, d.MCPInject...)
	steps = append(steps, d.ConfigInjection...)
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
