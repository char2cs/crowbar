package terminal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/google/uuid"

	store "github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
)

// Engine is the PTY-session surface the terminal usecase passes through to.
type Engine interface {
	Create(
		ctx context.Context,
		workspaceID string,
		workspaceDir string,
		prof *domain.TerminalProfile,
	) (sessionID string, err error)
	Kill(
		ctx context.Context,
		sessionID string,
	) error
	// LoadPlaceholder registers a durable session as a PTY-less placeholder so
	// a subsequent Attach can restore it. Idempotent: no-op if session already
	// present. Called by RestorePersistedSessions at daemon startup.
	LoadPlaceholder(
		ctx context.Context,
		m engineterminal.SessionMeta,
		scrollback []byte,
	) error
}

// WorkspaceRepo is the workspace surface used to resolve a session's
// working directory.
type WorkspaceRepo interface {
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
}

// Usecase is the terminal session lifecycle + profile CRUD surface.
type Usecase interface {
	// CreateSession spawns a PTY session in the workspace's worktree directory.
	CreateSession(
		ctx context.Context,
		wsID string,
		prof *domain.TerminalProfile,
	) (string, error)

	// KillSession terminates a PTY session.
	KillSession(
		ctx context.Context,
		sessionID string,
	) error

	// ListProfiles returns every stored terminal profile.
	ListProfiles(
		ctx context.Context,
	) ([]domain.TerminalProfile, error)

	// CreateProfile mints an id and stores a new terminal profile.
	CreateProfile(
		ctx context.Context,
		prof domain.TerminalProfile,
	) (domain.TerminalProfile, error)

	// UpdateProfile overwrites an existing terminal profile.
	UpdateProfile(
		ctx context.Context,
		prof domain.TerminalProfile,
	) (domain.TerminalProfile, error)

	// DeleteProfile removes a terminal profile.
	DeleteProfile(
		ctx context.Context,
		id string,
	) error

	// RestorePersistedSessions reloads all durable terminal session rows from
	// the metastore as PTY-less placeholders in the engine. Orphaned rows
	// (workspace deleted) are reconciled away. Per-row errors are logged and
	// skipped; the overall call never fails the daemon startup. Must be called
	// after both the engine and the metastore are wired.
	RestorePersistedSessions(
		ctx context.Context,
	) error
}

type terminalUsecase struct {
	engine     Engine
	profiles   store.Store[domain.TerminalProfile, string]
	workspaces WorkspaceRepo
	metaStore  engineterminal.SessionMetaStore
}

// New builds a Usecase from the terminal engine, the profile store, the
// workspace repo, and the durable session metastore. metaStore may be nil; in
// that case RestorePersistedSessions is a no-op.
func New(
	engine Engine,
	profiles store.Store[domain.TerminalProfile, string],
	workspaces WorkspaceRepo,
	metaStore engineterminal.SessionMetaStore,
) Usecase {
	return &terminalUsecase{
		engine:     engine,
		profiles:   profiles,
		workspaces: workspaces,
		metaStore:  metaStore,
	}
}

// CreateSession spawns a PTY session in the workspace's worktree directory.
func (u *terminalUsecase) CreateSession(
	ctx context.Context,
	wsID string,
	prof *domain.TerminalProfile,
) (string, error) {
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return "", fmt.Errorf("terminal: create session: load workspace: %w", err)
	}
	id, err := u.engine.Create(ctx, wsID, ws.WorktreePath, prof)
	if err != nil {
		return "", fmt.Errorf("terminal: create session: %w", err)
	}
	return id, nil
}

// KillSession terminates a PTY session.
func (u *terminalUsecase) KillSession(
	ctx context.Context,
	sessionID string,
) error {
	if err := u.engine.Kill(ctx, sessionID); err != nil {
		return fmt.Errorf("terminal: kill session: %w", err)
	}
	return nil
}

// ListProfiles returns every stored terminal profile.
func (u *terminalUsecase) ListProfiles(
	ctx context.Context,
) ([]domain.TerminalProfile, error) {
	list, err := u.profiles.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("terminal: list profiles: %w", err)
	}
	return list, nil
}

// CreateProfile mints an id and stores a new terminal profile.
func (u *terminalUsecase) CreateProfile(
	ctx context.Context,
	prof domain.TerminalProfile,
) (domain.TerminalProfile, error) {
	prof.ID = uuid.NewString()
	if err := u.profiles.Save(ctx, prof); err != nil {
		return domain.TerminalProfile{}, fmt.Errorf("terminal: create profile: %w", err)
	}
	return prof, nil
}

