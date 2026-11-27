package terminal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type WorktreeResolver interface {
	Resolve(ctx context.Context, chatID string) (domain.Workspace, error)
}

type WorkspaceReader interface {
	Get(ctx context.Context, id string) (domain.Workspace, error)
}

type sessionMetaStoreImpl struct {
	worktrees   WorktreeResolver
	workspaces  WorkspaceReader
	sessions    store.Store[domain.TerminalSession, string]
	crowbarHome func() (string, error)
}

func NewSessionMetaStore(
	worktrees WorktreeResolver,
	workspaces WorkspaceReader,
	sessions store.Store[domain.TerminalSession, string],
	crowbarHome func() (string, error),
) engineterminal.SessionMetaStore {
	return &sessionMetaStoreImpl{
		worktrees:   worktrees,
		workspaces:  workspaces,
		sessions:    sessions,
		crowbarHome: crowbarHome,
	}
}

func (s *sessionMetaStoreImpl) owner(ctx context.Context, ownerID string) (domain.Workspace, error) {
	ws, err := s.worktrees.Resolve(ctx, ownerID)
	if !errors.Is(err, worktree.ErrNoWorktreeInAncestry) {
		return ws, err
	}
	home, homeErr := s.workspaces.Get(ctx, ownerID)
	if homeErr != nil || home.Kind != domain.WorkspaceKindHome {
		return domain.Workspace{}, err
	}
	return home, nil
}

func (s *sessionMetaStoreImpl) Save(
	ctx context.Context,
	meta engineterminal.SessionMeta,
) error {
	ws, err := s.owner(ctx, meta.ChatID)
	if err != nil {
		return fmt.Errorf("terminal: metastore: save: resolve chat worktree: %w", err)
	}

	existing, err := s.sessions.FindByKey(ctx, meta.SessionID)
	if err != nil {
		return fmt.Errorf("terminal: metastore: save: lookup: %w", err)
	}

	createdAt := time.Now().UTC()
	if existing != nil {
		createdAt = existing.CreatedAt
	}

	row := domain.TerminalSession{
		SessionID:    meta.SessionID,
		ChatID:       meta.ChatID,
		ProjectID:    ws.ProjectID,
		RepoID:       ws.RepoID,
		CWD:          meta.CWD,
		Shell:        meta.Shell,
		ProfileID:    meta.ProfileID,
		State:        meta.State,
		CreatedAt:    createdAt,
		LastActiveAt: meta.LastActiveAt,
	}
	if err := s.sessions.Save(ctx, row); err != nil {
		return fmt.Errorf("terminal: metastore: save: %w", err)
	}
	return nil
}

func (s *sessionMetaStoreImpl) Delete(
	ctx context.Context,
	sessionID string,
) error {
	if err := s.sessions.Delete(ctx, sessionID); err != nil {
		return fmt.Errorf("terminal: metastore: delete: %w", err)
	}
	return nil
}

func (s *sessionMetaStoreImpl) StorageDir(
	ctx context.Context,
	chatID string,
) (string, error) {
	home, err := s.crowbarHome()
	if err != nil {
		return "", fmt.Errorf("terminal: metastore: storage dir: home: %w", err)
	}

	ws, err := s.owner(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("terminal: metastore: storage dir: resolve chat worktree: %w", err)
	}

	return worktreepath.StorageDir(home, ws.ProjectID, ws.RepoID, chatID), nil
}

func (s *sessionMetaStoreImpl) List(
	ctx context.Context,
) ([]domain.TerminalSession, error) {
	rows, err := s.sessions.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("terminal: metastore: list: %w", err)
	}
	return rows, nil
}
