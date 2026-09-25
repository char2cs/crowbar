package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
	"github.com/char2cs/crowbar/api/internal/core/binpath"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// attachedView is one runner's currently-live native-TUI PTY, and everything
// needed to re-establish the api connection it temporarily replaced.
type attachedView struct {
	termSessID string
	agent      engineagents.Agent
	tctx       engineagents.TemplateCtx
}

// attachRegistry is in memory only, exactly like apiConnRegistry: it names a
// live process, not a fact to persist. A daemon restart mid-attach drops back
// to native chat — harmless, since the provider's own session lives in ITS
// rollout, untouched either way — the same reasoning apiConnRegistry's own
// doc comment gives for not surviving a restart.
type attachRegistry struct {
	mu    sync.Mutex
	byRun map[string]attachedView
}

func newAttachRegistry() *attachRegistry {
	return &attachRegistry{byRun: make(map[string]attachedView)}
}

func (r *attachRegistry) set(runnerID string, v attachedView) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byRun[runnerID] = v
}

func (r *attachRegistry) get(runnerID string) (attachedView, bool) {
	if r == nil {
		return attachedView{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.byRun[runnerID]
	return v, ok
}

// drop is nil-safe like apiConnRegistry's own, for the same reason: retire()
// (lifecycle.go) now reaches this from every teardown path, including tests
// and callers that construct a Runners with no attach registry at all because
// nothing about their scenario ever attaches one.
func (r *attachRegistry) drop(runnerID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byRun, runnerID)
}

// AttachedTerminalSession is the terminal session runnerID's native view IS,
// while it has one — what the chat DTO reports instead of the runner's row.
func (rs *Runners) AttachedTerminalSession(runnerID string) (string, bool) {
	view, ok := rs.attached.get(runnerID)
	if !ok {
		return "", false
	}
	return view.termSessID, true
}

// ShowingNativeView reports whether the CLI's own UI is the one in front of
// the user — i.e. whether runnerID's chat is on the terminal surface right
// now, however it got there.
//
// It reads the surface, NOT this file's attach registry. Those were two
// independent answers and they disagreed: a chat BORN on the terminal
// (domain.Chat.Surface, seeded at spawn) has never called SwitchToTerminal
// and never appears in rs.attached, so it used to report "chat" while its
// TUI was the only thing on screen. The registry still answers WHICH
// terminal session the native view is (AttachedTerminalSession above); it is
// no longer a second opinion on which surface that makes.
//
// Still blind to a HOTSWAP provider's terminal, and unavoidably so: both of
// its faces are live at once over one channel, so Crowbar is never told which
// one is being looked at (design spec P6b's own stated limit).
func (rs *Runners) ShowingNativeView(runnerID string) bool {
	return rs.surfaces.get(runnerID) == engineagents.SurfaceTerminal
}

// ErrNoNativeTerminal is SwitchToTerminal's refusal for a provider with
// nothing to switch to: no live api connection, or one whose descriptor
// declares no attach at all (capability 2's "no reachable native view" state).
var ErrNoNativeTerminal = fmt.Errorf("agent: provider has no native terminal to show: %w", apperr.ErrUnprocessable)

// ErrTurnInProgress is SwitchToTerminal's refusal for a non-hotswap provider
// mid-turn — the one restriction this whole capability exists to enforce (a
// hotswap provider never calls this at all; its terminal is already live).
var ErrTurnInProgress = fmt.Errorf("agent: provider cannot hand a live turn to its native view: %w", apperr.ErrConflict)

// SwitchToTerminal moves chatID onto its provider's own TUI and returns the
// terminal session that TUI is. How depends on what the runner is now:
//
//   - its own PTY (claude; a chat born on the terminal): that PTY already is
//     the TUI, so only the surface moves;
//   - an api connection whose session the provider can load (a completed
//     turn wrote its rollout): the session rung — the connection is handed
//     over to a hooks-transport `attach` of that session, same runner;
//   - otherwise: the TUI is launched through the resume ladder like any
//     other replacement — Crowbar's transcript when there is history, fresh
//     when there is none.
//
// A dormant chat only records the move; its next start lands there.
func (rs *Runners) SwitchToTerminal(ctx context.Context, chatID string) (string, error) {
	park, release, err := rs.spawns.Acquire(ctx, chatID)
	if err != nil {
		return "", err
	}
	defer release()

	live, err := rs.runnerStore.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return "", rs.moveDormantSurface(ctx, chatID, engineagents.SurfaceTerminal)
	}
	if err != nil {
		return "", fmt.Errorf("agent: switch to terminal: live runner: %w", err)
	}
	// Idempotent: a retry racing its own earlier success gets the same view.
	if already, ok := rs.attached.get(live.ID); ok {
		return already.termSessID, nil
	}
	conn, ok := rs.apiConns.get(live.ID)
	if !ok {
		if live.TerminalSession == "" {
			return "", ErrNoNativeTerminal
		}
		rs.moveSurface(ctx, chatID, live.ID, engineagents.SurfaceTerminal)
		return live.TerminalSession, nil
	}
	working, err := rs.turns.ChatWorking(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: switch to terminal: chat working: %w", err)
	}
	if working {
		return "", ErrTurnInProgress
	}
	attachArgv, tctx, attachable, err := rs.attachPlan(ctx, chatID, live, conn)
	if err != nil {
		return "", err
	}
	if attachable {
		return rs.attachNativeView(ctx, chatID, live, conn.agent, tctx, attachArgv)
	}
	if !conn.agent.SurfaceStartHere(engineagents.SurfaceTerminal) {
		return "", ErrNoNativeTerminal
	}
	return rs.respawnOnSurface(ctx, park, chatID, live.ProviderID, engineagents.SurfaceTerminal)
}

