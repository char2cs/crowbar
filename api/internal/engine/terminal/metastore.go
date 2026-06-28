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
	WorkspaceID  string
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

	// StorageDir returns the per-workspace storage directory where terminal
	// scrollback files (.buf) are written. It resolves the workspace to its
	// (projectID, repoID) pair and delegates to worktreepath.StorageDir.
	StorageDir(
		ctx context.Context,
		workspaceID string,
	) (string, error)

	// List returns all persisted terminal session rows. Called at daemon start
	// by RestorePersistedSessions to reload placeholder sessions into the engine.
	List(
		ctx context.Context,
	) ([]domain.TerminalSession, error)
}
