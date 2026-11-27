package terminal

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/google/uuid"

	store "github.com/char2cs/crowbar/api/internal/adapter/store"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type Engine interface {
	Create(
		ctx context.Context,
		chatID string,
		workspaceDir string,
		prof *domain.TerminalProfile,
	) (sessionID string, err error)
	Kill(
		ctx context.Context,
		sessionID string,
	) error
	LoadPlaceholder(
		ctx context.Context,
		m engineterminal.SessionMeta,
		scrollback []byte,
	) error
}

type Usecase interface {
	CreateSession(
		ctx context.Context,
		chatID string,
		prof *domain.TerminalProfile,
	) (string, error)

	KillSession(
		ctx context.Context,
		sessionID string,
	) error

	ListProfiles(
		ctx context.Context,
	) ([]domain.TerminalProfile, error)

	CreateProfile(
		ctx context.Context,
		prof domain.TerminalProfile,
	) (domain.TerminalProfile, error)

	UpdateProfile(
		ctx context.Context,
		prof domain.TerminalProfile,
	) (domain.TerminalProfile, error)

	DeleteProfile(
		ctx context.Context,
		id string,
	) error

	RestorePersistedSessions(
		ctx context.Context,
	) error
}

type terminalUsecase struct {
	engine    Engine
	profiles  store.Store[domain.TerminalProfile, string]
	worktrees WorktreeResolver
	metaStore engineterminal.SessionMetaStore
}

func New(
	engine Engine,
	profiles store.Store[domain.TerminalProfile, string],
	worktrees WorktreeResolver,
	metaStore engineterminal.SessionMetaStore,
) Usecase {
	return &terminalUsecase{
		engine:    engine,
		profiles:  profiles,
		worktrees: worktrees,
		metaStore: metaStore,
	}
}

func (u *terminalUsecase) CreateSession(
	ctx context.Context,
	chatID string,
	prof *domain.TerminalProfile,
) (string, error) {
	ws, err := u.worktrees.Resolve(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("terminal: create session: resolve chat worktree: %w", err)
	}
	id, err := u.engine.Create(ctx, chatID, ws.WorktreePath, prof)
	if err != nil {
		return "", fmt.Errorf("terminal: create session: %w", err)
	}
	return id, nil
}

func (u *terminalUsecase) KillSession(
	ctx context.Context,
	sessionID string,
) error {
	if err := u.engine.Kill(ctx, sessionID); err != nil {
		return fmt.Errorf("terminal: kill session: %w", err)
	}
	return nil
}

func (u *terminalUsecase) ListProfiles(
	ctx context.Context,
) ([]domain.TerminalProfile, error) {
	list, err := u.profiles.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("terminal: list profiles: %w", err)
	}
	return list, nil
}

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

func (u *terminalUsecase) DeleteProfile(
	ctx context.Context,
	id string,
) error {
	if err := u.profiles.Delete(ctx, id); err != nil {
		return fmt.Errorf("terminal: delete profile: %w", err)
	}
	return nil
}

func (u *terminalUsecase) RestorePersistedSessions(ctx context.Context) error {
	if u.metaStore == nil {
		return nil
	}
	rows, err := u.metaStore.List(ctx)
	if err != nil {
		return fmt.Errorf("terminal: restore: list: %w", err)
	}

	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].LastActiveAt.After(rows[j].LastActiveAt)
	})
	limit := engineterminal.MaxTotalSessions()

	restored, dropped, evicted := 0, 0, 0
	for _, row := range rows {
		dir, dirErr := u.metaStore.StorageDir(ctx, row.ChatID)
		if dirErr != nil {
			if delErr := u.metaStore.Delete(ctx, row.SessionID); delErr != nil {
				slog.WarnContext(ctx, "terminal: restore: delete orphan", "component", "terminal", "session", row.SessionID, "err", delErr)
			}
			dropped++
			continue
		}

		if restored >= limit {
			u.evictPersistedRow(ctx, dir, row.SessionID)
			evicted++
			continue
		}

		scrollback := readScrollback(dir, row.SessionID)

		if loadErr := u.engine.LoadPlaceholder(ctx, engineterminal.SessionMeta{
			SessionID: row.SessionID,
			ChatID:    row.ChatID,
			CWD:       row.CWD,
			Shell:     row.Shell,
			ProfileID: row.ProfileID,
			State:     "suspended",
		}, scrollback); loadErr != nil {
			slog.WarnContext(ctx, "terminal: restore: load placeholder", "component", "terminal", "session", row.SessionID, "err", loadErr)
			continue
		}
		restored++
	}

	if restored+dropped+evicted > 0 {
		slog.InfoContext(ctx, "terminal: restored persisted sessions", "component", "terminal",
			"restored", restored, "dropped_orphans", dropped, "evicted_over_cap", evicted)
	}
	return nil
}

func (u *terminalUsecase) evictPersistedRow(
	ctx context.Context,
	dir string,
	sessionID string,
) {
	if delErr := u.metaStore.Delete(ctx, sessionID); delErr != nil {
		slog.WarnContext(ctx, "terminal: restore: delete over-cap row", "component", "terminal", "session", sessionID, "err", delErr)
	}
	path := filepath.Join(dir, sessionID+".buf")
	if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
		slog.WarnContext(ctx, "terminal: restore: remove over-cap buf", "component", "terminal", "session", sessionID, "err", rmErr)
	}
}

func readScrollback(dir, sessionID string) []byte {
	path := filepath.Join(dir, sessionID+".buf")
	data, err := os.ReadFile(path) //nolint:gosec // path controlled by callers
	if err != nil {
		return nil
	}
	return data
}
