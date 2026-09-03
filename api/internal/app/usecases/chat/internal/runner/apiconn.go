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
	"fmt"
	"hash/fnv"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/core/binpath"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type apiconn struct {
	serveCmd *exec.Cmd
	driver   *engineagents.APIConn
	// ctx/cancel are the connection's OWN lifetime, deliberately NOT derived
	// from whatever request's ctx happened to trigger the spawn. pumpAPIConn's
	// goroutine outlives that request by design — codex's reply to THIS
	// message can arrive minutes after the spawn (or prompt) HTTP call already
	// returned — and a request-scoped ctx is cancelled the instant that call
	// completes, silently failing every IngestHook call after with "context
	// canceled" and making the reply vanish. cancel is called from drop, the
	// same place serveCmd is killed, so nothing outlives the connection either.
	ctx    context.Context
	cancel context.CancelFunc
	// agent/tctx are this connection's own descriptor and rendered template
	// context, set once establish succeeds. SwitchToTerminal (attach.go) reads
	// them back to render attach's argv and to re-establish this SAME
	// connection later, on the way back to native — nothing else needs them.
	agent engineagents.Agent
	tctx  engineagents.TemplateCtx
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
// common case for a hooks-only provider, and is how submitPromptOverAPI
// decides to fall back to restart_tui instead.
func (r *apiConnRegistry) get(runnerID string) (*apiconn, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.byRun[runnerID]
	return c, ok
}

// drop closes and forgets runnerID's connection, if it has one. Safe to call
// for a runner that never had one (the hooks-only common case).
func (r *apiConnRegistry) drop(runnerID string) {
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
	if c.serveCmd != nil && c.serveCmd.Process != nil {
		_ = c.serveCmd.Process.Kill()
	}
}

// closeAll drops every connection this registry holds. This is the ONLY path
// that reaches apiConnRegistry at daemon shutdown: individual runners are
// already torn down at retire, provider switch, spawn-failure rollback, and
// PTY-exit, but nothing on the shutdown chain (engine.Container.Close only
// knows PTYs and LSP servers; apiConnRegistry lives a layer below that) ever
// visits the ones still live when the daemon exits. Without this, every
// `serve` process a mixed-transport provider forked outlives the daemon that
// spawned it.
//
// Snapshots the ids under the lock, then calls drop per id with the lock
// released — drop takes its own lock, so holding r.mu across those calls
// would deadlock.
func (r *apiConnRegistry) closeAll() {
	r.mu.Lock()
	ids := make([]string, 0, len(r.byRun))
	for id := range r.byRun {
		ids = append(ids, id)
	}
	r.mu.Unlock()
	for _, id := range ids {
		r.drop(id)
	}
}

// apiSocketPath derives a short path under the OS temp dir, keyed by a hash of
// runnerID — mirroring internal/core/gateway/transports.overrideSocketPath's own
// convention. It must be short and NEVER under a Crowbar worktree: macOS's
// sun_path is a hard 104 bytes, and a worktree-rooted tmpDir routinely exceeds
// it (see [[project_dev_home_isolation]]).
func apiSocketPath(runnerID string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(runnerID))
	return filepath.Join(os.TempDir(), fmt.Sprintf("crowbar-api-%x.sock", h.Sum64()))
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
	cmd, err := forkServeProcess(serveArgv)
	if err != nil {
		slog.WarnContext(ctx, "agent: api transport: start serve", "err", err, "runner_id", runnerID)
		return nil, false
	}
	if err := waitForSocket(ctx, tctx.Socket); err != nil {
		slog.WarnContext(ctx, "agent: api transport: serve never opened its socket",
			"err", err, "runner_id", runnerID)
		_ = cmd.Process.Kill()
		return nil, false
	}
	driver, err := agent.StartAPIConn(ctx, tctx.Socket)
	if err != nil {
		slog.WarnContext(ctx, "agent: api transport: handshake", "err", err, "runner_id", runnerID)
		_ = cmd.Process.Kill()
		return nil, false
	}
	connCtx, cancel := context.WithCancel(context.Background())
	conn := &apiconn{serveCmd: cmd, driver: driver, ctx: connCtx, cancel: cancel}
	rs.apiConns.set(runnerID, conn)
	return conn, true
}

