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

// SessionMetaStore is the port through which the engine persists terminal-session
// metadata without importing GORM or the domain; the terminal usecase implements it
// and it is injected via Engine.SetMetaStore.
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

	// StorageDir returns the per-chat directory for scrollback .buf files. It takes the
	// owning chat id, never a workspace id, so sibling chats sharing a worktree never
	// write into each other's buffers; resolving chat to workspace is the store's job.
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
