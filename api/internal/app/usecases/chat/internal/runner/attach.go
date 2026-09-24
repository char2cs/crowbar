package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
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

// ErrNativeViewNotYetAvailable is SwitchToTerminal's refusal for a session
// that has never completed a turn: the attach resume needs a flushed rollout,
// and a provider writes none until a turn completes.
var ErrNativeViewNotYetAvailable = fmt.Errorf("agent: provider has no completed turn yet to show its native view of: %w", apperr.ErrConflict)

// SwitchToTerminal hands chatID's live turn over to its provider's own native
// view — an idle-only capability (design spec's non-hotswap state): the api
// connection is torn down and a bare resume of the SAME session is forked as
// a REAL, ordinary hooks-transport PTY, wired with the exact same
// MCPInject/ConfigInjection steps any hooks-attached CLI gets (APIAttachArgv),
// so it reports back into Crowbar's ledger exactly like claude's always-live
// PTY does — nothing here is a degraded or disconnected view.
//
// Returns the new terminal session id, for the caller to hand the frontend so
// it can point its existing terminal-rendering path at it — the same one a
// hotswap provider's terminal view already uses.
func (rs *Runners) SwitchToTerminal(ctx context.Context, chatID string) (string, error) {
	_, release, err := rs.spawns.Acquire(ctx, chatID)
	if err != nil {
		return "", err
	}
	defer release()

	live, err := rs.runnerStore.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return "", fmt.Errorf("agent: switch to terminal: %w", apperr.ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("agent: switch to terminal: live runner: %w", err)
	}
	// Idempotent: a caller that already switched (or a retry racing its own
	// earlier success) gets the SAME session back rather than ErrNoNativeTerminal
	// — there is no live api connection to check attach against once attached,
	// which is the expected shape here, not a failure.
	if already, ok := rs.attached.get(live.ID); ok {
		return already.termSessID, nil
	}
	conn, ok := rs.apiConns.get(live.ID)
	if !ok {
		return "", ErrNoNativeTerminal
	}
	attachArgv, ok := conn.agent.APIAttachArgv(conn.tctx)
	if !ok {
		return "", ErrNoNativeTerminal
	}
	working, err := rs.turns.ChatWorking(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: switch to terminal: chat working: %w", err)
	}
	if working {
		return "", ErrTurnInProgress
	}
	// live.CurrentSession, NOT conn.tctx.Session: OpenTurn/CloseTurn (internal/turn)
	// always stamp agent_turns.session_id from the runner row's durable
	// CurrentSession, never from conn.tctx.Session — a copy apiconn.go sets once,
	// at establish, and never reassigns again. The two usually agree, but nothing
	// resyncs them after that, and checking the connection's stale copy here
	// refused a session that HAD completed a turn (confirmed live), because the
	// row it needed was written under CurrentSession, not this copy. codex writes
	// nothing for a thread until a turn against it completes, so a session that
	// has never closed one has no rollout for `codex resume` to load. Checked
	// here, after `working`: a turn currently in flight (the first one, not yet
	// completed) must still report ErrTurnInProgress, the more actionable refusal.
	_, everTurned, err := rs.activity.LastTurnForSession(ctx, chatID, live.ProviderID, live.CurrentSession)
	if err != nil {
		return "", fmt.Errorf("agent: switch to terminal: check session history: %w", err)
	}
	if !everTurned {
		return "", ErrNativeViewNotYetAvailable
	}

	agent, tctx := conn.agent, conn.tctx // capture before drop erases the entry
	// The native view below becomes this runner's process, so killing the
	// connection here is a HANDOVER, not a death — see handOverAPIConn. Marked
	// before the drop: the watcher can observe the process die the instant
	// drop signals it, and a PTY-less runner would otherwise be reconciled
	// away underneath the view it is switching to.
	rs.handOverAPIConn(live.ID)
	rs.apiConns.drop(live.ID)

	argv := append([]string{binpath.Resolve(attachArgv[0])}, attachArgv[1:]...)
	// Keyed by the CHAT, not the runner row's workspace: the native view is a
	// PTY this chat owns, and live.WorkspaceID is empty for a chat with no
	// worktree of its own. tctx.Cwd stays the separately-resolved directory.
	//
	// os.Environ(), the same base every ordinary spawn plans from — NOT nil.
	// CreateCommand takes the env verbatim, so nil left the native view with
	// three variables and no PATH or HOME: measured live, every hook
	// APIAttachArgv wires died with exit 127 and `crowbar mcp` never started,
	// which is the exact "reports NOTHING back to Crowbar's ledger" that
	// method's own doc says this path exists to prevent.
	termSessID, err := rs.term.CreateCommand(ctx, chatID, tctx.Cwd, argv, os.Environ(),
		rs.onAttachExit(chatID, live.ID))
	if err != nil {
		// The api connection is already gone; degrade to dormant rather than leave
		// the chat believing a connection is live when it is not — Resume revives it.
		return "", fmt.Errorf("agent: switch to terminal: fork native view: %w", err)
	}
	rs.attached.set(live.ID, attachedView{termSessID: termSessID, agent: agent, tctx: tctx})
	rs.moveSurface(ctx, chatID, live.ID, engineagents.SurfaceTerminal)
	rs.touch(ctx, chatID) // the pane's terminal session is now the native view
	return termSessID, nil
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

// SwitchToNative reverses SwitchToTerminal: the native-view PTY is torn down
// and the api connection is re-established over the SAME session via
// applyAPITransport's own Resume path (EstablishSession is a no-op once a
// session id is already known — it just resumes). Idempotent: a chat with
// nothing attached returns nil, since there is nothing to switch back FROM.
func (rs *Runners) SwitchToNative(ctx context.Context, chatID string) error {
	_, release, err := rs.spawns.Acquire(ctx, chatID)
	if err != nil {
		return err
	}
	defer release()

	live, err := rs.runnerStore.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("agent: switch to native: live runner: %w", err)
	}
	view, ok := rs.attached.get(live.ID)
	if !ok {
		return nil
	}
	rs.attached.drop(live.ID)
	rs.touch(ctx, chatID)

	if err := rs.term.TerminateGraceful(ctx, view.termSessID); err != nil &&
		!errors.Is(err, engineterminal.ErrSessionNotFound) {
		slog.WarnContext(ctx, "agent: switch to native: terminate native view (best-effort, continuing)",
			"runner_id", live.ID, "terminal_session_id", view.termSessID, "err", err)
	}
	// No resumeContext: this is switching VIEWS on one still-live session, not
	// resuming one that was ever actually away — view.tctx's own Context is
	// whatever the ORIGINAL spawn assembled and would only be a stale replay
	// of the same document on every terminal<->chat toggle if reused here.
	// No plan to point at attach either: there is no PTY being forked here, and
	// the native view this just replaced has already been torn down above.
	_ = rs.applyAPITransport(ctx, live.ID, live.ProviderID, view.agent, view.tctx, "")
	// A runner with a PTY of its own is carried by it either way. One without
	// has just swapped processes again: the re-established connection is the
	// new one, and if none came back it has none at all. Still under the spawn
	// gate, so the view's own exit callback cannot race this answer.
	if live.TerminalSession == "" && !rs.rearmAPIConnExit(live.ID, view.tctx) {
		rs.exitProcesslessRunner(ctx, live.ID)
	}
	rs.moveSurface(ctx, chatID, live.ID, engineagents.SurfaceChat)
	return nil
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
