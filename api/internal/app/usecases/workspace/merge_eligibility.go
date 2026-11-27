package workspace

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

type MergeEligibility struct {
	CanMergeLocally bool
	ParentBranch    string
	MergeConflicts  bool
}

type MergeConflictBatchChecker interface {
	WouldMergeConflicts(ctx context.Context, repoPath string, pairs []gitengine.MergePair) []gitengine.MergeVerdict
}

func ResolveMergeEligibilities(
	ctx context.Context,
	wss []domain.Workspace,
	siblings []domain.Workspace,
	git MergeConflictBatchChecker,
) []MergeEligibility {
	out := make([]MergeEligibility, len(wss))
	type batch struct {
		pairs []gitengine.MergePair
		at    []int
	}
	byPath := map[string]*batch{}
	var paths []string
	for i, ws := range wss {
		parent, ok := mergeParent(ws, siblings)
		if !ok {
			continue
		}
		out[i] = structuralEligibility(parent)
		if !out[i].CanMergeLocally || git == nil {
			continue
		}
		b := byPath[parent.WorktreePath]
		if b == nil {
			b = &batch{}
			byPath[parent.WorktreePath] = b
			paths = append(paths, parent.WorktreePath)
		}
		b.pairs = append(b.pairs, gitengine.MergePair{Ours: parent.Branch, Theirs: ws.Branch})
		b.at = append(b.at, i)
	}
	for _, path := range paths {
		b := byPath[path]
		for k, v := range git.WouldMergeConflicts(ctx, path, b.pairs) {
			if v.Err == nil {
				out[b.at[k]].MergeConflicts = v.Conflict
			}
		}
	}
	return out
}

func mergeParent(
	ws domain.Workspace,
	siblings []domain.Workspace,
) (domain.Workspace, bool) {
	if ws.ParentID == "" {
		return domain.Workspace{}, false
	}
	for _, s := range siblings {
		if s.ID == ws.ParentID && s.ProjectID == ws.ProjectID && s.RepoID == ws.RepoID {
			return s, true
		}
	}
	return domain.Workspace{}, false
}

func structuralEligibility(parent domain.Workspace) MergeEligibility {
	return MergeEligibility{
		CanMergeLocally: parent.Status != domain.WorkspaceStatusLocked &&
			parent.Status != domain.WorkspaceStatusDeleted,
		ParentBranch: parent.Branch,
	}
}