// applyAPITransport starts serve+handshake for an api-transport descriptor,
// ESTABLISHES its session (Fresh or Resume — see apiconn's own EstablishSession
// doc) before anything else, and, once connected, points plan at `attach`'s
// argv instead of the bare descriptor spawn.cmd — so the PTY spawnRunner forks
// carries the attached TUI, not a second copy of the hooks-only launch.
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
	plan *engineagents.SpawnPlan,
	resumeContext string,
) {
	conn, ok := rs.startAPIConn(ctx, runnerID, agent, tctx)
	if !ok {
		return
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
	established, err := conn.driver.EstablishSession(ctx, "prompt", values)
	if err != nil {
		slog.WarnContext(ctx, "agent: api transport: establish session", "err", err, "runner_id", runnerID)
		rs.apiConns.drop(runnerID)
		return
	}
	tctx.Session = established["session_id"]
	// The ONLY channel that reaches an already-resumed codex thread: no CLI
	// argv exists to carry it (the redundant hooks-only PTY is deliberately
	// starved of both the native id and this document — see apiOwnsResume,
	// prompts.go — because handing either to a second writer on the same
	// thread is what corrupted the switch in the first place), and
	// thread/resume's own send: template has nowhere to put it either. Best
	// effort, like every other step in this function: a failed inject leaves
	// the resumed session running with no memory of the gap, not un-resumed.
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
	if agent.Capabilities().Hotswap {
		if attachArgv, ok := agent.APIAttachArgv(tctx); ok {
			plan.Executable = binpath.Resolve(attachArgv[0])
			plan.Argv = attachArgv[1:]
		}
	}
	rs.pumpAPIConn(runnerID, providerID, agent, conn)
}

// forkServeProcess starts argv as a long-lived BACKGROUND process — not a PTY:
// codex's app-server is a headless control-plane process, and the PTY the rest
// of spawnRunner manages is reserved for `attach`, if the descriptor declares
// one.
func forkServeProcess(argv []string) (*exec.Cmd, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("agent: api transport: empty serve argv")
	}
	cmd := exec.Command(binpath.Resolve(argv[0]), argv[1:]...) //nolint:gosec // argv is descriptor-declared and template-expanded, not user input
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("agent: api transport: start %s: %w", argv[0], err)
	}
	return cmd, nil
}

