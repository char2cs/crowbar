package hierarchy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

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
	Branches      []ImportBranch
}

// ImportBranch is one requested branch, the chat id the caller draws it under,
// and whether its workspace is created locked.
type ImportBranch struct {
	Name   string
	ChatID string
	Lock   bool
}

// ImportProgress hears each branch's outcome as the import settles it.
type ImportProgress interface {
	Branch(o ImportOutcome)
}

// ImportOutcome is one branch's result: the chat that now holds it, the
// workspace (a placeholder when Err is set), and why it failed.
type ImportOutcome struct {
	Branch      string
	ChatID      string
	WorkspaceID string
	Err         error
}

// chainFor returns branch's ancestors-first chain of branches to create. It climbs the PR-base graph,
// stopping (exclusive) at an existing workspace or the default branch; a visited set breaks cycles.
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

// CreateFromImport imports each requested branch, PR-parented with missing
// ancestors created first, and reports every branch to p once; a branch that
// cannot be materialised becomes a placeholder its chat owns, with the cause.
func (u *hierarchyUsecase) CreateFromImport(ctx context.Context, in ImportInput, p ImportProgress) error {
	// 1. Open-PR graph head→base (best-effort; empty on provider failure).
	base := u.prBaseGraph(ctx, in.RepoPath)

	// 2. Existing non-default workspaces: branch → id. Best-effort, like the PR
	// graph: it only decides parenting and where a chain walk stops, so a read
	// failure imports flat, which the PR-poll reparent then corrects.
	existing := u.existingBranchWorkspaces(ctx, in.RepoID)
	// Best-effort too: with no home found a branch imports unparented, as before.
	home, _, err := u.repoHomeWorkspaceID(ctx, in.RepoID)
	if err != nil {
		slog.WarnContext(ctx, "import: repo home unavailable; importing without a home parent", "err", err)
	}

	requested := map[string]ImportBranch{}
	names := make([]string, 0, len(in.Branches))
	for _, b := range in.Branches {
		requested[b.Name] = b
		names = append(names, b.Name)
		if wsID, ok := existing[b.Name]; ok {
			p.Branch(u.existingOutcome(ctx, b.Name, wsID))
		}
	}

	// 3. Global creation order, parents-before-children, deduped; 4. create each
	// node, resolving its parent from existing / just-created.
	created := map[string]string{}
	for _, branch := range creationOrder(names, in.DefaultBranch, base, existing) {
		parentID, parentBranch := resolveImportParent(branch, in.DefaultBranch, home, base, existing, created)
		ask, isRequested := requested[branch]
		outcome := u.createImportNode(ctx, in, importNode{
			branch: branch, chatID: ask.ChatID, parentID: parentID, parentBranch: parentBranch,
			locked: isRequested && ask.Lock,
		})
		if outcome.Err == nil && outcome.WorkspaceID != "" {
			created[branch] = outcome.WorkspaceID
		}
		p.Branch(outcome)
	}
	return ctx.Err()
}

// existingOutcome reports a branch that already has workspace wsID: its chat
// is the one that owns that workspace.
func (u *hierarchyUsecase) existingOutcome(
	ctx context.Context,
	branch string,
	wsID string,
) ImportOutcome {
	owner, err := u.owningChats.OwnerOf(ctx, wsID)
	if err == nil && owner == "" {
		err = fmt.Errorf("import: %s already has workspace %s, and no chat owns it", branch, wsID)
	}
	return ImportOutcome{Branch: branch, ChatID: owner, WorkspaceID: wsID, Err: err}
}

// prBaseGraph returns the open-PR head→base map used to parent imports. Best-effort: a provider
// failure yields an empty graph, parenting everything under the default branch rather than refusing.
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

// existingBranchWorkspaces maps branch → workspace id for the repo's managed workspaces, protected ones
// included (they are the parents imports nest under); the adopted default folder is excluded. A read
// failure yields an empty map, never an error.
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

// resolveImportParent picks the workspace of branch's PR base, else the default branch's workspace (so
// PRs against main nest under main, not beside it). Only when that is missing does it parent under the
// repo home workspace `home`, as a fork off the home does, so the branch can still merge back.
func resolveImportParent(
	branch string,
	defaultBranch string,
	home string,
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
	return home, defaultBranch
}

// importNode is one branch of the batch's creation order: chatID is the
// caller's for a requested branch, "" (daemon-minted) for an added ancestor.
type importNode struct {
	branch       string
	chatID       string
	parentID     string
	parentBranch string
	locked       bool
}

