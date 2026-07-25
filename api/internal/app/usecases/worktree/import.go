package worktree

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ImportInput carries a batch branch-import request: branches to import as
// managed workspaces, PR-parented up to a protected/default root.
type ImportInput struct {
	RepoID        string
	ProjectID     string
	RepoPath      string
	RemoteURL     string
	DefaultBranch string
	Branches      []string
}

// chainFor returns branch's ancestors-first chain of branches that must be
// CREATED: [rootmost-missing-ancestor, …, branch]. The walk climbs the PR-base
// graph and stops (EXCLUDING the terminal) at an existing workspace or the
// default branch; a branch with no PR base terminates at the default branch. A
// per-walk visited set breaks PR-base cycles.
func chainFor(branch, defaultBranch string, base, existing map[string]string) []string {
	var leafFirst []string
	visited := map[string]bool{}
	cur := branch
	for cur != "" && cur != defaultBranch {
		if _, ok := existing[cur]; ok {
			break // existing workspace is the parent terminal, not created here
		}
		if visited[cur] {
			break // cycle
		}
		visited[cur] = true
		leafFirst = append(leafFirst, cur)
		cur = base[cur] // "" when no open PR → loop ends, terminal is the default branch
	}
	// reverse to ancestors-first
	for i, j := 0, len(leafFirst)-1; i < j; i, j = i+1, j-1 {
		leafFirst[i], leafFirst[j] = leafFirst[j], leafFirst[i]
	}
	return leafFirst
}

// CreateFromImport imports each requested branch as a managed workspace,
// parenting it under the workspace for its open PR's base branch. Missing
// ancestors are created first and the whole chain is parented up to an existing
// workspace, a protected branch (already a locked workspace), or the repo
// default branch (parented under the repo home via an empty ParentID). It is
// best-effort per branch: one branch's failure is logged and does not abort the
// batch.
//
// This is the deterministic, import-time counterpart to the poll-driven
// provider.maybeReparentFromPR: it additionally CREATES missing parents and
// sets ParentID at creation. Because it sets ParentID explicitly, the poll's
// ParentID=="" guard leaves these rows untouched — no double-parenting.
func (u *worktreeUsecase) CreateFromImport(ctx context.Context, in ImportInput) error {
	// 1. Open-PR graph head→base (best-effort; empty on provider failure).
	base := map[string]string{}
	links, err := u.provider.OpenPullRequests(ctx, in.RepoPath)
	if err != nil {
		slog.WarnContext(ctx, "import: open-PR graph unavailable; importing without PR parenting", "err", err)
	}
	for _, l := range links {
		if l.Head != "" && l.Base != "" {
			base[l.Head] = l.Base
		}
	}

	// 2. Existing non-default workspaces: branch → id (matches hasWorkspace).
	existing := map[string]string{}
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return fmt.Errorf("import: list workspaces: %w", err)
	}
	for _, w := range all {
		if w.RepoID == in.RepoID && !w.IsDefault && w.Status != domain.WorkspaceStatusDeleted {
			existing[w.Branch] = w.ID
		}
	}

	// 3. Global creation order, parents-before-children, deduped.
	order := []string{}
	queued := map[string]bool{}
	for _, b := range in.Branches {
		for _, node := range chainFor(b, in.DefaultBranch, base, existing) {
			if !queued[node] {
				queued[node] = true
				order = append(order, node)
			}
		}
	}

	// 4. Create each node, resolving its parent from existing / just-created.
	created := map[string]string{}
	for _, branch := range order {
		parentBranch := base[branch]
		parentID := ""
		switch {
		case parentBranch == "" || parentBranch == in.DefaultBranch:
			parentBranch = in.DefaultBranch
		default:
			if id, ok := existing[parentBranch]; ok {
				parentID = id
			} else if id, ok := created[parentBranch]; ok {
				parentID = id
			} else {
				parentBranch = in.DefaultBranch // parent unresolved (cycle) → default
			}
		}
		ws, createErr := u.CreateChild(ctx, CreateChildInput{
			RepoID:       in.RepoID,
			ProjectID:    in.ProjectID,
			RepoPath:     in.RepoPath,
			RemoteURL:    in.RemoteURL,
			Branch:       branch,
			ParentID:     parentID,
			ParentBranch: parentBranch,
		})
		if createErr != nil {
			slog.WarnContext(ctx, "import: create workspace failed", "branch", branch, "err", createErr)
			continue
		}
		created[branch] = ws.ID
	}
	return nil
}
