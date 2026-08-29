package hierarchy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/holder"
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
// best-effort per branch: one branch's failure does not abort the batch.
//
// Every requested branch yields a ROW — that is the contract. A branch that
// cannot be materialised (most often because a live worktree outside Crowbar
// already has it checked out, which git refuses to duplicate) falls back to a
// PLACEHOLDER row, the same shape the protected-branch import produces (spec
// §3.3): an empty WorktreePath carrying HeldByPath, which the client renders
// with a reason plus Retry / Detach… and toasts once. Dropping the branch
// instead is what stranded the client's optimistic import row forever with
// nothing surfaced: that row is cleared only when a workspace for the branch
// arrives on the stream, and a 202'd batch has no other completion signal.
//
// This is the deterministic, import-time counterpart to the poll-driven
// provider.maybeReparentFromPR: it additionally CREATES missing parents and
// sets ParentID at creation. Because it sets ParentID explicitly, the poll's
// ParentID=="" guard leaves these rows untouched — no double-parenting.
func (u *hierarchyUsecase) CreateFromImport(ctx context.Context, in ImportInput) error {
	// 1. Open-PR graph head→base (best-effort; empty on provider failure).
	base := u.prBaseGraph(ctx, in.RepoPath)

	// 2. Existing non-default workspaces: branch → id (matches hasWorkspace).
	// Best-effort, like the PR graph: this map only decides PARENTING and where
	// a chain walk terminates, and aborting here produced the one outcome the
	// client cannot recover from — a 202'd batch with no row for any branch, so
	// every optimistic import row spins forever with nothing surfaced (the async
	// error rides runAsync's blank workspace id, which broadcastLastError drops).
	// Degrading to an empty map imports everything flat instead, which the
	// PR-poll reparent then corrects.
	existing := u.existingBranchWorkspaces(ctx, in.RepoID)

	// 3. Global creation order, parents-before-children, deduped.
	order := creationOrder(in.Branches, in.DefaultBranch, base, existing)

	// 4. Create each node, resolving its parent from existing / just-created.
	created := map[string]string{}
	var stranded []string
	for _, branch := range order {
		parentID, parentBranch := resolveImportParent(branch, in.DefaultBranch, base, existing, created)
		id, err := u.createImportNode(ctx, in, branch, parentID, parentBranch)
		if err != nil {
			stranded = append(stranded, branch)
			continue
		}
		if id != "" {
			created[branch] = id
		}
	}
	if len(stranded) > 0 {
		return fmt.Errorf("import: no workspace row produced for %s", strings.Join(stranded, ", "))
	}
	return nil
}

// prBaseGraph returns the open-PR head→base map used to parent imported
// branches. It is best-effort: a provider failure yields an empty graph and the
// import falls back to parenting everything under the default branch, which is
// strictly better than refusing to import because GitHub was unreachable.
func (u *hierarchyUsecase) prBaseGraph(ctx context.Context, repoPath string) map[string]string {
	base := map[string]string{}
	links, err := u.provider.OpenPullRequests(ctx, repoPath)
	if err != nil {
		slog.WarnContext(ctx, "import: open-PR graph unavailable; importing without PR parenting", "err", err)
	}
	for _, l := range links {
		if l.Head != "" && l.Base != "" {
			base[l.Head] = l.Base
		}
	}
	return base
}

// existingBranchWorkspaces maps branch → workspace id for the repo's MANAGED
// workspaces. The default workspace is excluded because it is the adopted repo
// folder, not a branch Crowbar owns — the same rule the import branch list uses
// for hasWorkspace. Every protected branch — the default branch included — DOES
// appear here: repo import gives each one its own locked managed worktree, and
// those are exactly the parents resolveImportParent nests imports under.
//
// A read failure yields an empty map, never an error: see CreateFromImport.
func (u *hierarchyUsecase) existingBranchWorkspaces(
	ctx context.Context,
	repoID string,
) map[string]string {
	existing := map[string]string{}
	all, err := u.workspaces.List(ctx)
	if err != nil {
		slog.WarnContext(ctx, "import: workspace list unavailable; importing without existing-parent resolution",
			"err", err)
		return existing
	}
	for _, w := range all {
		if w.RepoID == repoID && !w.IsDefault && w.Status != domain.WorkspaceStatusDeleted {
			existing[w.Branch] = w.ID
		}
	}
	return existing
}

// creationOrder flattens every requested branch's chain into one deduped,
// parents-before-children creation order.
func creationOrder(
	branches []string,
	defaultBranch string,
	base, existing map[string]string,
) []string {
	order := []string{}
	queued := map[string]bool{}
	for _, b := range branches {
		for _, node := range chainFor(b, defaultBranch, base, existing) {
			if !queued[node] {
				queued[node] = true
				order = append(order, node)
			}
		}
	}
	return order
}