// attachPlan is the session rung for an api runner's native view: its own
// session resumed in the TUI, offered only once the provider can load it —
// codex writes a thread's rollout when its first turn completes.
//
// live.CurrentSession, not conn.tctx.Session: turns are recorded under the
// runner row's session, and the connection's copy is set once at establish.
func (rs *Runners) attachPlan(
	ctx context.Context, chatID string, live engineagents.Runner, conn *apiconn,
) ([]string, engineagents.TemplateCtx, bool, error) {
	tctx := conn.tctx
	tctx.Session = live.CurrentSession
	argv, ok := conn.agent.APIAttachArgv(tctx)
	if !ok || live.CurrentSession == "" {
		return nil, tctx, false, nil
	}
	_, everTurned, err := rs.activity.LastTurnForSession(ctx, chatID, live.ProviderID, live.CurrentSession)
	if err != nil {
		return nil, tctx, false, fmt.Errorf("agent: switch to terminal: check session history: %w", err)
	}
	if !everTurned || rs.verifiedResume(ctx, conn.agent, chatID, live.CurrentSession) == "" {
		return nil, tctx, false, nil
	}
	return argv, tctx, true, nil
}

// attachNativeView hands live's api connection over to a hooks-transport PTY
// resuming the same session (APIAttachArgv carries the same injection steps
// any hooks-attached CLI gets, so it reports into the ledger as usual).
func (rs *Runners) attachNativeView(
	ctx context.Context, chatID string, live engineagents.Runner,
	agent engineagents.Agent, tctx engineagents.TemplateCtx, attachArgv []string,
) (string, error) {
	// The native view becomes this runner's process, so killing the connection
	// is a handover, not a death. Marked before the drop: the watcher can see
	// the process die the instant drop signals it.
	rs.handOverAPIConn(live.ID)
	rs.apiConns.drop(live.ID)

	argv := append([]string{binpath.Resolve(attachArgv[0])}, attachArgv[1:]...)
	// Keyed by the chat: live.WorkspaceID is empty for a chat with no worktree
	// of its own. os.Environ(), as every spawn: CreateCommand takes env verbatim,
	// and without PATH/HOME every hook the attach wires dies with exit 127.
	termSessID, err := rs.term.CreateCommand(ctx, chatID, tctx.Cwd, argv, os.Environ(),
		rs.onAttachExit(chatID, live.ID))
	if err != nil {
		// The connection is already gone; the chat degrades to dormant and a
		// resume revives it.
		return "", fmt.Errorf("agent: switch to terminal: fork native view: %w", err)
	}
	rs.attached.set(live.ID, attachedView{termSessID: termSessID, agent: agent, tctx: tctx})
	rs.moveSurface(ctx, chatID, live.ID, engineagents.SurfaceTerminal)
	rs.touch(ctx, chatID) // the pane's terminal session is now the native view
	return termSessID, nil
}

