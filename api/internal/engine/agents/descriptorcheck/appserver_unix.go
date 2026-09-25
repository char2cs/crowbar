//go:build unix

package descriptorcheck

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// appServer starts runtime.api.serve and runs the handshake the runner runs.
func (l *live) appServer(ctx context.Context) Step {
	if l.d.Runtime.Transport != string(spec.ChannelAPI) {
		return skip("app_server", "not an api-transport provider")
	}
	tctx := l.templateCtx(filepath.Join(l.scratch, "serve"))
	tctx.Socket = filepath.Join(l.scratch, "s.sock")
	argv, ok := l.agent.APIServeArgv(tctx)
	if !ok {
		return skip("app_server", "the descriptor declares no runtime.api.serve")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // the descriptor's own serve argv, run on purpose
	cmd.Env, cmd.Dir = l.opts.Env, l.cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fail("app_server", "start: "+err.Error())
	}
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	defer func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-exited
	}()

	ctx, cancel := context.WithTimeout(ctx, l.opts.ServeWindow)
	defer cancel()
	if err := awaitSocket(ctx, tctx.Socket, exited); err != nil {
		return fail("app_server", err.Error())
	}
	conn, err := l.agent.StartAPIConn(ctx, tctx.Socket, nil)
	if err != nil {
		return fail("app_server", err.Error())
	}
	_ = conn.Close()
	return pass("app_server", "initialize answered over "+argv[0]+" "+argv[1])
}

func awaitSocket(ctx context.Context, path string, exited <-chan struct{}) error {
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		select {
		case <-exited:
			return errors.New("the app-server exited before opening its socket")
		case <-ctx.Done():
			return fmt.Errorf("the app-server never opened %s", path)
		case <-time.After(pollEvery):
		}
	}
}
