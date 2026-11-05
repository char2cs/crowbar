// Package handlers contains the HTTP and WebSocket handler logic for terminal endpoints.
package handlers

import (
	"context"
	"image/color"

	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// WSConn is the WebSocket abstraction used by the terminal engine Attach method.
// It is an alias for engineterminal.WSConn so the concrete engine satisfies
// TerminalEngine without a wrapping adapter.
type WSConn = engineterminal.WSConn

// TerminalEngine is the subset of the terminal engine surface used by the handlers.
type TerminalEngine interface {
	// Create spawns a PTY owned by chatID, running in workspaceDir. The two
	// are different questions: the owner is the chat, the directory is the
	// worktree that chat resolved to — one sibling chats may share.
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
	// StateOf returns the current lifecycle state ("active", "detached",
	// "suspended") for the given session and true; ("", false) if not found.
	StateOf(sessionID string) (string, bool)
	// SetHostTheme records the host's default colours as the OSC 10/11 answers every later
	// session is born with; the per-session push arrives after the process has already asked.
	SetHostTheme(
		bg color.Color,
		fg color.Color,
	)
}

// ProfileStore is the CRUD surface for terminal profiles.
type ProfileStore interface {
	FindAll(ctx context.Context) ([]domain.TerminalProfile, error)
	FindByKey(ctx context.Context, id string) (*domain.TerminalProfile, error)
	Save(ctx context.Context, item domain.TerminalProfile) error
	Delete(ctx context.Context, id string) error
}

// Handlers holds the dependencies shared across all terminal handler methods. It has no workspace
// reader: the chat group's resolveChatWorktree middleware already put the workspace in reqscope.
type Handlers struct {
	termEng      TerminalEngine
	profileStore ProfileStore
}

// New returns an initialised Handlers.
func New(
	termEng TerminalEngine,
	profileStore ProfileStore,
) *Handlers {
	return &Handlers{
		termEng:      termEng,
		profileStore: profileStore,
	}
}
