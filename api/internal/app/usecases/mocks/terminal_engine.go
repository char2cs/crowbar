package mocks

import (
	"context"

	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type TerminalEngine struct {
	CreateFn          func(ctx context.Context, chatID, dir string, prof *domain.TerminalProfile) (string, error)
	KillFn            func(ctx context.Context, sessionID string) error
	LoadPlaceholderFn func(ctx context.Context, m engineterminal.SessionMeta, scrollback []byte) error
}

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
