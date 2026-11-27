package seam

import (
	"context"
	"errors"

	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

var ErrHookNotRecorded = errors.New("agent: hook acknowledged, not recorded")

type ChatFeed struct {
	Telemetry     func(chatID, workspaceID string, report engineagents.Telemetry)
	TelemetryGone func(chatID string)
}

type TerminalCommander interface {
	CreateCommand(
		ctx context.Context,
		workspaceID string,
		cwd string,
		argv []string,
		env []string,
		onExit func(engineterminal.Exit),
	) (string, error)
	TerminateGraceful(
		ctx context.Context,
		sessionID string,
	) error
	Write(ctx context.Context, sessionID string, data []byte) error
	SessionLive(
		ctx context.Context,
		sessionID string,
	) bool
}

type WorkspaceReader interface {
	WorktreeDir(
		ctx context.Context,
		workspaceID string,
	) (crowbarHome, projectID, repoID, worktree string, err error)
	AgentChatsDir(
		ctx context.Context,
		workspaceID string,
	) (string, error)
}

type ChatLineage interface {
	Ancestors(
		ctx context.Context,
		chatID string,
	) ([]string, error)
}
