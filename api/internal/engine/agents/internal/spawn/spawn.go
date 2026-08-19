// Package spawn renders a descriptor into a concrete process launch.
package spawn

import (
	"fmt"
	"os"
	"strconv"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/env"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/rewake"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spawn/internal/verbs"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/template"
)

// ErrForbiddenFlag reports an assembled argv containing a flag the descriptor
// declared it must never carry — in practice, the flags that would make the CLI
// headless.
var ErrForbiddenFlag = fmt.Errorf("agents: forbidden flag")

// Plan renders spawn.args + mcp_injection + config_injection + extra into a
// concrete argv/env/cwd, writing any config files under ctx.Tmp.
//
// It renders whatever MCPInject the descriptor it is HANDED declares. Whether a
// provider's tool surface should be registered at all is a user preference, so
// that decision is the caller's and arrives here as a descriptor with the field
// already emptied — the engine has no access to a preference table and must not
// grow one.
func Plan(
	d *spec.Descriptor,
	ctx models.TemplateCtx,
	baseEnv []string,
	extra []spec.InjectStep,
) (*models.SpawnPlan, error) {
	// The rewake wrapper's two halves come from the descriptor being rendered, not
	// from the caller. See models.TemplateCtx.RewakeSentinel: the value registered
	// with the provider and the value matched when the prompt returns are one fact,
	// and this is the single line that makes them so.
	ctx.RewakeSentinel = rewake.Sentinel(d)
	ctx.RewakeSummary = rewake.Summary(d)
	ctx.RewakeWakeStatus = strconv.Itoa(rewake.WakeStatus(d))

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

	// mcp_injection BEFORE config_injection, so a descriptor can guarantee with
	// its own steps what follows its MCP registration. Claude's --mcp-config is
	// VARIADIC and swallows any bare positional after it; what stops that is the
	// --settings pair sitting immediately behind it in config_injection.
	// Rendering the MCP steps last would put the JSON one token away from a
	// resumed session's id.
	steps := make([]spec.InjectStep, 0, len(d.MCPInject)+len(d.ConfigInjection)+len(extra))
	steps = append(steps, d.MCPInject...)
	steps = append(steps, d.ConfigInjection...)
	steps = append(steps, extra...)

	for _, step := range steps {
		if err := verbs.Apply(step, ctx, plan); err != nil {
			plan.Cleanup()
			return nil, err
		}
	}
	if err := checkForbidden(d, plan.Argv); err != nil {
		plan.Cleanup()
		return nil, err
	}
	return plan, nil
}

// checkForbidden is the hard guard that the engine never spawns a headless CLI.
//
// It stops at an end-of-options `--`: everything after it is data, and a user
// whose prompt happens to be the exact text "--print" must not have their message
// mistaken for a flag.
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

// PromptSteps returns a defensive copy of the descriptor's argv steps for a fresh
// or resumed prompt submission.
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
