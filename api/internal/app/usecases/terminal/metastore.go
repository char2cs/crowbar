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

// WorktreeResolver resolves a chat id to the workspace behind the worktree it
// reads and writes through (spec 2026-09-02-chat-scoped-api-design §3).
//
// Declared HERE, where it is consumed, rather than imported from
// usecases/worktree (law 4): the metastore needs exactly one method, and the
// container is the only place that knows which concrete value satisfies it
// (law 6).
//
// The metastore needs it because a terminal session is owned by a CHAT while
// its on-disk scrollback still has to land under the owning project/repo —
// two different questions about one session, and the chat id is the only one
// of the two the engine carries.
type WorktreeResolver interface {
	Resolve(ctx context.Context, chatID string) (domain.Workspace, error)
}

type sessionMetaStoreImpl struct {
	worktrees   WorktreeResolver
	sessions    store.Store[domain.TerminalSession, string]
	crowbarHome func() (string, error)
}

// NewSessionMetaStore constructs a SessionMetaStore backed by the global
// view.db TerminalSession store and the chat→worktree resolver. It implements
// engineterminal.SessionMetaStore and is injected into the engine via
// Engine.SetMetaStore after both layers are constructed.
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

// StorageDir resolves the per-CHAT storage directory: it resolves the chat to
// the workspace behind its worktree for (projectID, repoID), then delegates to
// worktreepath.StorageDir keyed by the chat id.
//
// The leaf segment is the CHAT, not the workspace, and that is the point: two
// sibling chats sharing one worktree resolve the same (projectID, repoID) and
// would otherwise share a scrollback directory, where a .buf is named by
// session id alone. Keying the directory by owner keeps one chat's suspended
// scrollback unreadable and undeletable by the other.
//
// It must never use WorktreePath for the project/repo pair (home workspaces
// have no repo path).
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