// waitForSocket polls for sockPath to exist, bounded by ctx. codex's app-server
// creates the socket file synchronously on bind, so a short poll is enough —
// there is no readiness protocol beyond the file's existence.
func waitForSocket(ctx context.Context, sockPath string) error {
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(sockPath); err == nil {
			return nil
		}
		select {
		case <-ticker.C:
			continue
		case <-deadline.C:
			return fmt.Errorf("agent: api transport: socket %s never appeared", sockPath)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// pumpAPIConn forwards every canonical event this connection's driver resolves
// into the SAME ingest entrypoint hooks use — ownership, activity, and the
// answer desk need no transport-specific branch because of this. Runs until the
// driver's Events() channel closes (the connection died) or conn's own ctx is
// cancelled (by drop, on runner exit) — NEVER the ctx of whatever request
// triggered the spawn: this goroutine outlives that request by design, and
// using its ctx would cancel every IngestHook the instant that request
// returned. See apiconn's own field comment.
//
// agent (the SAME engineagents.Agent spawnRunner already holds) is how this
// reaches TransportFor — never a raw *spec.Descriptor, which this package
// cannot name.
func (rs *Runners) pumpAPIConn(
	runnerID, providerID string, agent engineagents.Agent, conn *apiconn,
) {
	ctx := conn.ctx
	go func() {
		for ev := range conn.driver.Events() {
			if agent.TransportFor(ev.Canonical) != "api" {
				// Declared on hooks by this descriptor — the hooks wire already
				// carries it, or will; a driver-resolved copy here would double it.
				continue
			}
			// Marked on EVERY event, not just asks: ingestResolvedHook's
			// "redundant hooks-delivered copy" guard cannot otherwise tell this
			// call apart from the companion PTY's hooks echo of the SAME
			// api-owned event — see inflight.FromAPITransport's own doc comment.
			evCtx := inflight.WithAPITransport(ctx)
			if ev.AskID != nil {
				// A synthetic delivery id scopes this ask's slot exactly the way an
				// HTTP hook delivery's id would — holdForAnswer (internal/turn's
				// observation.go) reads this straight out of the context and needs
				// no other change to hold the prompt open.
				evCtx = inflight.WithDeliveryID(evCtx, runnerID+":"+hex.EncodeToString(ev.AskID))
			}
			if err := rs.turns.IngestHook(evCtx, runnerID, providerID, ev.Canonical, ev.Raw); err != nil {
				slog.WarnContext(ctx, "agent: api transport: ingest", "err", err,
					"runner_id", runnerID, "event", ev.Canonical)
				continue
			}
			if ev.AskID != nil {
				rs.awaitAndReplyOverSocket(ctx, runnerID, ev.AskID, conn)
			}
		}
	}()
}

// HasLiveAPIConnection reports whether runnerID has an ACTIVE api-transport
// connection right now — as opposed to torn down for its native view (see
// SwitchToTerminal, attach.go), or never declared one at all. chatRuntime
// (chats.go) uses this to tell a legitimately-live TerminalSession (claude:
// its PTY IS the conversation) from the disconnected companion PTY every
// api-transport spawn still forks alongside a LIVE connection (a known gap) —
// reporting the latter as "the terminal" is what let a user type into an
// unrelated codex session and have it silently promoted into its own new
// chat. Confirmed live.
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
	rs.apiConns.closeAll()
}

// pushPromptOverAPI delivers text to runnerID's live api connection, if it has
// one — no PTY restart, no new runnerID: the same connection applyAPITransport
// opened at spawn carries every message the conversation ever sends. ok=false
// means this runner has no live api connection at all (a hooks-only provider,
// or a mixed-transport one whose serve process never came up); the caller
// falls back to restart_tui exactly as it did before mixed transport existed.
func (rs *Runners) pushPromptOverAPI(
	ctx context.Context, runnerID, sessionID, cwd, text string,
) (usedSessionID string, ok bool, err error) {
	conn, ok := rs.apiConns.get(runnerID)
	if !ok {
		return "", false, nil
	}
	// The session was already established (Fresh or Resume) by
	// applyAPITransport at spawn time, before attach's argv was ever
	// rendered — this call only ever runs Action (turn/start).
	result, err := conn.driver.Dispatch(ctx, "prompt", map[string]string{
		"session_id": sessionID,
		"cwd":        cwd,
		"text":       text,
	})
	if err != nil {
		return "", true, err
	}
	return result["session_id"], true, nil
}

// awaitAndReplyOverSocket blocks on the SAME answerdesk.Await an HTTP hook relay
// would, on this goroutine instead of an HTTP request — there is no relay
// process for the API transport, so this goroutine IS the thing waiting. On a
// verdict (or an expired budget, which yields an empty stdout exactly as it
// would for a hook relay nobody answered in time — the provider then falls back
// to answering through its own attached TUI, per capability 2), it writes the
// reply back over the socket.
func (rs *Runners) awaitAndReplyOverSocket(
	ctx context.Context, runnerID string, askID json.RawMessage, conn *apiconn,
) {
	deliveryID := runnerID + ":" + hex.EncodeToString(askID)
	answer, err := rs.answers.Await(ctx, deliveryID)
	if err != nil {
		return // ctx cancelled or the connection closed underneath the wait
	}
	if len(answer.Stdout) == 0 {
		return // nobody answered from Crowbar in time; codex's own attached TUI still has it
	}
	if err := conn.driver.Reply(askID, answer.Stdout); err != nil {
		slog.WarnContext(ctx, "agent: api transport: write reply", "err", err, "runner_id", runnerID)
	}
}