// createImportNode materialises one branch, falling back to a placeholder the
// attempt's chat owns; the outcome carries the cause (and the placeholder's).
func (u *hierarchyUsecase) createImportNode(
	ctx context.Context,
	in ImportInput,
	node importNode,
) ImportOutcome {
	outcome := ImportOutcome{Branch: node.branch, ChatID: node.chatID}
	if u.owningChats == nil {
		outcome.Err = ErrNoOwningChats
		return outcome
	}
	chatID, wsID, createErr := u.owningChats.ImportBranchAsChat(ctx, ImportedBranch{
		ChatID:            node.chatID,
		RepoID:            in.RepoID,
		ProjectID:         in.ProjectID,
		RepoPath:          in.RepoPath,
		RemoteURL:         in.RemoteURL,
		Branch:            node.branch,
		ParentWorkspaceID: node.parentID,
		ParentBranch:      node.parentBranch,
		ForceLocked:       node.locked,
	})
	if chatID != "" {
		outcome.ChatID = chatID
	}
	if createErr == nil {
		outcome.WorkspaceID = wsID
		return outcome
	}
	// A branch that got a workspace since the batch began keeps that one; the
	// chat this attempt minted owns nothing and goes.
	if errors.Is(createErr, ErrBranchWorkspaceExists) {
		return u.concurrentOutcome(ctx, in.RepoID, node.branch, chatID)
	}
	slog.WarnContext(ctx, "import: create workspace failed; falling back to a placeholder",
		"branch", node.branch, "err", createErr)
	placeholder, phErr := u.importPlaceholder(ctx, in, node, outcome.ChatID)
	outcome.WorkspaceID = placeholder.ID
	outcome.Err = errors.Join(createErr, phErr)
	return outcome
}

// concurrentOutcome reports branch as the workspace someone else created for
// it, discarding the chat this attempt minted.
func (u *hierarchyUsecase) concurrentOutcome(
	ctx context.Context,
	repoID string,
	branch string,
	mintedChatID string,
) ImportOutcome {
	if mintedChatID != "" {
		if err := u.owningChats.DiscardOwningChat(ctx, mintedChatID); err != nil {
			slog.WarnContext(ctx, "import: discard the chat of a branch imported concurrently",
				"chat_id", mintedChatID, "err", err)
		}
	}
	wsID, ok := u.existingBranchWorkspaces(ctx, repoID)[branch]
	if !ok {
		return ImportOutcome{Branch: branch, ChatID: mintedChatID, Err: ErrBranchWorkspaceExists}
	}
	return u.existingOutcome(ctx, branch, wsID)
}

// importPlaceholder records an unmaterialisable branch as a placeholder (empty WorktreePath), with the
// holder path when a live worktree blocked it; chatID owns it ("" mints one). It is not locked: locked
// survives provisioning, and the empty-WorktreePath guards already protect it.
func (u *hierarchyUsecase) importPlaceholder(
	ctx context.Context,
	in ImportInput,
	node importNode,
	chatID string,
) (domain.Workspace, error) {
	if chatID == "" {
		minted, err := u.owningChats.MintOwningChat(ctx, node.chatID, node.parentID)
		if err != nil {
			return domain.Workspace{}, fmt.Errorf("import: mint owning chat for %q: %w", node.branch, err)
		}
		chatID = minted
	}
	heldBy := u.resolveHolderPath(ctx, in.RepoPath, node.branch)
	ws, err := u.workspaces.Create(ctx, workspace.CreateInput{
		ID:           uuid.NewString(),
		RepoID:       in.RepoID,
		ProjectID:    in.ProjectID,
		Branch:       node.branch,
		ParentID:     node.parentID,
		HeldByPath:   heldBy,
		Provisioning: domain.WorkspacePlaceholder,
	}, u.now())
	if err != nil {
		return domain.Workspace{}, u.discardOwningChat(ctx, chatID,
			fmt.Errorf("import: create placeholder for %q: %w", node.branch, err))
	}
	if nErr := u.mintWorkspaceNode(ctx, ws.ID); nErr != nil {
		return domain.Workspace{}, u.discardUnownedWorkspace(ctx, chatID, ws,
			fmt.Errorf("import: mint placeholder node for %q: %w", node.branch, nErr))
	}
	if aErr := u.owningChats.AttachOwningWorkspace(ctx, chatID, ws); aErr != nil {
		return domain.Workspace{}, u.discardUnownedWorkspace(ctx, chatID, ws,
			fmt.Errorf("import: attach placeholder for %q: %w", node.branch, aErr))
	}
	return ws, nil
}

// resolveHolderPath returns the worktree dir holding branch when the user can free it (repo home or an
// external checkout); "" otherwise, meaning there is no Detach to offer.
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
