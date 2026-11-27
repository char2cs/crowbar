package runner

import (
	"context"
	"fmt"

	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (rs *Runners) apiConnIsTheRunner(req forkRequest, attachArgv []string) bool {
	return len(attachArgv) == 0 && rs.HasLiveAPIConnection(req.runnerID)
}

func carriedSelection(
	sel engineagents.Selection,
	steps func(engineagents.Selection) []engineagents.InjectStep,
) engineagents.Selection {
	carried := engineagents.Selection{PermissionLevel: sel.PermissionLevel}
	if sel.Model != "" && len(steps(engineagents.Selection{Model: sel.Model})) > 0 {
		carried.Model = sel.Model
	}
	if sel.Effort != "" && len(steps(engineagents.Selection{Effort: sel.Effort})) > 0 {
		carried.Effort = sel.Effort
	}
	return carried
}

func (rs *Runners) forkOrAdopt(
	ctx context.Context, req forkRequest, attachArgv []string,
) (string, engineagents.Selection, error) {
	if rs.apiConnIsTheRunner(req, attachArgv) {
		return "", req.apiSelection, rs.adoptAPIConn(ctx, req)
	}
	termSessID, err := rs.forkCLI(ctx, req)
	if err != nil || len(attachArgv) == 0 {
		return termSessID, req.argvSelection, err
	}
	if err := rs.carryPromptOverAPIConn(ctx, req); err != nil {
		return "", req.apiSelection, rs.teardownForked(ctx, req.chatID, req.runnerID, termSessID, err)
	}
	return termSessID, req.apiSelection, nil
}

func (rs *Runners) adoptAPIConn(ctx context.Context, req forkRequest) error {
	//nolint:contextcheck // the exit outlives the request; its values must not reach it.
	if !rs.watchAPIConnExit(req.runnerID, req.crowbarHome, req.tmpDir) {
		rs.abandonUnforked(ctx, req.runnerID, req.paths())
		return fmt.Errorf("agent: spawn runner: api connection vanished before its exit could be watched")
	}
	if err := rs.carryPromptOverAPIConn(ctx, req); err != nil {
		rs.abandonUnforked(ctx, req.runnerID, req.paths())
		return err
	}
	return nil
}

func (rs *Runners) carryPromptOverAPIConn(ctx context.Context, req forkRequest) error {
	if req.promptMessage == "" {
		return nil
	}
	conn, ok := rs.apiConns.get(req.runnerID)
	if !ok {
		return ErrSpawnPromptUndeliverable
	}
	if _, _, err := rs.pushPromptOverAPI(
		ctx, req.runnerID, conn.tctx.Session, conn.tctx.Cwd, req.promptMessage,
	); err != nil {
		carrier := engineagents.Runner{ID: req.runnerID, ProviderID: req.providerID}
		rs.loseAPISession(ctx, req.chatID, carrier, conn.tctx.Session, err)
		return fmt.Errorf("%w: %w", ErrSpawnPromptUndeliverable, err)
	}
	return nil
}

func (rs *Runners) handOverAPIConn(runnerID string) {
	if c, ok := rs.apiConns.get(runnerID); ok {
		c.detached.Store(true)
	}
}

func (rs *Runners) runnerHasAnotherProcess(runnerID string) bool {
	if rs.ShowingNativeView(runnerID) || rs.HasLiveAPIConnection(runnerID) {
		return true
	}
	runner, ok := rs.live.byRunner(runnerID)
	return ok && runner.TerminalSession != ""
}

func (rs *Runners) exitProcesslessRunner(ctx context.Context, runnerID string) {
	ctx = context.WithoutCancel(ctx)
	if rs.runnerHasAnotherProcess(runnerID) {
		return
	}
	rs.reconcileRunnerExit(ctx, runnerID)
}

func (rs *Runners) watchAPIConnExit(runnerID, home, tmpDir string) bool {
	onExit := rs.onRunnerExit(home, runnerID, tmpDir, resumeWatch{})
	return rs.apiConns.watchExit(runnerID, func(lost string) {
		if lost != "" {
			rs.live.noteCause(runnerID, lost)
		}
		onExit(engineterminal.Exit{})
	})
}

func (r *apiConnRegistry) watchExit(runnerID string, onExit func(lost string)) bool {
	c, ok := r.get(runnerID)
	if !ok || c.ended() == nil {
		return false
	}
	go func() {
		<-c.ended()
		if c.detached.Load() {
			return
		}
		onExit(c.exitCause())
	}()
	return true
}

func (c *apiconn) ended() <-chan struct{} {
	if c.serve != nil {
		return c.serve.exited
	}
	return c.gone
}
