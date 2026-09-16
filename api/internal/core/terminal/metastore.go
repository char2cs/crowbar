package terminal

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SessionMeta carries the durable metadata the engine writes for a single
// terminal session. It is intentionally free of GORM/domain types so the
// engine package never imports the persistence or usecase layers.
type SessionMeta struct {
	SessionID    string
	ChatID       string
	CWD          string
	Shell        string
	ProfileID    string
	State        string
	LastActiveAt time.Time
}

// SessionMetaStore is the port through which the engine persists and removes
// terminal-session metadata without importing GORM, the domain layer, or any
// usecase package. The implementation lives in the terminal usecase and is
// injected via Engine.SetMetaStore after both layers are constructed.
type SessionMetaStore interface {
	// Save upserts the session metadata row, preserving CreatedAt on update.
	Save(
		ctx context.Context,
		meta SessionMeta,
	) error

	// Delete removes the session metadata row. A missing row is not an error.
	Delete(
		ctx context.Context,
		sessionID string,
	) error

	// StorageDir returns the per-chat storage directory where terminal
	// scrollback files (.buf) are written. It resolves the chat to the
	// workspace behind its worktree for the (projectID, repoID) pair and
	// delegates to worktreepath.StorageDir.
	//
	// The engine passes the OWNING CHAT id, never a workspace id: scrollback
	// belongs to the session, the session belongs to one chat, and two sibling
	// chats sharing a worktree must not write into one another's .buf files.
	// Resolving chat → workspace is this store's job precisely so the engine
	// never has to hold a workspace id it would then be tempted to scope by.
	StorageDir(
		ctx context.Context,
		chatID string,
	) (string, error)

	// List returns all persisted terminal session rows. Called at daemon start
	// by RestorePersistedSessions to reload placeholder sessions into the engine.
	List(
		ctx context.Context,
	) ([]domain.TerminalSession, error)
}
