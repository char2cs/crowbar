// Package runner (file apirunner.go) is the spawn shape where the api
// connection IS the whole runner: no PTY is forked at all, and the `serve`
// process carries the liveness a PTY normally would.
//
// apiconn.go establishes and pumps a connection; attach.go forks the native
// view a user switched to. This file answers the one question in between —
// does this spawn still need a PTY? — and wires the exit signal for when it
// does not.
package runner

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// apiConnIsTheRunner reports whether runnerID's spawn has nothing left to
// fork: its api connection came up and no hotswap `attach` argv was rendered.
// One channel per runner: a connection that came up is the whole runner; one
// that did not leaves the PTY (over hooks) as the only process there is.
func (rs *Runners) apiConnIsTheRunner(req forkRequest, attachArgv []string) bool {
	return len(attachArgv) == 0 && rs.HasLiveAPIConnection(req.runnerID)
}

// carriedSelection is the part of sel that a carrier rendering `steps`
// actually takes — asked one field at a time, because a descriptor may
// declare a carrier for one and not the other.
//
// recordRunner stamps THIS, never the chat's intended selection. LaunchModel
// is the only authority on what a live CLI is running, so a row claiming a
// model no carrier received makes selectionRequiresRestart compare equal
// forever and the revert permanent.
//
// PermissionLevel passes through: both carriers take it (apply: on an argv,
// vars: in the establish call's send: tree), so neither branch can drop it.
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

// forkOrAdopt starts whatever this spawn actually needs: the descriptor's
// PTY, or nothing at all when the api connection is already the whole runner.
// The terminal-session id it returns is "" in the second case, which is what
// the runner row then records — see recordRunner's own StartInput.
//
// It is also the single door every spawn's PROMPT goes through, and the two
// branches carry it differently: the PTY carries it in the argv the
// descriptor's prompt_submit steps rendered it into, and an adopted
// connection has no argv at all, so it must dispatch the text itself. That is
// why req.promptMessage exists as a field rather than living only inside
// argv — the adopt branch used to fork nothing, throw the whole rendered plan
// away and report success, which silently dropped a user's message.
//
// The SELECTION is the same invariant one field over, and the branch that
// runs is what reports which carrier took it — so nothing downstream can
// record a model/effort the process never received.
func (rs *Runners) forkOrAdopt(
	ctx context.Context, req forkRequest, attachArgv []string,
) (string, engineagents.Selection, error) {
	if rs.apiConnIsTheRunner(req, attachArgv) {
		return "", req.apiSelection, rs.adoptAPIConn(ctx, req)
	}
	// An attach argv REPLACES the rendered spawn plan wholesale
	// (pointPlanAtAttach), so a forked PTY carries the selection in its own
	// argv only when it is running that plan. When it is attaching, the live
	// connection it attaches to is the carrier.
	carried := req.argvSelection
	if len(attachArgv) > 0 {
		carried = req.apiSelection
	}
	termSessID, err := rs.forkCLI(ctx, req)
	return termSessID, carried, err
}

// adoptAPIConn is forkCLI for a spawn with no PTY: it arms the same exit
// reconcile against the `serve` process instead of a terminal session. The
// startup barrier spawnRunner opened before the connection holds every event
// the connection delivers until the runner row commits.
func (rs *Runners) adoptAPIConn(ctx context.Context, req forkRequest) error {
	if !rs.apiConns.watchExit(req.runnerID, rs.onRunnerExit(req.crowbarHome, req.runnerID, req.tmpDir)) {
		rs.pendingHooks.Discard(req.runnerID)
		rs.abandonAdoptedSpawn(ctx, req)
		return fmt.Errorf("agent: spawn runner: api connection vanished before its exit could be watched")
	}
	// AFTER the barrier, never before: the reply to this starts arriving
	// immediately, over the same ingest the hook relay uses, and the runner row
	// does not exist yet.
	if err := rs.carryPromptOverAPIConn(ctx, req); err != nil {
		rs.pendingHooks.Discard(req.runnerID)
		rs.abandonAdoptedSpawn(ctx, req)
		return err
	}
	return nil
}

