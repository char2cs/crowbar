package terminal

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	store "github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Engine is the PTY-session surface the terminal usecase passes through to.
type Engine interface {
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
}

// WorkspaceRepo is the workspace surface used to resolve a session's
// working directory.
type WorkspaceRepo interface {
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
}

// Usecase is the terminal session lifecycle + profile CRUD surface.
type Usecase interface {
	// CreateSession spawns a PTY session in the workspace's worktree directory.
	CreateSession(
		ctx context.Context,
		wsID string,
		prof *domain.TerminalProfile,
	) (string, error)

	// KillSession terminates a PTY session.
	KillSession(
		ctx context.Context,
		sessionID string,
	) error

	// ListProfiles returns every stored terminal profile.
	ListProfiles(
		ctx context.Context,
	) ([]domain.TerminalProfile, error)

	// CreateProfile mints an id and stores a new terminal profile.
	CreateProfile(
		ctx context.Context,
		prof domain.TerminalProfile,
	) (domain.TerminalProfile, error)

	// UpdateProfile overwrites an existing terminal profile.
	UpdateProfile(
		ctx context.Context,
		prof domain.TerminalProfile,
	) (domain.TerminalProfile, error)

	// DeleteProfile removes a terminal profile.
	DeleteProfile(
		ctx context.Context,
		id string,
	) error
}

type terminalUsecase struct {
	engine     Engine
	profiles   store.Store[domain.TerminalProfile, string]
	workspaces WorkspaceRepo
}

// New builds a Usecase from the terminal engine, the profile store, and the
// workspace repo.
func New(
	engine Engine,
	profiles store.Store[domain.TerminalProfile, string],
	workspaces WorkspaceRepo,
) Usecase {
	return &terminalUsecase{
		engine:     engine,
		profiles:   profiles,
		workspaces: workspaces,
	}
}

// CreateSession spawns a PTY session in the workspace's worktree directory.
func (u *terminalUsecase) CreateSession(
	ctx context.Context,
	wsID string,
	prof *domain.TerminalProfile,
) (string, error) {
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return "", fmt.Errorf("terminal: create session: load workspace: %w", err)
	}
	id, err := u.engine.Create(ctx, wsID, ws.WorktreePath, prof)
	if err != nil {
		return "", fmt.Errorf("terminal: create session: %w", err)
	}
	return id, nil
}

// KillSession terminates a PTY session.
func (u *terminalUsecase) KillSession(
	ctx context.Context,
	sessionID string,
) error {
	if err := u.engine.Kill(ctx, sessionID); err != nil {
		return fmt.Errorf("terminal: kill session: %w", err)
	}
	return nil
}

// ListProfiles returns every stored terminal profile.
func (u *terminalUsecase) ListProfiles(
	ctx context.Context,
) ([]domain.TerminalProfile, error) {
	list, err := u.profiles.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("terminal: list profiles: %w", err)
	}
	return list, nil
}

// CreateProfile mints an id and stores a new terminal profile.
func (u *terminalUsecase) CreateProfile(
	ctx context.Context,
	prof domain.TerminalProfile,
) (domain.TerminalProfile, error) {
	prof.ID = uuid.NewString()
	if err := u.profiles.Save(ctx, prof); err != nil {
		return domain.TerminalProfile{}, fmt.Errorf("terminal: create profile: %w", err)
	}
	return prof, nil
}

// UpdateProfile overwrites an existing terminal profile.
func (u *terminalUsecase) UpdateProfile(
	ctx context.Context,
	prof domain.TerminalProfile,
) (domain.TerminalProfile, error) {
	if prof.ID == "" {
		return domain.TerminalProfile{}, fmt.Errorf("terminal: update profile: missing id")
	}
	if err := u.profiles.Save(ctx, prof); err != nil {
		return domain.TerminalProfile{}, fmt.Errorf("terminal: update profile: %w", err)
	}
	return prof, nil
}

// DeleteProfile removes a terminal profile.
func (u *terminalUsecase) DeleteProfile(
	ctx context.Context,
	id string,
) error {
	if err := u.profiles.Delete(ctx, id); err != nil {
		return fmt.Errorf("terminal: delete profile: %w", err)
	}
	return nil
}
