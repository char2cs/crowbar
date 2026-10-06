package workspace

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

// MergeEligibility is the computed, non-persisted answer to "can this workspace
// be merged into its local parent, and what is that parent's branch" (spec §10).
// It is resolved per read from the sibling set by MergeEligibilitiesFor; it is never
// stored on the workspace aggregate.
type MergeEligibility struct {
	CanMergeLocally bool
	ParentBranch    string
	// MergeConflicts is true when a mergeable child would conflict with its
	// parent (predicted via a non-destructive dry-run), so the UI blocks the
	// merge until the conflicts are resolved.
	MergeConflicts bool
}

// MergeConflictBatchChecker predicts whether folding each pair's Theirs into its
// Ours would conflict, for many pairs at once, so a list asks git once per parent
// checkout instead of once per workspace. Implemented by the git engine.
type MergeConflictBatchChecker interface {
	WouldMergeConflicts(ctx context.Context, repoPath string, pairs []gitengine.MergePair) []gitengine.MergeVerdict
}

// ResolveMergeEligibilities computes the merge-eligibility overlay for every
// workspace in wss, in order, from the sibling set: structural eligibility (parent
// present and not locked/deleted) plus a predicted-conflict check for an eligible
// child, with the dry-runs batched per parent checkout. It is the single source
// of this overlay, shared by the usecase read path and the repository broadcast
// path so the snapshot and the live stream always agree.
//
// The dry-run is fast and best-effort: a failed one fails OPEN (no conflict), so
// a check glitch never wrongly blocks a mergeable branch. git may be nil (no
// checker available) — then conflicts are simply not predicted.
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

// mergeParent is ws's parent among siblings. A parent must live in the same
// repo as its child: the usecase path already passes repo-scoped siblings, but
// the broadcast path passes the full row set, so a wrong-repo row never matches.
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

// structuralEligibility is what the parent alone decides: it must be neither
// locked nor deleted.
func structuralEligibility(parent domain.Workspace) MergeEligibility {
	return MergeEligibility{
		CanMergeLocally: parent.Status != domain.WorkspaceStatusLocked &&
			parent.Status != domain.WorkspaceStatusDeleted,
		ParentBranch: parent.Branch,
	}
}
