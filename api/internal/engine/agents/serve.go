package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/selection"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spawn"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/template"
)

func StartServe(cmd *exec.Cmd) (APIStdio, error) {
	childIn, in, err := os.Pipe()
	if err != nil {
		return APIStdio{}, fmt.Errorf("agents: serve stdin: %w", err)
	}
	out, childOut, err := os.Pipe()
	if err != nil {
		return APIStdio{}, errors.Join(fmt.Errorf("agents: serve stdout: %w", err), childIn.Close(), in.Close())
	}
	cmd.Stdin, cmd.Stdout = childIn, childOut
	if err := cmd.Start(); err != nil {
		return APIStdio{}, errors.Join(fmt.Errorf("agents: start serve: %w", err),
			childIn.Close(), childOut.Close(), in.Close(), out.Close())
	}
	if err := errors.Join(childIn.Close(), childOut.Close()); err != nil {
		return APIStdio{}, errors.Join(fmt.Errorf("agents: serve pipes: %w", err),
			in.Close(), out.Close(), cmd.Process.Kill(), cmd.Wait())
	}
	return APIStdio{In: in, Out: out}, nil
}

var errAPITransportNotDeclared = errors.New("agents: provider does not declare an api transport")

func (a *agent) StartAPIConn(
	ctx context.Context, stdio APIStdio, origin APISessionOrigin,
) (*APIConn, error) {
	if a.spec.Runtime.Transport != "api" && !hasAPIEventOverride(a.spec) {
		return nil, errors.Join(errAPITransportNotDeclared, stdio.Close())
	}
	return protocol.StartAPIDriver(ctx, a.spec, stdio, origin)
}

func (a *agent) APIServeArgv(ctx TemplateCtx) ([]string, bool) {
	if len(a.spec.Runtime.API.Serve) == 0 {
		return nil, false
	}
	argv := expandArgv(a.spec.Runtime.API.Serve, ctx)
	plan := &SpawnPlan{Executable: argv[0], Argv: append([]string{}, argv[1:]...)}
	sel := Selection{Model: ctx.Model, Effort: ctx.Effort}
	if err := spawn.InjectServe(a.spec, ctx, plan, selection.APISteps(a.spec, sel)); err != nil {
		return nil, false
	}
	return append([]string{plan.Executable}, plan.Argv...), true
}

func (a *agent) APIAttachArgv(ctx TemplateCtx) ([]string, bool) {
	if len(a.spec.Runtime.API.Attach) == 0 {
		return nil, false
	}
	argv := expandArgv(a.spec.Runtime.API.Attach, ctx)
	plan := &SpawnPlan{Executable: argv[0], Argv: append([]string{}, argv[1:]...)}
	spawn.PrependArgs(a.spec, ctx, plan)
	sel := Selection{Model: ctx.Model, Effort: ctx.Effort, PermissionLevel: ctx.PermissionLevel}
	if err := spawn.Inject(a.spec, ctx, plan, selection.Steps(a.spec, sel)); err != nil {
		return nil, false
	}
	return append([]string{plan.Executable}, plan.Argv...), true
}

func expandArgv(argv []string, ctx TemplateCtx) []string {
	out := make([]string, len(argv))
	for i, a := range argv {
		out[i] = template.Expand(a, ctx)
	}
	return out
}

func hasAPIEventOverride(d *spec.Descriptor) bool {
	for _, e := range d.Events {
		if e.Transport == "api" {
			return true
		}
	}
	return false
}
