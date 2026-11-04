package mocks

import (
	"context"

	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TerminalEngine is a fake of the terminal-engine surface used by the terminal
// usecase. Create's first id is the OWNING CHAT; the directory it runs in is a
// separate lookup the usecase makes against the chat's worktree.
type TerminalEngine struct {
	CreateFn          func(ctx context.Context, chatID, dir string, prof *domain.TerminalProfile) (string, error)
	KillFn            func(ctx context.Context, sessionID string) error
	LoadPlaceholderFn func(ctx context.Context, m engineterminal.SessionMeta, scrollback []byte) error
}

// NewTerminalEngine returns an empty TerminalEngine.
func NewTerminalEngine() *TerminalEngine {
	return &TerminalEngine{}
}

func (e *TerminalEngine) Create(
	ctx context.Context,
	chatID string,
	workspaceDir string,
	prof *domain.TerminalProfile,
) (string, error) {
	return e.CreateFn(ctx, chatID, workspaceDir, prof)
}

func (e *TerminalEngine) Kill(
	ctx context.Context,
	sessionID string,
) error {
	return e.KillFn(ctx, sessionID)
}

// LoadPlaceholder is a no-op by default; set LoadPlaceholderFn to override.
func (e *TerminalEngine) LoadPlaceholder(
	ctx context.Context,
	m engineterminal.SessionMeta,
	scrollback []byte,
) error {
	if e.LoadPlaceholderFn != nil {
		return e.LoadPlaceholderFn(ctx, m, scrollback)
	}
	return nil
}
