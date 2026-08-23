package terminal

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type sessionMetaStoreImpl struct {
	workspaces  WorkspaceRepo
	sessions    store.Store[domain.TerminalSession, string]
	crowbarHome func() (string, error)
}

// NewSessionMetaStore constructs a SessionMetaStore backed by the global
// view.db TerminalSession store and the workspace repository. It implements
// engineterminal.SessionMetaStore and is injected into the engine via
// Engine.SetMetaStore after both layers are constructed.
func NewSessionMetaStore(
	workspaces WorkspaceRepo,
	sessions store.Store[domain.TerminalSession, string],
	crowbarHome func() (string, error),
) engineterminal.SessionMetaStore {
	return &sessionMetaStoreImpl{
		workspaces:  workspaces,
		sessions:    sessions,
		crowbarHome: crowbarHome,
	}
}

// Save upserts the session metadata row, resolving (projectID, repoID) from
// the workspace repo and preserving CreatedAt when a row already exists.
func (s *sessionMetaStoreImpl) Save(
	ctx context.Context,
	meta engineterminal.SessionMeta,
) error {
	ws, err := s.workspaces.Get(ctx, meta.WorkspaceID)
	if err != nil {
		return fmt.Errorf("terminal: metastore: save: resolve workspace: %w", err)
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
		WorkspaceID:  meta.WorkspaceID,
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

// Delete removes the session metadata row. A missing row is not an error.
func (s *sessionMetaStoreImpl) Delete(
	ctx context.Context,
	sessionID string,
) error {
	if err := s.sessions.Delete(ctx, sessionID); err != nil {
		return fmt.Errorf("terminal: metastore: delete: %w", err)
	}
	return nil
}

// StorageDir resolves the per-workspace storage directory by looking up
// (projectID, repoID) from the workspace repo, then delegating to
// worktreepath.StorageDir. It must never use WorktreePath for this purpose
// (home workspaces have no repo path).
func (s *sessionMetaStoreImpl) StorageDir(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	home, err := s.crowbarHome()
	if err != nil {
		return "", fmt.Errorf("terminal: metastore: storage dir: home: %w", err)
	}

	ws, err := s.workspaces.Get(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("terminal: metastore: storage dir: resolve workspace: %w", err)
	}

	return worktreepath.StorageDir(home, ws.ProjectID, ws.RepoID, workspaceID), nil
}

// List returns all persisted terminal session rows. Called at daemon start
// by RestorePersistedSessions to reload placeholder sessions into the engine.
func (s *sessionMetaStoreImpl) List(
	ctx context.Context,
) ([]domain.TerminalSession, error) {
	rows, err := s.sessions.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("terminal: metastore: list: %w", err)
	}
	return rows, nil
}
