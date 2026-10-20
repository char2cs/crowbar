// Package runner (file apiconn.go) is the api-transport half of spawning a
// provider: start `serve` as a background process, complete the handshake, pump
// its resolved events into the same ingest entrypoint hooks use, and answer any
// permission it asks from Crowbar's own chat.
//
// This file reaches the API transport ONLY through engineagents.Agent
// (StartAPIConn/APIServeArgv/APIAttachArgv/TransportFor) — never through
// .../agents/internal/protocol or .../apidriver, which this package's import
// path has no visibility into (Go's internal/ boundary sits one directory
// higher than this package).
package runner

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/core/binpath"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type apiconn struct {
	serve  *serveProcess
	driver *engineagents.APIConn
	// ctx/cancel are the connection's OWN lifetime, deliberately NOT derived
	// from whatever request's ctx happened to trigger the spawn. pumpAPIConn's
	// goroutine outlives that request by design — the provider's reply to THIS
	// message can arrive minutes after the spawn (or prompt) HTTP call already
	// returned — and a request-scoped ctx is cancelled the instant that call
	// completes, silently failing every IngestHook call after with "context
	// canceled" and making the reply vanish. cancel is called from drop, the
	// same place serve is killed, so nothing outlives the connection either.
	ctx    context.Context
	cancel context.CancelFunc
	// agent/tctx are this connection's own descriptor and rendered template
	// context, set once establish succeeds. SwitchToTerminal (attach.go) reads
	// them back to render attach's argv and to re-establish this SAME
	// connection later, on the way back to native — nothing else needs them.
	agent engineagents.Agent
	tctx  engineagents.TemplateCtx
	// detached marks a connection whose process is being killed without the
	// runner ending: handed over to its native view (SwitchToTerminal), or the
	// daemon shutting down, whose next boot reconciles the row. Read by
	// watchExit (apirunner.go), which holds this pointer after the entry is gone.
	detached atomic.Bool
	// lost is why the connection died on its own, kept here because only a
	// watched connection's death is its runner's exit (watchAPIConnExit).
	lost atomic.Pointer[string]
}

// lostReason is why c died on its own; "" while it lives or was taken down.
func (c *apiconn) lostReason() string {
	if reason := c.lost.Load(); reason != nil {
		return *reason
	}
	return ""
}

// apiConnRegistry is the per-runner registry pumpAPIConn's ingest loop and
// onRunnerExit's teardown look up by runnerID. In memory only, like every other
// live-process registry in this package (answerdesk's Desk, pendingHooks) — it
// describes a live connection to a live process, so it cannot survive a restart
// and must not try to.
type apiConnRegistry struct {
	mu    sync.Mutex
	byRun map[string]*apiconn
}

func newAPIConnRegistry() *apiConnRegistry {
	return &apiConnRegistry{byRun: make(map[string]*apiconn)}
}

func (r *apiConnRegistry) set(runnerID string, c *apiconn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byRun[runnerID] = c
}

// get returns runnerID's live connection, if it has one. ok=false is the
// common case for a hooks-channel runner. Nil-safe: test doubles build a
// Runners without a registry.
func (r *apiConnRegistry) get(runnerID string) (*apiconn, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.byRun[runnerID]
	return c, ok
}

