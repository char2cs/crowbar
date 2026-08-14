// Package handlers contains the HTTP and WebSocket handler logic for terminal endpoints.
package handlers

import (
	"context"
	"image/color"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// WSConn is the WebSocket abstraction used by the terminal engine Attach method.
// It is an alias for engineterminal.WSConn so the concrete engine satisfies
// TerminalEngine without a wrapping adapter.
type WSConn = engineterminal.WSConn

// TerminalEngine is the subset of the terminal engine surface used by the handlers.
type TerminalEngine interface {
	Create(
		ctx context.Context,
		workspaceID string,
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
	ListSessionsForWorkspace(
		workspaceID string,
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

// WorkspaceReader can fetch a single workspace by ID.
type WorkspaceReader interface {
	Get(ctx context.Context, id string) (domain.Workspace, error)
}

// Handlers holds the dependencies shared across all terminal handler methods.
type Handlers struct {
	termEng      TerminalEngine
	profileStore ProfileStore
	wsReader     WorkspaceReader
	broadcast    TerminalBroadcaster
}

// New returns an initialised Handlers.
func New(
	termEng TerminalEngine,
	profileStore ProfileStore,
	wsReader WorkspaceReader,
	broadcast TerminalBroadcaster,
) *Handlers {
	return &Handlers{
		termEng:      termEng,
		profileStore: profileStore,
		wsReader:     wsReader,
		broadcast:    broadcast,
	}
}
