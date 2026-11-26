package agents

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/env"
	agentexec "github.com/char2cs/crowbar/api/internal/engine/agents/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spawn"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// connectOutputLimit bounds what a connect command may print: one small JSON object.
const connectOutputLimit = 64 << 10

func (a *agent) APIDialed() bool {
	d := a.spec.Runtime.API.Delivery
	return d == spec.APIDeliveryWebsocket || d == spec.APIDeliveryHTTP
}

// StartAPIEndpoint connects a websocket or http descriptor: runtime.api.connect
// (when declared) runs with the env a PTY spawn gets, and its stdout is parsed and
// never logged, as it carries the endpoint's credential. mcp_injection's files are
// written first, once per connection, so a fresh and a resumed session both find them.
func (a *agent) StartAPIEndpoint(ctx context.Context, tctx TemplateCtx, origin APISessionOrigin) (*APIConn, error) {
	if err := spawn.WriteMCPFiles(a.spec, tctx); err != nil {
		return nil, fmt.Errorf("agents: write the mcp files: %w", err)
	}
	ep := protocol.Endpoint{
		Argv: expandArgv(a.spec.Runtime.API.Connect, tctx),
		Env:  env.Clear(os.Environ(), a.spec.Spawn.Env.Clear),
		Run:  runConnect(tctx.Cwd),
	}
	return protocol.StartAPIEndpoint(ctx, a.spec, ep, origin)
}

func runConnect(cwd string) protocol.ConnectExec {
	return func(ctx context.Context, argv, environ []string) ([]byte, error) {
		if len(argv) == 0 {
			return nil, errors.New("agents: connect: no command")
		}
		return agentexec.New(agentexec.Options{
			Executable: agentexec.Executable(argv[0], environ), Cwd: cwd, Env: environ,
			MaxStdout: connectOutputLimit, MaxStderr: connectOutputLimit,
		}).Run(ctx, argv[1:])
	}
}
