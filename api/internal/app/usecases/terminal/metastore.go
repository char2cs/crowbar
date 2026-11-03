package terminal

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// WorktreeResolver resolves a chat id to the workspace behind its worktree: sessions are owned by chats,
// but scrollback lands under the owning project/repo. Declared here, where consumed, so the container
// picks the concrete value.
type WorktreeResolver interface {
	Resolve(ctx context.Context, chatID string) (domain.Workspace, error)
}

type sessionMetaStoreImpl struct {
	worktrees   WorktreeResolver
	sessions    store.Store[domain.TerminalSession, string]
	crowbarHome func() (string, error)
}

// NewSessionMetaStore builds the engine's SessionMetaStore over view.db's TerminalSession store and the
// chat→worktree resolver; it is injected via Engine.SetMetaStore after both layers exist.
func NewSessionMetaStore(
	worktrees WorktreeResolver,
	sessions store.Store[domain.TerminalSession, string],
	crowbarHome func() (string, error),
) engineterminal.SessionMetaStore {
	return &sessionMetaStoreImpl{
		worktrees:   worktrees,
		sessions:    sessions,
		crowbarHome: crowbarHome,
	}
}

// Save upserts the session metadata row, resolving (projectID, repoID) from
// the chat's worktree and preserving CreatedAt when a row already exists.
func (s *sessionMetaStoreImpl) Save(
	ctx context.Context,
	meta engineterminal.SessionMeta,
) error {
	ws, err := s.worktrees.Resolve(ctx, meta.ChatID)
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

// StorageDir resolves the per-chat storage dir under the chat's (projectID, repoID). It is keyed by chat,
// not workspace, so sibling chats sharing a worktree cannot read or delete each other's scrollback; it
// never uses WorktreePath (home workspaces have no repo path).
func (s *sessionMetaStoreImpl) StorageDir(
	ctx context.Context,
	chatID string,
) (string, error) {
	home, err := s.crowbarHome()
	if err != nil {
		return "", fmt.Errorf("terminal: metastore: storage dir: home: %w", err)
	}

	ws, err := s.worktrees.Resolve(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("terminal: metastore: storage dir: resolve chat worktree: %w", err)
	}

	return worktreepath.StorageDir(home, ws.ProjectID, ws.RepoID, chatID), nil
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
