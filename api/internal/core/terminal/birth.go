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

type engineBirth struct {
	Cols            int
	Rows            int
	ScrollbackLines int
	Blob            []byte
	Notice          []byte
}

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

func (e *terminalEngine) admit() error {
	if e.reaps.admit() {
		return nil
	}
	return ErrShuttingDown
}

func (e *terminalEngine) startReaper(ctx context.Context, ent *sessionEntry, s *session.Session) {
	go e.reap(context.WithoutCancel(ctx), ent, s)
}

func (e *terminalEngine) reap(ctx context.Context, ent *sessionEntry, s *session.Session) {
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
			slog.Warn("terminal: create: startup command not delivered", "session", id, "err", err)
			break
		}
	}
	return id, nil
}

func (e *terminalEngine) CreateCommand(
	ctx context.Context,
	chatID string,
	cwd string,
	argv []string,
	env []string,
	onExit func(Exit),
) (string, error) {
	if err := e.admit(); err != nil {
		return "", err
	}
	id := uuid.NewString()
	env = withTerminalDefaults(env)
	bg, fg := e.HostTheme()
	s, err := session.NewCommand(ctx, id, argv, cwd, env, 80, 24, 0, session.WithTheme(bg, fg))
	if err != nil {
		e.reaps.settle(false)
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
