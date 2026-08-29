// Package seam declares the vocabulary that crosses this feature's components:
// the ports it reaches the rest of the daemon through: the PTY it starts vendor CLIs on, the workspace layout it
// runs them in, and the chat ancestry a thread inherits.
//
// They are declared HERE, by the consumer, and narrowly: each one names only
// what this feature actually calls, so a change on the other side of the seam
// that this feature does not use cannot reach it.
//
// This package has no test file, and deliberately so: it declares interfaces and
// nothing else, so there is no behaviour to exercise and no statement to cover. A
// test asserting that the declarations exist would pass on any tree that
// compiles. What CAN be checked — that the production terminal engine and
// workspace reader satisfy them — is checked where those live, as compile-time
// assertions, because a satisfaction claim belongs next to the implementation
// making it.
package seam

import (
	"context"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// TerminalCommander is the PTY seam every vendor CLI is started, ended and
// inspected through.
type TerminalCommander interface {
	CreateCommand(
		ctx context.Context,
		workspaceID string,
		cwd string,
		argv []string,
		env []string,
		onExit func(),
	) (string, error)
	// TerminateGraceful quits a vendor CLI: a clean-exit SIGTERM, never SIGKILL.
	// It is the ONLY way this package ends a runner — we kill the process and let
	// its death carry the runner away (the engine's onExit → runner.Exit →
	// the live row disappears), because the PTY is the sole authority on liveness
	// and asserting an Exit we have not observed would make us a second one.
	//
	// Graceful matters twice over: a well-behaved CLI flushes its native transcript
	// on SIGTERM, so neither the provider being switched out nor the runner being
	// EVICTED from a conversation loses its last turn — and an evicted runner's
	// conversation is about to be read by the runner taking it over. Applies
	// uniformly to every provider (Codex tolerates it too); no provider branching.
	TerminateGraceful(
		ctx context.Context,
		sessionID string,
	) error
	// SessionLive reports whether a terminal session id is backed by a LIVE PTY right
	// now. Its one caller is ReconcileRunnersOnBoot (below): it is the single authority
	// boot reconciliation asks, Exiting every runner whose PTY did not survive the
	// restart. A false here means the CLI is definitively gone, even though no event
	// ever recorded it.
	//
	// It is deliberately NOT the engine's SessionExists, which is also true for a
	// PTY-less suspended placeholder — a session whose process is already dead and
	// whose only remaining substance is scrollback on disk. Asking the registry "do
	// you know this id?" instead of "is this process alive?" is what previously let a
	// restart-orphaned chat keep advertising a live agent.
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
	// AgentChatsDir returns the directory holding the workspace's own agent-work
	// state — per-spawn tmp dirs (the rendered hook config; nothing else — no
	// descriptor copies any credential into them, and none may) and the
	// per-runner hook-delivery journal. It is ALWAYS strictly under crowbar
	// home, even for a home-kind / adopted-checkout workspace whose worktree (Cwd)
	// is the user's REAL directory outside home: for a managed worktree it is the
	// sibling of the worktree, and for an adopted checkout it reroots under home
	// so plaintext state never lands on the user's filesystem. The worktree/Cwd is
	// unaffected — WorktreeDir still returns it unchanged.
	//
	// NOT the chat's own ledger (its prompt-delivery journal): that is keyed by
	// the chat's id alone (worktreepath.LedgerChatsDir), never by this workspace
	// lookup, because WorkspaceID is optional and mutable (spec §1.5).
	AgentChatsDir(
		ctx context.Context,
		workspaceID string,
	) (string, error)
}

// ChatLineage answers "what does this chat read" at spawn time: the chat
// ancestors, nearest parent first, that a thread inherits context from.
//
// It is an interface rather than the concrete reader so a test can answer the
// question without two tables behind it.
type ChatLineage interface {
	Ancestors(
		ctx context.Context,
		chatID string,
	) ([]string, error)
}

// Stall is a turn the SCREEN says will never finish — a usage limit, a service
// outage — carried from the detector that recognised it to the ingress that must
// close the turn.
//
// It lives here because it is the one type both sides name: the CLI lifecycle
// produces it, the hook ingress consumes it, and neither may import the other.
type Stall struct {
	ChatID      string
	WorkspaceID string
	ProviderID  string
	RunnerID    string
	SessionID   string

	// Notice is what the provider's descriptor recognised on the screen.
	Notice engineagents.TerminalNotice
}
