package handlers

import (
	"context"
	"image/color"

	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type WSConn = engineterminal.WSConn

type TerminalEngine interface {
	Create(
		ctx context.Context,
		chatID string,
		workspaceDir string,
		prof *domain.TerminalProfile,
	) (sessionID string, err error)
	Kill(
		ctx context.Context,
		sessionID string,
	) error
	SessionExists(
		ctx context.Context,
		sessionID string,
	) bool
	Attach(
		ctx context.Context,
		sessionID string,
		conn WSConn,
	) error
	ListSessionsForChat(
		chatID string,
	) []string
	StateOf(sessionID string) (string, bool)
	SetHostTheme(
		bg color.Color,
		fg color.Color,
	)
	HostTheme() (bg, fg color.Color)
}

type ProfileStore interface {
	FindAll(ctx context.Context) ([]domain.TerminalProfile, error)
	FindByKey(ctx context.Context, id string) (*domain.TerminalProfile, error)
	Save(ctx context.Context, item domain.TerminalProfile) error
	Delete(ctx context.Context, id string) error
}

type Handlers struct {
	termEng      TerminalEngine
	profileStore ProfileStore
}

func New(
	termEng TerminalEngine,
	profileStore ProfileStore,
) *Handlers {
	return &Handlers{
		termEng:      termEng,
		profileStore: profileStore,
	}
}
