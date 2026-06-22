package workspace

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// MergeEligibility is the computed, non-persisted answer to "can this workspace
// be merged into its local parent, and what is that parent's branch" (spec §10).
// It is resolved per read from the sibling set by MergeEligibilityFor; it is never
// stored on the workspace aggregate.
type MergeEligibility struct {
	CanMergeLocally bool
	ParentBranch    string
	// MergeConflicts is true when a mergeable child would conflict with its
	// parent (predicted via a non-destructive dry-run), so the UI blocks the
	// merge until the conflicts are resolved.
	MergeConflicts bool
}

// MergeConflictChecker predicts whether folding theirs into ours would conflict.
// Implemented by the git engine's WouldMergeConflict.
type MergeConflictChecker interface {
	WouldMergeConflict(ctx context.Context, repoPath, ours, theirs string) (bool, error)
}

// ResolveMergeEligibility computes the merge-eligibility overlay for ws from its
// sibling set: structural eligibility (parent present and not locked/deleted)
// plus a predicted-conflict check for an eligible child. It is the single source
// of this overlay, shared by the usecase read path and the repository broadcast
// path so the snapshot and the live stream always agree.
//
// The conflict dry-run is fast and best-effort: any error fails OPEN (false), so
// a check glitch never wrongly blocks a mergeable branch. git may be nil (no
// checker available) — then conflicts are simply not predicted.
func ResolveMergeEligibility(
	ctx context.Context,
	ws domain.Workspace,
	siblings []domain.Workspace,
	git MergeConflictChecker,
) MergeEligibility {
	if ws.ParentID == "" {
		return MergeEligibility{}
	}
	for _, s := range siblings {
		if s.ID != ws.ParentID {
			continue
		}
		// A parent must live in the same repo as its child. The usecase path
		// already passes repo-scoped siblings, but the broadcast path passes the
		// full row set, so guard here to never match a wrong-repo row.
		if s.ProjectID != ws.ProjectID || s.RepoID != ws.RepoID {
			continue
		}
		eligible := s.Status != domain.WorkspaceStatusLocked &&
			s.Status != domain.WorkspaceStatusDeleted
		conflicts := false
		if eligible && git != nil {
			if c, err := git.WouldMergeConflict(ctx, s.WorktreePath, s.Branch, ws.Branch); err == nil {
				conflicts = c
			}
		}
		return MergeEligibility{
			CanMergeLocally: eligible,
			ParentBranch:    s.Branch,
			MergeConflicts:  conflicts,
		}
	}
	return MergeEligibility{}
}
