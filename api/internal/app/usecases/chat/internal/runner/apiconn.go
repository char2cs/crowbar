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
	// goroutine outlives that request by design — codex's reply to THIS
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
	// replacedSession is set when the provider refused the session this spawn
	// asked to resume and a fresh one was opened in its place.
	replacedSession bool
	// originated is every conversation this connection's own driver opened in
	// place of one Crowbar named — see sessionorigin.go.
	originated *originatedSessions
	// detached marks a connection whose process is being killed without the
	// runner ending: handed over to its native view (SwitchToTerminal), or the
	// daemon shutting down, whose next boot reconciles the row. Read by
	// watchExit (apirunner.go), which holds this pointer after the entry is gone.
	detached atomic.Bool
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
// descriptor. For an api-transport one, it forks `serve`, waits for the socket
// to exist, and hands the connection to protocol.StartAPIDriver (via
// engineagents.Agent.StartAPIConn — see this file's own package doc comment for
// why that indirection is required, not optional).
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
	// override below: a unit test's descriptor is the REAL codex.yaml, and a
	// developer machine with codex actually installed would otherwise fork a
	// genuine `codex app-server` subprocess as a side effect of any test that
	// spawns a "codex" runner — slow, leaks processes, and makes test behavior
	// depend on what happens to be on the local PATH. CI (no codex installed)
	// already degrades this way for free; this makes every environment agree.
	if os.Getenv("CROWBAR_DISABLE_API_TRANSPORT") != "" {
		return nil, false
	}
	serveArgv, ok := agent.APIServeArgv(tctx)
	if !ok {
		return nil, false
	}
	// A previous connection for this SAME runner id (SwitchToNative re-establishes
	// on live.ID, never a fresh one) can have left its socket file behind: drop's
	// SIGKILL gives the old `serve` no chance to unlink what it was listening on.
	// Left in place, that stale file satisfies waitForSocket's mere existence
	// check the instant this fork starts, racing the handshake below against a
	// corpse instead of the process just started — confirmed live as "connection
	// refused" on a socket path that very much exists. Removing it first
	// guarantees the file waitForSocket blocks on is always this call's OWN
	// process binding, never a leftover one. A path with nothing there (the
	// common case, a runner id's first connection) is a silent no-op.
	_ = os.Remove(tctx.Socket)
	serve, err := forkServeProcess(serveArgv)
	if err != nil {
		slog.WarnContext(ctx, "agent: api transport: start serve", "err", err, "runner_id", runnerID)
		return nil, false
	}
	if err := waitForSocket(ctx, tctx.Socket, serve.exited); err != nil {
		slog.WarnContext(ctx, "agent: api transport: serve never opened its socket",
			"err", err, "runner_id", runnerID)
		serve.kill()
		return nil, false
	}
	// Built BEFORE the driver, and handed to it: a driver that recovers a lost
	// session opens a conversation nobody asked for, and the claim has to be
	// open before the first call of that recovery reaches the wire.
	originated := newOriginatedSessions()
	driver, err := agent.StartAPIConn(ctx, tctx.Socket, originated.Claim)
	if err != nil {
		slog.WarnContext(ctx, "agent: api transport: handshake", "err", err, "runner_id", runnerID)
		serve.kill()
		return nil, false
	}
	connCtx, cancel := context.WithCancel(context.Background())
	conn := &apiconn{
		serve: serve, driver: driver, ctx: connCtx, cancel: cancel, originated: originated,
	}
	rs.apiConns.set(runnerID, conn)
	return conn, true
}

