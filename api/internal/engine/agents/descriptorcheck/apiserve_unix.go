//go:build unix

package descriptorcheck

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// apiServe starts runtime.api.serve and runs the handshake the runner runs:
// serve is ready only once it answers over its own stdio.
func (l *live) apiServe(ctx context.Context) Step {
	if l.d.Runtime.Transport != string(spec.ChannelAPI) {
		return skip("api_serve", "not an api-transport provider")
	}
	argv, ok := l.agent.APIServeArgv(l.templateCtx(filepath.Join(l.scratch, "serve")))
	if !ok {
		return skip("api_serve", "the descriptor declares no runtime.api.serve")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // the descriptor's own serve argv, run on purpose
	cmd.Env, cmd.Dir = l.opts.Env, l.cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdio, err := agents.StartServe(cmd)
	if err != nil {
		return fail("api_serve", "start: "+err.Error())
	}
	defer func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	}()

	ctx, cancel := context.WithTimeout(ctx, l.opts.ServeWindow)
	defer cancel()
	conn, err := l.agent.StartAPIConn(ctx, stdio, nil)
	if err != nil {
		return fail("api_serve", err.Error())
	}
	_ = conn.Close()
	return pass("api_serve", l.d.Runtime.API.Handshake["call"]+" answered over the stdio of "+strings.Join(argv, " "))
}