// carryPromptOverAPIConn delivers a prompt-bearing spawn's message down the
// connection this runner adopted instead of forking a PTY — the only carrier
// such a runner has (its rendered argv, prompt and all, is never run).
//
// A failure FAILS THE SPAWN: there is no other carrier, and the caller turns
// it into the journal's "uncertain" rather than a runner reported healthy with
// the user's message nowhere.
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
		return fmt.Errorf("%w: %w", ErrSpawnPromptUndeliverable, err)
	}
	return nil
}

func (rs *Runners) abandonAdoptedSpawn(ctx context.Context, req forkRequest) {
	rs.apiConns.drop(req.runnerID)
	rs.agents.ForgetRunner(req.runnerID)
	worktreepath.RemoveUnderHome(ctx, req.crowbarHome, req.tmpDir)
}

// handOverAPIConn marks runnerID's connection as being torn down in favour of
// another process for the SAME runner, so the kill that follows is not read
// as the runner ending. Called before the drop, never after: the watcher can
// observe the process die the instant drop signals it.
func (rs *Runners) handOverAPIConn(runnerID string) {
	if c, ok := rs.apiConns.get(runnerID); ok {
		c.handedOver.Store(true)
	}
}

// runnerHasAnotherProcess reports whether runnerID is still SOMETHING —
// after one of the several processes it can be has just died.
//
// A runner is exactly one process, but which kind is not fixed: an api-driven
// one is its `serve` connection normally and its native view between a
// SwitchToTerminal and the matching SwitchToNative, while every other runner
// is its own PTY. This is the one place that asks which, so no teardown path
// has to know.
func (rs *Runners) runnerHasAnotherProcess(runnerID string) bool {
	if rs.ShowingNativeView(runnerID) || rs.HasLiveAPIConnection(runnerID) {
		return true
	}
	runner, err := rs.runnerStore.Get(context.Background(), runnerID)
	return err == nil && runner.TerminalSession != ""
}

// exitProcesslessRunner reconciles a runner that has just lost its last
// process — the reconcile a dead PTY drives for every other runner.
//
// The CALLER serialises: every path that reaches this holds the chat's spawn
// gate, because the question "is anything else driving this runner" is only
// answerable once whatever is mid-teardown (or mid-re-establish) has
// finished. Taking the gate here instead would deadlock SwitchToNative, which
// holds it across exactly that window.
func (rs *Runners) exitProcesslessRunner(runnerID string) {
	if rs.runnerHasAnotherProcess(runnerID) {
		return
	}
	rs.reconcileRunnerExit(context.Background(), runnerID)
}

// rearmAPIConnExit points a PTY-less runner's exit signal back at the
// connection SwitchToNative just re-established, and reports whether there
// was one to point at. Without this the runner came back from its native view
// with nothing watching it at all: adoptAPIConn arms the ORIGINAL connection
// at spawn, and SwitchToTerminal killed that one.
func (rs *Runners) rearmAPIConnExit(runnerID string, tctx engineagents.TemplateCtx) bool {
	return rs.apiConns.watchExit(runnerID, rs.onRunnerExit(tctx.CrowbarHome, runnerID, tctx.Tmp))
}

// watchExit arms onExit against runnerID's `serve` process — the liveness
// signal a PTY-less runner has instead of a terminal session, and the one
// forkServeProcess's own doc already names as this connection's process.
//
// Reaping it here is also what keeps a killed `serve` from lingering as a
// zombie: drop SIGKILLs without waiting, so before this nothing ever called
// Wait on it. Wait is called from exactly one goroutine per connection, so it
// is never raced.
//
// onExit fires for a DELIBERATE teardown too (retire, provider switch, a
// failed persist — each drops the connection, which kills the process),
// exactly as a PTY's own exit callback already did when those paths
// terminated it. reconcileRunnerExit is idempotent against a row that is
// already gone.
//
// false means there was no connection to watch, which is the caller's signal
// that this spawn has no liveness signal at all and must not be recorded.
func (r *apiConnRegistry) watchExit(runnerID string, onExit func()) bool {
	c, ok := r.get(runnerID)
	if !ok || c.serveCmd == nil || c.serveCmd.Process == nil {
		return false
	}
	go func() {
		_ = c.serveCmd.Wait()
		if c.handedOver.Load() {
			return // another process took this runner over — see handOverAPIConn
		}
		onExit()
	}()
	return true
}
