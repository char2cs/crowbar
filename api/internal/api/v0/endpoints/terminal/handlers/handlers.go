// Package handlers contains the HTTP and WebSocket handler logic for terminal endpoints.
package handlers

import (
	"context"
	"image/color"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
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
	// SetHostTheme records the host terminal's default colours as the OSC 10/11 query
	// answers every subsequently spawned session is BORN with — the half of theme
	// propagation the per-session push cannot reach, since the process is already
	// running before any client attaches.
	SetHostTheme(
		bg color.Color,
		fg color.Color,
	)
}

// TerminalBroadcaster receives terminal-session lifecycle DTOs so the v0
// Broadcaster[TerminalSessionDTO] can fan them out to subscribed clients. The
// session create/kill paths push active/ended frames through it (D2).
type TerminalBroadcaster interface {
	Push(
		d dto.TerminalSessionDTO,
	)
}

// ProfileStore is the CRUD surface for terminal profiles.
type ProfileStore interface {
	FindAll(ctx context.Context) ([]domain.TerminalProfile, error)
	FindByKey(ctx context.Context, id string) (*domain.TerminalProfile, error)
	Save(ctx context.Context, item domain.TerminalProfile) error
	Delete(ctx context.Context, id string) error
}

// Handlers holds the dependencies shared across all terminal handler methods.
//
// There is deliberately no WorkspaceReader here any more. These routes mount
// under /v0/chats/:chatId, whose resolveChatWorktree middleware has ALREADY
// resolved the chat to its workspace and stashed it on the request context
// (reqscope.Workspace). A reader on this struct would only let a handler
// resolve the same thing a second time per request, which is precisely what
// doing the resolve as middleware exists to avoid.
type Handlers struct {
	termEng      TerminalEngine
	profileStore ProfileStore
	broadcast    TerminalBroadcaster
}

// New returns an initialised Handlers.
func New(
	termEng TerminalEngine,
	profileStore ProfileStore,
	broadcast TerminalBroadcaster,
) *Handlers {
	return &Handlers{
		termEng:      termEng,
		profileStore: profileStore,
		broadcast:    broadcast,
	}
}
