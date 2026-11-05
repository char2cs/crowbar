// Package runner (file apirunner.go) is the spawn shape where the api connection IS
// the whole runner: no PTY is forked, and the `serve` process carries the liveness a
// PTY normally would.
package runner

import (
	"context"
	"fmt"

	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// apiConnIsTheRunner reports whether runnerID's spawn has nothing left to fork: its
// api connection came up and no hotswap `attach` argv was rendered.
func (rs *Runners) apiConnIsTheRunner(req forkRequest, attachArgv []string) bool {
	return len(attachArgv) == 0 && rs.HasLiveAPIConnection(req.runnerID)
}

// carriedSelection is the part of sel a carrier rendering `steps` actually takes, per
// field. recordRunner stamps THIS, never the intent: a row claiming a model no carrier
// received would make selectionRequiresRestart compare equal forever.
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

// forkOrAdopt forks the PTY a spawn needs, or none when its connection is the
// whole runner, and returns which carrier took the selection: a PTY running the
// plan carries prompt and pick in argv, while a connection must dispatch them.
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
	// pointPlanAtAttach replaced the rendered plan wholesale: the connection
	// the PTY attaches to carries the selection and the prompt.
	if err := rs.carryPromptOverAPIConn(ctx, req); err != nil {
		return "", req.apiSelection, rs.teardownForked(ctx, req.chatID, req.runnerID, termSessID, err)
	}
	return termSessID, req.apiSelection, nil
}

// adoptAPIConn is forkCLI for a spawn with no PTY: the exit reconcile is armed on
// `serve`. Its events wait on the chat's lock, held until runner_started commits.
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

// carryPromptOverAPIConn pushes a spawn's prompt down the connection that is its
// only carrier. A failure fails the spawn, so the prompt is settled; a session
// gone under the push is lost, as a refused establish is.
func (rs *Runners) carryPromptOverAPIConn(ctx context.Context, req forkRequest) error {
	if req.promptMessage == "" {
		return nil
	}
	conn, ok := rs.apiConns.get(req.runnerID)
	if !ok {
		return ErrSpawnPromptUndeliverable
	}
	// conn.tctx, not the spawn's own: applyAPITransport overwrites Session with
	// whatever EstablishSession actually confirmed — a freshly minted thread id
	// on a fresh spawn — and the spawn's copy still holds the id it asked for.
	if _, _, err := rs.pushPromptOverAPI(
		ctx, req.runnerID, conn.tctx.Session, conn.tctx.Cwd, req.promptMessage,
	); err != nil {
		carrier := engineagents.Runner{ID: req.runnerID, ProviderID: req.providerID}
		rs.loseAPISession(ctx, req.chatID, carrier, conn.tctx.Session, err)
		return fmt.Errorf("%w: %w", ErrSpawnPromptUndeliverable, err)
	}
	return nil
}

// handOverAPIConn marks runnerID's connection as replaced by another process of the
// SAME runner, so its kill is not read as the runner ending. Call it before the drop.
func (rs *Runners) handOverAPIConn(runnerID string) {
	if c, ok := rs.apiConns.get(runnerID); ok {
		c.detached.Store(true)
	}
}

// runnerHasAnotherProcess reports whether runnerID is still alive after one of its
// processes died. A runner is one process, of a varying kind (serve, native view, PTY);
// this is the one place that asks which, so no teardown path has to.
func (rs *Runners) runnerHasAnotherProcess(runnerID string) bool {
	if rs.ShowingNativeView(runnerID) || rs.HasLiveAPIConnection(runnerID) {
		return true
	}
	runner, ok := rs.live.byRunner(runnerID)
	return ok && runner.TerminalSession != ""
}

// exitProcesslessRunner reconciles a runner that just lost its last process. The
// CALLER holds the chat's lock; taking it here would deadlock SwitchToNative, which
// holds it across the re-establish window.
func (rs *Runners) exitProcesslessRunner(ctx context.Context, runnerID string) {
	// The exit is reconciled in full even if the request that noticed it ends.
	ctx = context.WithoutCancel(ctx)
	if rs.runnerHasAnotherProcess(runnerID) {
		return
	}
	rs.reconcileRunnerExit(ctx, runnerID)
}

// watchAPIConnExit makes runnerID's connection its process: the serve exiting
// ends the runner, and a connection lost on its own names why. Armed at adoption
// and again after SwitchToNative, which re-establishes the connection.
func (rs *Runners) watchAPIConnExit(runnerID, home, tmpDir string) bool {
	onExit := rs.onRunnerExit(home, runnerID, tmpDir, resumeWatch{})
	return rs.apiConns.watchExit(runnerID, func(lost string) {
		if lost != "" {
			rs.live.noteCause(runnerID, lost)
		}
		// No screen: a connection's refusal is the establish error, not its exit.
		onExit(engineterminal.Exit{})
	})
}

// watchExit arms onExit (also on deliberate teardown) against runnerID's `serve`, the
// liveness a PTY-less runner has, and reaps it from one goroutine so it never lingers
// as a zombie. false means no connection: the spawn has no liveness and must not record.
func (r *apiConnRegistry) watchExit(runnerID string, onExit func(lost string)) bool {
	c, ok := r.get(runnerID)
	if !ok || c.serve == nil {
		return false
	}
	go func() {
		<-c.serve.exited
		if c.detached.Load() {
			return // not the runner's exit — see apiconn.detached
		}
		onExit(c.exitCause())
	}()
	return true
}
