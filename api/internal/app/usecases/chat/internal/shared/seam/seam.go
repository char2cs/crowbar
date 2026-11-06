// Package seam declares the narrow, consumer-side ports the chat feature reaches the daemon
// through (the PTY, the workspace layout, chat ancestry), so changes it does not use cannot reach
// it. No tests: implementations assert satisfaction at compile time where they live.
package seam

import (
	"context"
	"errors"

	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// ErrHookNotRecorded is a hook the turns acknowledge and deliberately record
// nothing of: the ingress keeps it as a dead letter.
var ErrHookNotRecorded = errors.New("agent: hook acknowledged, not recorded")

// ChatFeed is where the chat usecase publishes live conversation facts no aggregate projection carries.
// It is bound once, at sweep start, by the layer that owns the hub; a nil field is skipped. One struct
// rather than a setter per fact, so a new live fact is one field here and one line where the hub is wired.
type ChatFeed struct {
	// Telemetry is the provider's newest usage report for the chat.
	Telemetry func(chatID, workspaceID string, report engineagents.Telemetry)
}

// TerminalCommander is the PTY seam every vendor CLI is started, ended and
// inspected through.
type TerminalCommander interface {
	CreateCommand(
		ctx context.Context,
		workspaceID string,
		cwd string,
		argv []string,
		env []string,
		onExit func(engineterminal.Exit),
	) (string, error)
	// TerminateGraceful quits a vendor CLI with SIGTERM, never SIGKILL, so it flushes its native
	// transcript. It is the only way a runner ends: the observed exit removes the runner, because the
	// PTY is the sole authority on liveness.
	TerminateGraceful(
		ctx context.Context,
		sessionID string,
	) error
	// Write sends raw keystrokes to a live session's PTY.
	Write(ctx context.Context, sessionID string, data []byte) error
	// SessionLive reports whether a session id is backed by a live PTY now; boot reconciliation exits
	// every runner for which it is false. Not SessionExists, which is also true for a suspended
	// placeholder whose process is dead and would keep an orphaned chat advertising a live agent.
	SessionLive(
		ctx context.Context,
		sessionID string,
	) bool
}

// WorkspaceReader resolves the on-disk locations one workspace's agent work
// happens in. It is the only seam this package has onto the workspace layer.
type WorkspaceReader interface {
	WorktreeDir(
		ctx context.Context,
		workspaceID string,
	) (crowbarHome, projectID, repoID, worktree string, err error)
	// AgentChatsDir returns the workspace's per-spawn tmp dirs (rendered hook config, never credentials).
	// It is always under crowbar home, even for an adopted checkout, so plaintext state never lands in
	// the user's directory. Not the chat's own dir, which is keyed by chat id since WorkspaceID is mutable.
	AgentChatsDir(
		ctx context.Context,
		workspaceID string,
	) (string, error)
}

// ChatLineage returns, at spawn time, the chat ancestors (nearest parent first) a thread inherits
// context from.
type ChatLineage interface {
	Ancestors(
		ctx context.Context,
		chatID string,
	) ([]string, error)
}
