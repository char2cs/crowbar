package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.byRun[runnerID]
	return v, ok
}

func (r *attachRegistry) drop(runnerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byRun, runnerID)
}

// AttachedTerminalSession answers the one question the read side needs: does
// runnerID currently have a native-view PTY live, and if so which terminal
// session IS it. A chat's own TerminalSession (the durable, event-sourced
// field) is unrelated while this is set — the redundant hooks-only PTY every
// api-transport spawn still forks (a separate, known gap) is NOT what a user
// who switched to the native view is looking at, and must not be what the
// chat DTO reports while they are.
func (rs *Runners) AttachedTerminalSession(runnerID string) (string, bool) {
	view, ok := rs.attached.get(runnerID)
	if !ok {
		return "", false
	}
	return view.termSessID, true
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
// that has never completed a turn: codex's own `codex resume {id}` (this
// capability's attach mechanism) needs a flushed rollout to load, and codex
// writes nothing for a thread until a turn completes. Attempting it anyway
// forks a process that dies within milliseconds — and with nothing catching
// that, the chat's terminal session silently fell back to the disconnected
// companion PTY every api-transport spawn still forks alongside a live
// connection (see the DTO fix in agent.go), not an error a caller could see.
// Confirmed live.
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
	defer rs.spawns.Lock(chatID)()

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
	rs.apiConns.drop(live.ID)

	argv := append([]string{binpath.Resolve(attachArgv[0])}, attachArgv[1:]...)
	termSessID, err := rs.term.CreateCommand(ctx, live.WorkspaceID, tctx.Cwd, argv, nil,
		rs.onAttachExit(chatID, live.ID))
	if err != nil {
		// The api connection is already gone; degrade to dormant rather than leave
		// the chat believing a connection is live when it is not — Resume revives it.
		return "", fmt.Errorf("agent: switch to terminal: fork native view: %w", err)
	}
	rs.attached.set(live.ID, attachedView{termSessID: termSessID, agent: agent, tctx: tctx})
	return termSessID, nil
}

// SwitchToNative reverses SwitchToTerminal: the native-view PTY is torn down
// and the api connection is re-established over the SAME session via
// applyAPITransport's own Resume path (EstablishSession is a no-op once a
// session id is already known — it just resumes). Idempotent: a chat with
// nothing attached returns nil, since there is nothing to switch back FROM.
func (rs *Runners) SwitchToNative(ctx context.Context, chatID string) error {
	defer rs.spawns.Lock(chatID)()

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

	if err := rs.term.TerminateGraceful(ctx, view.termSessID); err != nil &&
		!errors.Is(err, engineterminal.ErrSessionNotFound) {
		slog.WarnContext(ctx, "agent: switch to native: terminate native view (best-effort, continuing)",
			"runner_id", live.ID, "terminal_session_id", view.termSessID, "err", err)
	}
	// No resumeContext: this is switching VIEWS on one still-live session, not
	// resuming one that was ever actually away — view.tctx's own Context is
	// whatever the ORIGINAL spawn assembled and would only be a stale replay
	// of the same document on every terminal<->chat toggle if reused here.
	rs.applyAPITransport(ctx, live.ID, live.ProviderID, view.agent, view.tctx, &engineagents.SpawnPlan{}, "")
	return nil
}

// onAttachExit runs when the native-view PTY exits on its own — the user
// closed it, or its process died. Switching back to native automatically is
// the same "nothing left to show" recovery StopChat's own retire() gives a
// dead runner elsewhere: a chat left pointing at a terminal session that no
// longer exists is worse than one quietly resuming its api connection.
func (rs *Runners) onAttachExit(chatID, runnerID string) func() {
	return func() {
		if _, ok := rs.attached.get(runnerID); !ok {
			return // already switched back deliberately; nothing to reconcile
		}
		if err := rs.SwitchToNative(context.Background(), chatID); err != nil {
			slog.Error("agent: native view exited: switch back to api transport (best-effort)",
				"chat_id", chatID, "runner_id", runnerID, "err", err)
		}
	}
}
