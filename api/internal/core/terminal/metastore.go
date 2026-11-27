package terminal

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type SessionMeta struct {
	SessionID    string
	ChatID       string
	CWD          string
	Shell        string
	ProfileID    string
	State        string
	LastActiveAt time.Time
}

type SessionMetaStore interface {
	Save(
		ctx context.Context,
		meta SessionMeta,
	) error

	Delete(
		ctx context.Context,
		sessionID string,
	) error

	StorageDir(
		ctx context.Context,
		chatID string,
	) (string, error)

	List(
		ctx context.Context,
	) ([]domain.TerminalSession, error)
}