// resolveImportParent picks the parent for branch: the workspace for its open
// PR's base when one can be resolved, and the repo's DEFAULT BRANCH workspace
// otherwise. Only when even that workspace is missing does it fall back to an
// empty ParentID, which parents the branch under the repo home.
//
// The default branch used to be short-circuited to that empty ParentID before
// the lookup ever ran — so a PR targeting develop/main, which is the shape of
// almost every PR, landed its branch at the REPO ROOT as a sibling of the very
// branch it is based on. The lookup would have succeeded: repo import gives
// every protected branch (and, when the provider reports none, the resolved
// default branch) its own locked managed workspace, and existingBranchWorkspaces
// excludes only the repo home. A branch with NO PR resolves the same way — its
// base is the default branch by definition, so it nests there too.
func resolveImportParent(
	branch string,
	defaultBranch string,
	base, existing, created map[string]string,
) (parentID, parentBranch string) {
	parentBranch = base[branch]
	if parentBranch == "" {
		parentBranch = defaultBranch
	}
	if id, ok := existing[parentBranch]; ok {
		return id, parentBranch
	}
	if id, ok := created[parentBranch]; ok {
		return id, parentBranch
	}
	// An unresolvable PR base (a cycle, or a base branch nobody imported) still
	// belongs under the default branch's workspace when there is one.
	if parentBranch != defaultBranch {
		if id, ok := existing[defaultBranch]; ok {
			return id, defaultBranch
		}
	}
	return "", defaultBranch
}

// createImportNode materialises one branch of the batch and returns its
// workspace id. A branch that cannot be materialised falls back to a placeholder
// row so it still reaches the client (see CreateFromImport); the returned error
// is reserved for the one outcome that produces NO row at all.
//
// An empty id with a nil error means "already represented, nothing to record".
func (u *hierarchyUsecase) createImportNode(
	ctx context.Context,
	in ImportInput,
	branch string,
	parentID string,
	parentBranch string,
) (string, error) {
	ws, createErr := u.CreateChild(ctx, CreateChildInput{
		RepoID:       in.RepoID,
		ProjectID:    in.ProjectID,
		RepoPath:     in.RepoPath,
		RemoteURL:    in.RemoteURL,
		Branch:       branch,
		ParentID:     parentID,
		ParentBranch: parentBranch,
	})
	if createErr == nil {
		return ws.ID, nil
	}
	slog.WarnContext(ctx, "import: create workspace failed; falling back to a placeholder",
		"branch", branch, "err", createErr)
	// A branch that already has a workspace is not stranded and must not get a
	// second row: the client's optimistic row matches on branch alone, so the row
	// that already exists is what clears it.
	if errors.Is(createErr, ErrBranchWorkspaceExists) {
		return "", nil
	}
	placeholder, phErr := u.importPlaceholder(ctx, in, branch, parentID, createErr)
	if phErr != nil {
		// Nothing landed for this branch — the only genuinely invisible outcome
		// left, and the one the aggregated error reports.
		slog.ErrorContext(ctx, "import: placeholder fallback failed; branch produced no row",
			"branch", branch, "err", phErr)
		return "", phErr
	}
	return placeholder.ID, nil
}

// importPlaceholder records a branch that could not be materialised as a
// placeholder workspace: a row with an EMPTY WorktreePath (the placeholder
// signal) carrying the holder path when a live worktree is what blocked it.
//
// It is deliberately NOT protected. The protected-branch import seeds its
// placeholders locked to inherit every protection guard, but locked survives
// provisioning (ProvisionInPlace leaves Status untouched), so a locked imported
// feature branch could never be merged, renamed or deleted again. The guards
// that matter here key off the empty WorktreePath, not the status: the
// parent-tip ops refuse with ErrParentUnprovisioned and removeOne drops the row
// without running git against "".
//
// A failure with no live holder has no reconstructable reason, so the cause is
// persisted as LastError for the row to explain itself.
func (u *hierarchyUsecase) importPlaceholder(
	ctx context.Context,
	in ImportInput,
	branch string,
	parentID string,
	cause error,
) (domain.Workspace, error) {
	heldBy := u.resolveHolderPath(ctx, in.RepoPath, branch)
	ws, err := u.workspaces.Create(ctx, workspace.CreateInput{
		ID:         uuid.NewString(),
		RepoID:     in.RepoID,
		ProjectID:  in.ProjectID,
		Branch:     branch,
		ParentID:   parentID,
		HeldByPath: heldBy,
		// WorktreePath + ForkPointSha stay empty — this is the placeholder signal.
	}, u.now())
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("import: create placeholder for %q: %w", branch, err)
	}
	if heldBy == "" && cause != nil {
		if _, sErr := u.workspaces.SetLastError(ctx, ws.ID, cause.Error()); sErr != nil {
			slog.WarnContext(ctx, "import: could not record placeholder cause",
				"branch", branch, "err", sErr)
		}
	}
	return ws, nil
}

// resolveHolderPath returns the worktree directory holding branch when that
// holder is one the user can actually free — the repo home or an external
// checkout, the same two kinds RetryProvision refuses to provision over. It
// returns "" for every other outcome (free, already-managed, or an unresolvable
// holder), which is the signal that there is no Detach… to offer.
func (u *hierarchyUsecase) resolveHolderPath(
	ctx context.Context,
	repoPath string,
	branch string,
) string {
	home, err := u.crowbarHome()
	if err != nil {
		return ""
	}
	outcome, err := holder.Resolve(ctx, u.git, repoPath, branch, home)
	if err != nil {
		return ""
	}
	if outcome.Kind == holder.HeldByHome || outcome.Kind == holder.HeldByExternal {
		return outcome.HeldByPath
	}
	return ""
}
