package terminal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/profile"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/session"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// engineBirth is either create (Blob == nil; zero sizes mean defaults) or restore (Blob is the
// persisted CRWB1 screen, plus an optional Notice injected before any client attaches).
type engineBirth struct {
	Cols            int
	Rows            int
	ScrollbackLines int
	Blob            []byte
	Notice          []byte
}

// spawnShell starts a live shell session. The host theme is seeded at birth so the model
// answers OSC 10/11 truthfully before the process can ask. The caller admits the birth
// first, then registers the session, calls startReaper and settles the birth.
func (e *terminalEngine) spawnShell(ctx context.Context, id, shell, cwd, profileID string, b engineBirth) (*session.Session, error) {
	bg, fg := e.HostTheme()
	var (
		s   *session.Session
		err error
	)
	if b.Blob != nil {
		s, err = session.NewRestored(ctx, id, shell, cwd, profileID, ptyEnv(), b.Blob, session.WithTheme(bg, fg))
	} else {
		s, err = session.New(ctx, id, shell, cwd, profileID, ptyEnv(), b.Cols, b.Rows, b.ScrollbackLines,
			session.WithTheme(bg, fg))
	}
	if err != nil {
		return nil, err
	}
	if len(b.Notice) > 0 {
		s.InjectLocal(b.Notice)
	}
	return s, nil
}

// admit opens a birth BEFORE any child is started, refusing once Shutdown has begun
// draining: a session born after the kill loop has walked the registry would never be
// reaped. Every successful admit is paired with exactly one e.reaps.settle.
func (e *terminalEngine) admit() error {
	if e.reaps.admit() {
		return nil
	}
	return ErrShuttingDown
}

// startReaper runs under WithoutCancel of the birth context: the process outlives the request,
// but keeps its trace/log values. It is joined by e.reaps, which Shutdown drains.
func (e *terminalEngine) startReaper(ctx context.Context, ent *sessionEntry, s *session.Session) {
	go e.reap(context.WithoutCancel(ctx), ent, s)
}

// reap waits for s to die and, if s is still the entry's live session, moves the entry to
// Exited. If a Kill or suspend already moved the entry on, it has nothing left to do.
func (e *terminalEngine) reap(ctx context.Context, ent *sessionEntry, s *session.Session) {
	// Registered FIRST so it runs LAST: even a panic recovered below must retire this
	// reap, or Shutdown's drain would wait on a goroutine that is already gone.
	defer e.reaps.done()
	defer safego.Recover("terminal.reap")
	<-s.Done()
	ent.mu.Lock()
	var fx effects
	if ent.state == stateLive && ent.sess.Load() == s {
		fx = e.endLocked(ctx, ent, stateExited, s.ExitCode())
	}
	ent.mu.Unlock()
	fx.run()
}

func (e *terminalEngine) Create(
	ctx context.Context,
	chatID string,
	workspaceDir string,
	prof *domain.TerminalProfile,
) (string, error) {
	resolved := profile.Resolve(prof, workspaceDir)
	id := uuid.NewString()

	if err := e.admit(); err != nil {
		return "", fmt.Errorf("terminal: create: %w", err)
	}
	// Create births at the historical 80×24 default; a fresh attach's first resize
	// reshapes both PTY and model.
	s, err := e.spawnShell(ctx, id, resolved.Shell, resolved.CWD, "", engineBirth{})
	if err != nil {
		e.reaps.settle(false)
		return "", fmt.Errorf("terminal: create: %w", err)
	}
	ent := &sessionEntry{
		id:     id,
		chatID: chatID,
		state:  stateLive,
		shell:  resolved.Shell,
		cwd:    resolved.CWD,
	}
	ent.sess.Store(s)
	e.register(ent)
	e.startReaper(ctx, ent, s)
	e.reaps.settle(true)

	for _, cmd := range resolved.Startup {
		if err := s.Write([]byte(cmd + "\n")); err != nil {
			// The PTY is alive; only the profile's startup command failed to reach it.
			slog.Warn("terminal: create: startup command not delivered", "session", id, "err", err)
			break
		}
	}
	return id, nil
}

// CreateCommand spawns an explicit argv+env as a registered session. onExit, if non-nil,
// is invoked exactly once when the session reaches Exited — see the Engine interface.
func (e *terminalEngine) CreateCommand(
	ctx context.Context,
	chatID string,
	cwd string,
	argv []string,
	env []string,
	onExit func(Exit),
) (string, error) {
	// A vendor CLI whose reaper never runs is strictly worse than one never spawned: its
	// onExit is the ONLY thing that records the runner's death.
	if err := e.admit(); err != nil {
		return "", err
	}
	id := uuid.NewString()
	// CreateCommand takes the caller's env verbatim, so under launchd TERM and the locale
	// are absent; backfill the terminal defaults for any keys not already set.
	env = withTerminalDefaults(env)
	bg, fg := e.HostTheme()
	s, err := session.NewCommand(ctx, id, argv, cwd, env, 80, 24, 0, session.WithTheme(bg, fg))
	if err != nil {
		e.reaps.settle(false)
		// exec.ErrNotFound means argv[0] is not installed / not executable — a fact about
		// the USER'S MACHINE, not a server fault.
		if errors.Is(err, exec.ErrNotFound) {
			return "", fmt.Errorf("%w: %s", ErrCommandNotFound, argv[0])
		}
		return "", fmt.Errorf("terminal: create command: %w", err)
	}
	ent := &sessionEntry{
		id:      id,
		chatID:  chatID,
		command: true,
		state:   stateLive,
		shell:   s.Shell(),
		cwd:     cwd,
		onExit:  onExit,
	}
	ent.sess.Store(s)
	e.register(ent)
	e.startReaper(ctx, ent, s)
	e.reaps.settle(true)
	return id, nil
}