// UpdateProfile overwrites an existing terminal profile.
func (u *terminalUsecase) UpdateProfile(
	ctx context.Context,
	prof domain.TerminalProfile,
) (domain.TerminalProfile, error) {
	if prof.ID == "" {
		return domain.TerminalProfile{}, fmt.Errorf("terminal: update profile: missing id")
	}
	if err := u.profiles.Save(ctx, prof); err != nil {
		return domain.TerminalProfile{}, fmt.Errorf("terminal: update profile: %w", err)
	}
	return prof, nil
}

// DeleteProfile removes a terminal profile.
func (u *terminalUsecase) DeleteProfile(
	ctx context.Context,
	id string,
) error {
	if err := u.profiles.Delete(ctx, id); err != nil {
		return fmt.Errorf("terminal: delete profile: %w", err)
	}
	return nil
}

// RestorePersistedSessions reloads all durable terminal session rows from the
// metastore as PTY-less placeholders. Orphaned rows (workspace no longer
// resolvable) are deleted from the store. Per-row errors are logged and
// skipped; the overall call never returns a fatal error.
func (u *terminalUsecase) RestorePersistedSessions(ctx context.Context) error {
	if u.metaStore == nil {
		return nil
	}
	rows, err := u.metaStore.List(ctx)
	if err != nil {
		return fmt.Errorf("terminal: restore: list: %w", err)
	}

	// Most-recent-first so the cap keeps the freshest sessions and discards the
	// stalest. The engine enforces the same ceiling in-memory; restoring more
	// than it can hold would just trigger immediate eviction, so we bound here
	// and also delete the excess .buf/row to keep DISK usage bounded too.
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].LastActiveAt.After(rows[j].LastActiveAt)
	})
	limit := engineterminal.MaxTotalSessions()

	restored, dropped, evicted := 0, 0, 0
	for _, row := range rows {
		dir, dirErr := u.metaStore.StorageDir(ctx, row.WorkspaceID)
		if dirErr != nil {
			// Workspace deleted or unresolvable — reconcile orphan away.
			if delErr := u.metaStore.Delete(ctx, row.SessionID); delErr != nil {
				_, _ = fmt.Fprintf(os.Stderr,
					"terminal: restore: delete orphan %s: %v\n", row.SessionID, delErr)
			}
			dropped++
			continue
		}

		// Over the cap: this is one of the stalest rows. Delete its row and .buf
		// so the on-disk backlog cannot grow without bound across restarts.
		if restored >= limit {
			u.evictPersistedRow(ctx, dir, row.SessionID)
			evicted++
			continue
		}

		scrollback := readScrollback(dir, row.SessionID)

		if loadErr := u.engine.LoadPlaceholder(ctx, engineterminal.SessionMeta{
			SessionID:   row.SessionID,
			WorkspaceID: row.WorkspaceID,
			CWD:         row.CWD,
			Shell:       row.Shell,
			ProfileID:   row.ProfileID,
			State:       "suspended",
		}, scrollback); loadErr != nil {
			_, _ = fmt.Fprintf(os.Stderr,
				"terminal: restore: load placeholder %s: %v\n", row.SessionID, loadErr)
			continue
		}
		restored++
	}

	_, _ = fmt.Fprintf(os.Stderr,
		"terminal: restore: restored=%d dropped_orphans=%d evicted_over_cap=%d\n",
		restored, dropped, evicted)
	return nil
}

// evictPersistedRow deletes a persisted session's meta row and its scrollback
// .buf file when it falls beyond the restore cap, bounding on-disk growth.
func (u *terminalUsecase) evictPersistedRow(
	ctx context.Context,
	dir string,
	sessionID string,
) {
	if delErr := u.metaStore.Delete(ctx, sessionID); delErr != nil {
		_, _ = fmt.Fprintf(os.Stderr,
			"terminal: restore: delete over-cap row %s: %v\n", sessionID, delErr)
	}
	path := filepath.Join(dir, sessionID+".buf")
	if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
		_, _ = fmt.Fprintf(os.Stderr,
			"terminal: restore: remove over-cap buf %s: %v\n", sessionID, rmErr)
	}
}

// readScrollback reads <dir>/<sessionID>.buf, returning nil when the file is
// absent (session had no persisted scrollback). Real IO errors are silently
// swallowed and treated as missing so a corrupt buf never blocks startup.
func readScrollback(dir, sessionID string) []byte {
	path := filepath.Join(dir, sessionID+".buf")
	data, err := os.ReadFile(path) //nolint:gosec // path controlled by callers
	if err != nil {
		return nil
	}
	return data
}