// applyAPITransport starts serve+handshake for an api-transport descriptor,
// ESTABLISHES its session (Fresh or Resume) before anything else, and — for a
// hotswap descriptor only — RETURNS `attach`'s argv for the caller's PTY to
// run instead of the descriptor's spawn.cmd. Returns nil otherwise.
//
// The session must exist BEFORE attach's argv is rendered: attach has to name
// the SAME thread `prompt`'s turn/start will act on (codex.yaml's attach is
// `codex resume {session_id} --remote ...`), and that id is not known until
// EstablishSession has run — which is why this happens here, at spawn time,
// rather than lazily on the first message the way sending itself still does.
//
// A no-op for a hooks-transport descriptor (agent.APIServeArgv reports
// ok=false) and, per design spec §2.2b, never a reason to fail the spawn: a
// failed serve OR a failed establish leaves plan untouched and the session
// runs over hooks alone.
//
// resumeContext is non-empty exactly when this is resuming a session that
// already existed (tctx.Session was non-blank going in) AND there is a real
// gap to hand over (renderSpawnContext's own inject gate — nothing recorded
// while this provider was away yields ""). It rides a SEPARATE call
// (InjectAt "context") from EstablishSession's own "context" value below:
// codex's thread/resume never accepts a context field (confirmed live — its
// own send: template does not reference {context} at all), so the identical
// composed document is threaded through twice, once per channel that can
// actually carry it depending on which branch runs.
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
	values := map[string]string{
		"session_id": tctx.Session,
		"cwd":        tctx.Cwd,
		// Same tctx.Context a restart_tui spawn's ContextSteps would fold into
		// its CLI argv (see spawnRunner/renderSpawnContext) — composed exactly
		// once, transport-agnostic. Fresh's own thread/start send: template is
		// the only Fresh-or-Resume step that references {context} (as
		// developerInstructions); Resume's ignores it, so passing it
		// unconditionally is a no-op on that branch — resumeContext below is
		// how the SAME document still reaches a resumed thread.
		"context": tctx.Context,
	}
	// permission.<key>, the SAME family a restart_tui spawn's own
	// permission_levels.apply pass_arg steps render from tctx.PermissionVars
	// via TemplateCtx.Replacer() — Fresh's thread/start send: tree references
	// these as {permission.sandbox}/{permission.approvalPolicy} (codex.yaml),
	// so a fresh api-transport session must resolve them from the same
	// source, or a codex chat spawned over this channel launches with an
	// EMPTY sandbox/approvalPolicy regardless of the chat's own dial.
	for k, v := range tctx.PermissionVars {
		values["permission."+k] = v
	}
	requested := tctx.Session
	established, err := conn.driver.EstablishSession(ctx, "prompt", values)
	if err != nil {
		slog.WarnContext(ctx, "agent: api transport: establish session", "err", err, "runner_id", runnerID)
		rs.apiConns.drop(runnerID)
		return nil
	}
	tctx.Session = established["session_id"]
	if requested != "" && tctx.Session != requested {
		// The provider refused the recorded session and a new one was opened:
		// the ladder's transcript rung, so it is handed the whole conversation.
		conn.replacedSession = true
		resumeContext = rs.transcriptFor(ctx, tctx.ChatID)
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

	// Auto-attaching at spawn is only correct for a HOTSWAP provider: it means
	// the attached view is meant to be there for the whole session, the way
	// claude's hooks-transport PTY already is. A provider that declares attach
	// WITHOUT hotswap wants it on demand only, once idle — attaching here,
	// before any turn has run, is exactly the request that fails live against
	// codex (its rollout is not flushed yet) — so that case is left to
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
		defer rs.onAPIConnLost(ctx, runnerID)
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
				go rs.awaitAndReplyOverSocket(ctx, runnerID, agent, ev, conn)
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

// Shutdown kills every live api-transport connection this daemon still holds.
// Nothing else on the shutdown path reaches these: engine.Container.Close()
// only knows about PTYs and LSP servers, and this registry lives one layer
// below that. Called once, at daemon shutdown — see ShutdownAPIConnections
// in the chat usecase and shutdownAgentRunners in app/container.go.
func (rs *Runners) Shutdown() {
	rs.background.stop()
	rs.apiConns.closeAll()
}

// awaitAndReplyOverSocket is the api channel's hook relay: it waits on the
// answer desk and writes the verdict back. Every ask gets a reply — an api
// provider has no TUI of its own to fall back to, so an ask left unanswered
// would hold its turn open forever. On an expired budget the descriptor's own
// refusal is sent instead.
func (rs *Runners) awaitAndReplyOverSocket(
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