// drop closes and forgets runnerID's connection, if it has one. Safe to call
// for a runner that never had one (the hooks-only common case), and on a nil
// registry — pumpAPIConn's own loss handler runs on a goroutine that outlives
// whatever built it, including test doubles that never made one.
func (r *apiConnRegistry) drop(runnerID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	c, ok := r.byRun[runnerID]
	delete(r.byRun, runnerID)
	r.mu.Unlock()
	if !ok {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	if c.driver != nil {
		_ = c.driver.Close()
	}
	c.serve.kill()
}

// closeAll drops every connection this registry holds. This is the ONLY path
// that reaches apiConnRegistry at daemon shutdown: individual runners are
// already torn down at retire, provider switch, spawn-failure rollback, and
// PTY-exit, but nothing on the shutdown chain (engine.Container.Close only
// knows PTYs and LSP servers; apiConnRegistry lives a layer below that) ever
// visits the ones still live when the daemon exits. Without this, every
// `serve` process a mixed-transport provider forked outlives the daemon that
// spawned it. Each is detached first: the daemon ending is not the runner
// exiting, and the next boot reconciles the row as daemon_restart.
//
// Snapshots the ids under the lock, then calls drop per id with the lock
// released — drop takes its own lock, so holding r.mu across those calls
// would deadlock.
func (r *apiConnRegistry) closeAll() {
	r.mu.Lock()
	ids := make([]string, 0, len(r.byRun))
	for id, c := range r.byRun {
		c.detached.Store(true)
		ids = append(ids, id)
	}
	r.mu.Unlock()
	for _, id := range ids {
		r.drop(id)
	}
}

// startAPIConn starts nothing and returns ok=false for a hooks-transport
// descriptor. For an api-transport one, it forks `serve` and hands its stdio to
// protocol.StartAPIDriver (via engineagents.Agent.StartAPIConn — see this
// file's own package doc comment for why that indirection is required, not
// optional). Serve is ready when it answers the handshake, and not before.
//
// A failure at any step degrades to ok=false rather than an error: the caller
// (spawnRunner) must still succeed over hooks alone. Design spec §2.2b: "Failure
// of attach must not fail the session."
func (rs *Runners) startAPIConn(
	ctx context.Context,
	runnerID string,
	agent engineagents.Agent,
	tctx engineagents.TemplateCtx,
) (*apiconn, bool) {
	// Test-only escape hatch, mirroring crowbarHookPath's CROWBAR_HOOK_BIN
	// override below: a unit test may spawn a SHIPPED descriptor, and a machine
	// with that provider's CLI installed would otherwise fork its real serve as
	// a side effect — slow, leaks processes, and makes test behavior depend on
	// what happens to be on the local PATH. CI (no CLI installed) already
	// degrades this way for free; this makes every environment agree.
	if os.Getenv("CROWBAR_DISABLE_API_TRANSPORT") != "" {
		return nil, false
	}
	serveArgv, ok := agent.APIServeArgv(tctx)
	if !ok {
		return nil, false
	}
	serve, stdio, err := forkServeProcess(serveArgv)
	if err != nil {
		slog.WarnContext(ctx, "agent: api transport: start serve", "err", err, "runner_id", runnerID)
		return nil, false
	}
	//nolint:contextcheck // the driver announces long after this request; its values must not reach the binding.
	driver, err := agent.StartAPIConn(ctx, stdio, rs.sessionOrigin(tctx.ChatID, agent.ID()))
	if err != nil {
		slog.WarnContext(ctx, "agent: api transport: handshake", "err", err, "runner_id", runnerID)
		serve.kill()
		return nil, false
	}
	connCtx, cancel := context.WithCancel(context.Background())
	conn := &apiconn{serve: serve, driver: driver, ctx: connCtx, cancel: cancel}
	rs.apiConns.set(runnerID, conn)
	return conn, true
}

// applyAPITransport connects an api-transport descriptor and establishes its
// session first, so a hotswap attach argv (returned for the PTY) names the thread
// prompts act on. Any failure leaves the spawn to run over hooks alone.
func (rs *Runners) applyAPITransport(
	ctx context.Context,
	runnerID, providerID string,
	agent engineagents.Agent,
	tctx engineagents.TemplateCtx,
	resumeContext string,
) []string {
	conn, ok := rs.startAPIConn(ctx, runnerID, agent, tctx)
	if !ok {
		return nil
	}
	if tctx.Session, ok = rs.establishAPISession(ctx, conn, runnerID, providerID, tctx); !ok {
		return nil
	}
	// The only channel that reaches an already-resumed thread with the gap
	// (thread/resume's own send: has nowhere to put it). Best effort: a failed
	// inject leaves the resumed session running without the gap, not unresumed.
	if resumeContext != "" {
		if err := conn.driver.InjectAt(ctx, "context", map[string]string{
			"session_id": tctx.Session,
			"context":    resumeContext,
		}); err != nil {
			slog.WarnContext(ctx, "agent: api transport: inject resume context", "err", err, "runner_id", runnerID)
		}
	}
	conn.agent, conn.tctx = agent, tctx

	// Auto-attaching at spawn is only correct for a HOTSWAP provider: its
	// attached view is meant to be there for the whole session, the way a
	// hooks-transport PTY already is. A provider that declares attach WITHOUT
	// hotswap wants it on demand only, once idle — before any turn has run, its
	// session may not exist to attach to yet — so that case is left to
	// SwitchToTerminal (attach.go), never rendered eagerly.
	var attach []string
	if agent.Capabilities().Hotswap {
		if attachArgv, ok := agent.APIAttachArgv(tctx); ok {
			attach = attachArgv
		}
	}
	rs.pumpAPIConn(runnerID, providerID, agent, conn)
	return attach
}

// establishAPISession opens or resumes tctx's session on conn. A session the
// provider refuses is lost, and any failure drops the connection.
func (rs *Runners) establishAPISession(
	ctx context.Context,
	conn *apiconn,
	runnerID, providerID string,
	tctx engineagents.TemplateCtx,
) (string, bool) {
	values := map[string]string{
		"session_id": tctx.Session,
		"cwd":        tctx.Cwd,
		// Only Fresh renders {context}; a resumed session gets the same document
		// through applyAPITransport's resumeContext inject.
		"context": tctx.Context,
	}
	// A fresh session renders the chat's permission level from the same vars a
	// restart_tui spawn's argv does, or it launches with none.
	for k, v := range tctx.PermissionVars {
		values["permission."+k] = v
	}
	established, err := conn.driver.EstablishSession(ctx, "prompt", values)
	if err != nil {
		slog.WarnContext(ctx, "agent: api transport: establish session", "err", err, "runner_id", runnerID)
		if errors.Is(err, engineagents.ErrAPISessionLost) &&
			rs.loseSession(ctx, tctx.ChatID, providerID, tctx.Session, err.Error(), runnerID) {
			rs.noteRefusedRunner(tctx.ChatID, runnerID)
		}
		rs.apiConns.drop(runnerID)
		return "", false
	}
	return established["session_id"], true
}

// pointPlanAtAttach redirects a spawn plan's PTY at applyAPITransport's attach
// argv. binpath.Resolve for the same reason spawnRunner resolves its own
// argv[0]: exec.Command looks a bare name up against the DAEMON's PATH.
func pointPlanAtAttach(plan *engineagents.SpawnPlan, attachArgv []string) {
	if len(attachArgv) == 0 {
		return
	}
	plan.Executable = binpath.Resolve(attachArgv[0])
	plan.Argv = attachArgv[1:]
}

// pumpAPIConn forwards every canonical event this connection's driver resolves
// into the same ingest entrypoint the hook relay uses. It runs on the
// connection's own ctx (never a request's) until the driver closes its events.
func (rs *Runners) pumpAPIConn(
	runnerID, providerID string, agent engineagents.Agent, conn *apiconn,
) {
	ctx := conn.ctx
	go func() {
		defer rs.onAPIConnLost(ctx, conn, runnerID)
		for ev := range conn.driver.Events() {
			evCtx := inflight.WithAPITransport(ctx)
			if ev.AskID != nil {
				// The synthetic delivery id scopes the ask's answer-desk slot the
				// way an HTTP relay's delivery id does.
				evCtx = inflight.WithDeliveryID(evCtx, apiAskDeliveryID(runnerID, ev.AskID))
			}
			if err := rs.turns.IngestHook(evCtx, runnerID, providerID, ev.Canonical, ev.Raw); err != nil {
				slog.WarnContext(ctx, "agent: api transport: ingest", "err", err,
					"runner_id", runnerID, "event", ev.Canonical)
				continue
			}
			if ev.AskID != nil {
				// Off the pump: a human's decision can take minutes, and every
				// event behind the ask must keep flowing.
				go rs.awaitAndReply(ctx, runnerID, agent, ev, conn)
			}
		}
	}()
}

func apiAskDeliveryID(runnerID string, askID json.RawMessage) string {
	return runnerID + ":" + hex.EncodeToString(askID)
}

// HasLiveAPIConnection reports whether runnerID's channel right now is an api
// connection — false for a hooks-channel runner and for one handed over to its
// native view (SwitchToTerminal).
func (rs *Runners) HasLiveAPIConnection(runnerID string) bool {
	_, ok := rs.apiConns.get(runnerID)
	return ok
}

// awaitAndReply is the api channel's hook relay: it waits on the
// answer desk and writes the verdict back. Every ask gets a reply — an api
// provider has no TUI of its own to fall back to, so an ask left unanswered
// would hold its turn open forever. On an expired budget the descriptor's own
// refusal is sent instead.
func (rs *Runners) awaitAndReply(
	ctx context.Context, runnerID string, agent engineagents.Agent, ev engineagents.APIEvent, conn *apiconn,
) {
	answer, err := rs.answers.Await(ctx, apiAskDeliveryID(runnerID, ev.AskID))
	if err != nil {
		return // the connection is closing: the provider's ask dies with it
	}
	reply := answer.Stdout
	if len(reply) == 0 {
		reply, err = agent.RenderAnswer(ev.Canonical, ev.Raw, engineagents.AnswerDecision{
			Key: answerdesk.RefusalKey(ev.Canonical), Reason: "No answer from Crowbar in time.",
		})
		if err != nil {
			slog.WarnContext(ctx, "agent: api transport: render refusal", "err", err, "runner_id", runnerID)
			return
		}
	}
	if err := conn.driver.Reply(ev.AskID, reply); err != nil {
		slog.WarnContext(ctx, "agent: api transport: write reply", "err", err, "runner_id", runnerID)
	}
}