// respawnOnSurface moves chatID to surface by relaunching its provider there
// through the resume ladder (switchProviderLocked to the same provider): its
// own session when the provider still has it, else Crowbar's transcript,
// else fresh. The surface is recorded first because the spawn reads it.
func (rs *Runners) respawnOnSurface(
	ctx, park context.Context, chatID, providerID, surface string,
) (string, error) {
	defer rs.enterPhase(ctx, chatID, snapshot.PhaseSwitching)()
	if _, err := rs.chats.SetSurface(ctx, chatID, surface); err != nil {
		return "", fmt.Errorf("agent: move chat to %s: record surface: %w", surface, err)
	}
	runnerID, err := rs.switchProviderLocked(ctx, park, chatID, providerID)
	if err != nil {
		return "", err
	}
	runner, err := rs.runnerStore.Get(ctx, runnerID)
	if err != nil {
		return "", fmt.Errorf("agent: move chat to %s: new runner: %w", surface, err)
	}
	return runner.TerminalSession, nil
}

// moveDormantSurface records the surface a chat with no runner will start on.
func (rs *Runners) moveDormantSurface(ctx context.Context, chatID, surface string) error {
	if _, err := rs.chats.SetSurface(ctx, chatID, surface); err != nil {
		return fmt.Errorf("agent: move chat to %s: record surface: %w", surface, err)
	}
	return nil
}

// moveSurface records, durably and in memory, that chatID is now on
// `surface`. This is what makes domain.Chat.Surface the CURRENT view rather
// than the one the chat was born on: the two switch calls are the only
// moments Crowbar is told the user moved, so they are the only writers.
//
// Best-effort on the durable half, deliberately: the view has ALREADY moved
// by the time this runs (the process is forked, or torn down), and refusing
// the switch over a failed write would leave the two disagreeing in the
// worse direction — a chat told it is somewhere its process is not. The
// in-memory mirror is written regardless, so this session stays consistent
// with what is actually on screen; a daemon restart re-reads the durable
// field and is the only thing a lost write costs.
func (rs *Runners) moveSurface(ctx context.Context, chatID, runnerID, surface string) {
	rs.surfaces.set(runnerID, surface)
	if _, err := rs.chats.SetSurface(ctx, chatID, surface); err != nil {
		slog.WarnContext(ctx, "agent: record the chat's current surface (best-effort, continuing)",
			"chat_id", chatID, "runner_id", runnerID, "surface", surface, "err", err)
	}
}

