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

type ImportInput struct {
	RepoID        string
	ProjectID     string
	RepoPath      string
	RemoteURL     string
	DefaultBranch string
	Branches      []ImportBranch
}

type ImportBranch struct {
	Name   string
	ChatID string
	Lock   bool
}

type ImportProgress interface {
	Branch(o ImportOutcome)
}

type ImportOutcome struct {
	Branch      string
	ChatID      string
	WorkspaceID string
	Err         error
}

func chainFor(branch, defaultBranch string, base, existing map[string]string) []string {
	var leafFirst []string
	visited := map[string]bool{}
	cur := branch
	for cur != "" && cur != defaultBranch {
		if _, ok := existing[cur]; ok {
			break
		}
		if visited[cur] {
			break
		}
		visited[cur] = true
		leafFirst = append(leafFirst, cur)
		cur = base[cur]
	}
	for i, j := 0, len(leafFirst)-1; i < j; i, j = i+1, j-1 {
		leafFirst[i], leafFirst[j] = leafFirst[j], leafFirst[i]
	}
	return leafFirst
}

func (u *hierarchyUsecase) CreateFromImport(ctx context.Context, in ImportInput, p ImportProgress) error {
	base := u.prBaseGraph(ctx, in.RepoPath)

	existing := u.existingBranchWorkspaces(ctx, in.RepoID)
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
	if parentBranch != defaultBranch {
		if id, ok := existing[defaultBranch]; ok {
			return id, defaultBranch
		}
	}
	return home, defaultBranch
}

type importNode struct {
	branch       string
	chatID       string
	parentID     string
	parentBranch string
	locked       bool
}

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
