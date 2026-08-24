package chat

import "context"

// WorkspaceReader resolves the on-disk locations one workspace's agent work
// happens in. It is the only seam this package has onto the workspace layer.
type WorkspaceReader interface {
	WorktreeDir(
		ctx context.Context,
		workspaceID string,
	) (crowbarHome, projectID, repoID, worktree string, err error)
	// AgentChatsDir returns the directory holding the workspace's agentic chat
	// state — the per-chat handoff ledger and the per-spawn tmp dirs (the rendered
	// hook config; nothing else — no descriptor copies any credential into them,
	// and none may). It is ALWAYS strictly under crowbar
	// home, even for a home-kind / adopted-checkout workspace whose worktree (Cwd)
	// is the user's REAL directory outside home: for a managed worktree it is the
	// sibling of the worktree, and for an adopted checkout it reroots under home
	// so plaintext ledgers never land on the user's filesystem. The worktree/Cwd is
	// unaffected — WorktreeDir still returns it unchanged.
	AgentChatsDir(
		ctx context.Context,
		workspaceID string,
	) (string, error)
}