// SwitchToNative moves chatID onto Crowbar's own chat surface. An attached
// native view is torn down and the api connection re-established over the
// same session; a runner that is its own PTY on the terminal surface either
// keeps serving (a provider whose chat surface is hooks-fed) or is relaunched
// on the chat surface through the resume ladder. A dormant chat only records
// the move. Idempotent.
func (rs *Runners) SwitchToNative(ctx context.Context, chatID string) error {
	park, release, err := rs.spawns.Acquire(ctx, chatID)
	if err != nil {
		return err
	}
	defer release()

	live, err := rs.runnerStore.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return rs.moveDormantSurface(ctx, chatID, engineagents.SurfaceChat)
	}
	if err != nil {
		return fmt.Errorf("agent: switch to native: live runner: %w", err)
	}
	if view, ok := rs.attached.get(live.ID); ok {
		rs.detachNativeView(ctx, chatID, live, view)
		return nil
	}
	if rs.surfaces.get(live.ID) != engineagents.SurfaceTerminal {
		return nil
	}
	_, agent, err := rs.chatCapabilityContext(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: switch to native: %w", err)
	}
	if !apiTransportDrivesSurface(agent, engineagents.SurfaceChat) {
		rs.moveSurface(ctx, chatID, live.ID, engineagents.SurfaceChat)
		return nil
	}
	working, err := rs.turns.ChatWorking(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: switch to native: chat working: %w", err)
	}
	if working {
		return ErrTurnInProgress
	}
	_, err = rs.respawnOnSurface(ctx, park, chatID, live.ProviderID, engineagents.SurfaceChat)
	return err
}

// detachNativeView reverses attachNativeView: the view's PTY is torn down and
// the api connection re-established over the same session (EstablishSession
// resumes a known session id).
func (rs *Runners) detachNativeView(
	ctx context.Context, chatID string, live engineagents.Runner, view attachedView,
) {
	rs.attached.drop(live.ID)
	rs.touch(ctx, chatID)

	if err := rs.term.TerminateGraceful(ctx, view.termSessID); err != nil &&
		!errors.Is(err, engineterminal.ErrSessionNotFound) {
		slog.WarnContext(ctx, "agent: switch to native: terminate native view (best-effort, continuing)",
			"runner_id", live.ID, "terminal_session_id", view.termSessID, "err", err)
	}
	// No resume context: this switches views on one live session; the spawn's
	// original context would only be replayed on every toggle.
	_ = rs.applyAPITransport(ctx, live.ID, live.ProviderID, view.agent, view.tctx, "")
	// A runner without a PTY of its own has just swapped processes again: the
	// re-established connection is its process, and if none came back it has
	// none. Still under the spawn gate, so the view's exit callback cannot race.
	if live.TerminalSession == "" && !rs.rearmAPIConnExit(live.ID, view.tctx) {
		rs.exitProcesslessRunner(ctx, live.ID)
	}
	rs.moveSurface(ctx, chatID, live.ID, engineagents.SurfaceChat)
}

// onAttachExit runs when the native-view PTY exits on its own — the user
// closed it, or its process died. Switching back to native automatically is
// the same "nothing left to show" recovery StopChat's own retire() gives a
// dead runner elsewhere: a chat left pointing at a terminal session that no
// longer exists is worse than one quietly resuming its api connection.
//
// Off the caller: the terminal engine runs this inside TerminateGraceful, and
// every deliberate teardown (SwitchToNative, Stop, retire) calls that while
// holding the chat's spawn gate — taking the gate here, in line, deadlocked
// the chat for good.
func (rs *Runners) onAttachExit(chatID, runnerID string) func() {
	return func() {
		rs.background.run(context.Background(), func(ctx context.Context) {
			if _, ok := rs.attached.get(runnerID); ok {
				if err := rs.SwitchToNative(ctx, chatID); err != nil {
					slog.Error("agent: native view exited: switch back to api transport (best-effort)",
						"chat_id", chatID, "runner_id", runnerID, "err", err)
				}
				return // SwitchToNative already settled what this runner is now
			}
			// Torn down deliberately by something that replaces it with NOTHING —
			// retire, or a provider switch. For a runner with no PTY of its own
			// this view was its last process, and nothing else would ever carry
			// its row away.
			_, release, err := rs.spawns.Acquire(ctx, chatID)
			if err != nil {
				return
			}
			defer release()
			rs.exitProcesslessRunner(ctx, runnerID)
		})
	}
}
