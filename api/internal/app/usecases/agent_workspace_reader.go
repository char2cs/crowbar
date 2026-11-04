package usecases

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// workspaceGetter is the minimal workspace-read surface agentWorkspaceReader
// needs: resolving the owning project/repo for a workspace id.
type workspaceGetter interface {
	Get(ctx context.Context, id string) (domain.Workspace, error)
}

// repoGetter is the minimal repository-read surface agentWorkspaceReader needs to
// resolve a home-kind (adopted-checkout) workspace's on-disk identity slug from
// its repo id, mirroring workspace's worktree hierarchy resolveSlug's load-the-row pattern so the
// no-remote fallback can still reach the repo NAME.
type repoGetter interface {
	FindByKey(ctx context.Context, id string) (*domain.Repository, error)
}

// agentWorkspaceReader adapts the workspace repository into the agent
// usecase's WorkspaceReader seam (internal/app/usecases/chat.WorkspaceReader):
// given a workspace id, it resolves the owning project/repo and the git
// worktree directory from the workspace read model's stored WorktreePath
// (WorktreeDir), and the directory holding the workspace's agentic chat state
// (AgentChatsDir), which for an adopted checkout reroots under crowbar home.
//
// It shares the container's injected crowbarHome resolver (the same one every
// other path-deriving usecase here is built with) rather than resolving a
// home of its own: path-deriving usecases MUST resolve against
// the home the adapter container was actually opened against (see
// adapter.Container.CrowbarHome's doc comment), and a hermetic test that
// overrides the home via adapter.WithHomeDir (rather than the CROWBAR_HOME env
// var) would otherwise silently diverge.
type agentWorkspaceReader struct {
	workspaces  workspaceGetter
	repos       repoGetter
	crowbarHome func() (string, error)
}

// WorktreeDir implements agentusecase.WorkspaceReader.
func (r *agentWorkspaceReader) WorktreeDir(
	ctx context.Context,
	workspaceID string,
) (crowbarHome, projectID, repoID, worktree string, err error) {
	home, err := r.crowbarHome()
	if err != nil {
		return "", "", "", "", fmt.Errorf("usecases: agent workspace reader: crowbar home: %w", err)
	}
	w, err := r.workspaces.Get(ctx, workspaceID)
	if err != nil {
		return "", "", "", "", fmt.Errorf("usecases: agent workspace reader: get workspace: %w", err)
	}
	return home, w.ProjectID, w.RepoID, w.WorktreePath, nil
}

// AgentChatsDir implements agentusecase.WorkspaceReader: the directory, always under
// crowbar home, of a workspace's own agent-work state. A chat's own directory is
// worktreepath.LedgerChatsDir, keyed by the chat id alone: its workspace may change.
//
// For a Crowbar-managed worktree (WorktreePath strictly under home) the chats dir
// is the sibling of the worktree (worktreepath.ChatsDir), reaped with the
// workspace root on delete. For an ADOPTED CHECKOUT — the repo-home / project-home
// whose WorktreePath is the user's REAL directory OUTSIDE home — the chats dir
// reroots under home at <home>/projects/<projectId>/<slug>/default/chats
// (worktreepath.HomeDefaultChatsDir), so plaintext state is never written onto
// the user's filesystem beside their repository. The Cwd is unaffected:
// WorktreeDir still returns the adopted worktree unchanged.
//
// The discriminator is the under-home test, NOT the workspace Kind: the
// chat-hosting repo-home is a Kind=git / IsDefault workspace (adoptRepoHome does
// not set WorkspaceKindHome) whose WorktreePath is repo.Path outside home, so
// keying on Kind would both MISS it (leaving the real leak) and break the
// project-level home (WorkspaceKindHome, but with no repo id to resolve a slug
// from). Keying on the path that must never be written to is what makes the output
// provably safe for every kind.
func (r *agentWorkspaceReader) AgentChatsDir(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	home, err := r.crowbarHome()
	if err != nil {
		return "", fmt.Errorf("usecases: agent workspace reader: crowbar home: %w", err)
	}
	w, err := r.workspaces.Get(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("usecases: agent workspace reader: get workspace: %w", err)
	}
	if worktreepath.UnderHome(w.WorktreePath, home) {
		return worktreepath.ManagedChatsDir(home, w.ProjectID, w.ID, w.WorktreePath), nil
	}
	slug, err := r.repoSlug(ctx, w.RepoID)
	if err != nil {
		return "", err
	}
	chatsDir := worktreepath.HomeDefaultChatsDir(home, w.ProjectID, slug)
	// Fail closed if the rerooted path escapes home. filepath.Join (inside
	// HomeDefaultChatsDir) cleans "..", so a crafted repo remote whose RemoteSlug
	// contains "../" segments could otherwise resolve OUTSIDE crowbar home — which
	// would let a plaintext ledger be WRITTEN onto the user's real filesystem. Real
	// remotes never do this; a poisoned one gets an error, not an escape (the
	// removal sites re-assert the same invariant as a second backstop).
	if !worktreepath.UnderHome(chatsDir, home) {
		return "", fmt.Errorf(
			"usecases: agent workspace reader: resolved chats dir %q escapes crowbar home %q (poisoned slug %q)",
			chatsDir, home, slug,
		)
	}
	return chatsDir, nil
}

// repoSlug resolves the repo's on-disk identity slug for an adopted-checkout
// workspace's rerooted chats dir. A blank repo id (the project-level home has no
// repo) yields an empty slug, which HomeDefaultChatsDir collapses to the project
// directory — still strictly under home. It always loads the repo row so the
// no-remote / unparseable-URL fallback can reach the repo NAME, mirroring
// workspace's worktree hierarchy resolveSlug.
func (r *agentWorkspaceReader) repoSlug(
	ctx context.Context,
	repoID string,
) (string, error) {
	if repoID == "" {
		return "", nil
	}
	repo, err := r.repos.FindByKey(ctx, repoID)
	if err != nil {
		return "", fmt.Errorf("usecases: agent workspace reader: get repo: %w", err)
	}
	if repo == nil {
		return "", fmt.Errorf("usecases: agent workspace reader: repo %q not found", repoID)
	}
	return worktreepath.RemoteSlug(*repo), nil
}
