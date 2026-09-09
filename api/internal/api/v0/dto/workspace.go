package dto

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// WorkspaceDTO is the wire shape of a Workspace: the git-worktree aggregate that
// backs a sidebar row (00 §5.3). It carries the live-badge fields the frontend
// renders — diff counts, status, merge strategy, the merge-eligibility overlay,
// the last-error overlay, and the pull-request summary.
type WorkspaceDTO struct {
	ID              string                  `json:"id"`
	RepoID          string                  `json:"repoId"`
	ProjectID       string                  `json:"projectId"`
	Kind            domain.WorkspaceKind    `json:"kind,omitempty"`
	Branch          string                  `json:"branch"`
	ParentID        string                  `json:"parentId,omitempty"`
	ForkPointSha    string                  `json:"forkPointSha,omitempty"`
	Status          domain.WorkspaceStatus  `json:"status,omitempty"`
	Working         bool                    `json:"working"`
	LastError       string                  `json:"lastError,omitempty"`
	IsDefault       bool                    `json:"isDefault,omitempty"`
	Added           int                     `json:"added"`
	Deleted         int                     `json:"deleted"`
	MergeStrategy   gitdomain.MergeStrategy `json:"mergeStrategy"`
	CanMergeLocally bool                    `json:"canMergeLocally"`
	MergeConflicts  bool                    `json:"mergeConflicts"`
	ParentBranch    string                  `json:"parentBranch,omitempty"`
	PRUrl           string                  `json:"prUrl,omitempty"`
	PRTitle         string                  `json:"prTitle,omitempty"`
	PRTargetBranch  string                  `json:"prTargetBranch,omitempty"`
	// LocalPath is the on-disk worktree directory for this workspace. Clients use
	// it to construct absolute file paths (e.g. "Copy Path" in the file explorer).
	// Exposed here so workspace-creation details never bleed into client code.
	LocalPath string `json:"localPath,omitempty"`
	// HeldByPath is the worktree directory holding this branch when the workspace
	// is a placeholder (empty LocalPath — a protected placeholder is also locked,
	// an imported one is not). The client reconstructs the "checked out
	// elsewhere" reason from it; absent on healthy workspaces.
	HeldByPath string `json:"heldByPath,omitempty"`
	// CreatedAt is the tiebreak the list sorts undragged rows by: placement no
	// longer lives on this resource at all (it is the row's own chat-row
	// ParentID/Order in the unified sidebar tree — see
	// usecases/chat/internal/tree), so a client merging its cache falls back to
	// creation order rather than whatever the map happened to yield.
	CreatedAt time.Time `json:"createdAt"`
	// OwningChatID is the id of the chat row this workspace owns — the SAME row
	// its sidebar placement is addressed by (see CreatedAt above). It is
	// resolved with usecases/chat/internal/tree's own branch-preferring
	// backfill logic (never re-derived here), and is a plain pointer FROM the
	// workspace rather than a second copy of the chat row: a regular fork's
	// owning chat is ALSO an independently-rendered conversation, and merging
	// it into this list the way a locked row's owning chat is resolved would
	// collide the two under one id. Empty only for a workspace this backfill
	// has not reached, which should not happen for any row this route serves.
	OwningChatID string `json:"owningChatId"`
	// FolderID and Order are the workspace's own SIDEBAR placement (2026-09-09
	// sidebar-placement-unification, workspace-placement fix) — the
	// Node{Kind:workspace} row PlaceWorkspace writes, read back here so the
	// panel a drag just wrote to actually redraws it, rather than reverting
	// to whatever CreatedAt/id order it fell back to before this fix. A
	// SEPARATE field from ParentID (which stays the fork parent, above): a
	// folded row's own placement never came from git lineage, and conflating
	// the two would make an ordinary fork's rendered position jump every time
	// its FORK PARENT changed, independent of any drag.
	//
	// FolderID is "" for the repo root, exactly like ParentID's own zero
	// value — never ambiguous with "unset", because this field is never
	// omitted (see WorkspacePlacementReader). Order is likewise always
	// present, 0 for a row never placed.
	FolderID string `json:"folderId"`
	Order    int    `json:"order"`
}

// WorkspacePlacementReader resolves a workspace's own sidebar placement —
// FolderID/Order above — from its Node{Kind:workspace} row. A narrow,
// single-method port (rather than reusing chat/internal/tree's own Nodes
// port, internal to that package) so WorkspaceDTOFrom's callers can each
// wire it from whatever concrete Node store they already hold, without this
// package importing the tree usecase.
//
// A resolution failure (no Node row yet — a pre-existing workspace this
// fix's mint-on-first-touch has not reached because nothing has placed it
// since) degrades to the zero value, "" / 0, rather than failing the whole
// row: the SAME degradation PlaceWorkspace itself makes for an untouched
// subject, and the one a client already treats as "at the repo root, first
// slot" for a row it has never seen ordered before.
type WorkspacePlacementReader interface {
	Placement(ctx context.Context, workspaceID string) (folderID string, order int)
}

// WorkspaceDTOFrom converts a domain Workspace into its wire DTO, populating the
// merge-eligibility overlay (CanMergeLocally/ParentBranch) from the resolved
// eligibility the caller computed via MergeEligibilityFor over the repo-scoped
// sibling set, and the sidebar placement overlay (FolderID/Order) from
// placement, over the workspace's own id. Resolving both outside the converter
// keeps this function itself free of any store read (spec §10's same rule for
// eligibility, extended here to placement).
func WorkspaceDTOFrom(
	ctx context.Context,
	w domain.Workspace,
	elig workspace.MergeEligibility,
	owningChatID string,
	placement WorkspacePlacementReader,
) WorkspaceDTO {
	folderID, order := "", 0
	if placement != nil {
		folderID, order = placement.Placement(ctx, w.ID)
	}
	return WorkspaceDTO{
		ID:              w.ID,
		RepoID:          w.RepoID,
		ProjectID:       w.ProjectID,
		Kind:            w.Kind,
		Branch:          w.Branch,
		ParentID:        w.ParentID,
		ForkPointSha:    w.ForkPointSha,
		Status:          effectiveStatus(w.Status, elig.MergeConflicts),
		Working:         w.Working,
		LastError:       w.LastError,
		IsDefault:       w.IsDefault,
		Added:           w.Added,
		Deleted:         w.Deleted,
		MergeStrategy:   w.MergeStrategy,
		CanMergeLocally: elig.CanMergeLocally,
		MergeConflicts:  elig.MergeConflicts,
		ParentBranch:    elig.ParentBranch,
		PRUrl:           w.PRUrl,
		PRTitle:         w.PRTitle,
		PRTargetBranch:  w.PRTargetBranch,
		LocalPath:       w.WorktreePath,
		HeldByPath:      w.HeldByPath,
		CreatedAt:       w.CreatedAt,
		OwningChatID:    owningChatID,
		FolderID:        folderID,
		Order:           order,
	}
}

// effectiveStatus folds the predicted "conflicts with parent" signal into the
// wire status: a branch that would conflict with its parent — whether from a
// reparent's failed rebase or the merge-tree prediction — is surfaced as
// pr-conflicts, the single conflict state the UI resolves on. A terminal/locked
// status takes precedence. The persisted aggregate status is unchanged; this is
// a read-time overlay, like the merge-eligibility overlay above.
func effectiveStatus(base domain.WorkspaceStatus, mergeConflicts bool) domain.WorkspaceStatus {
	if mergeConflicts &&
		base != domain.WorkspaceStatusDeleted &&
		base != domain.WorkspaceStatusLocked {
		return domain.WorkspaceStatusPRConflicts
	}
	return base
}

// WorkspaceDTOList converts a slice of domain Workspaces into wire DTOs in
// sidebar order, resolving each row's merge eligibility through eligFn
// (typically a closure over MergeEligibilityFor bound to the same sibling
// slice), its real owning chat id through owningChatIDFn (typically a
// closure resolving Task 3's own branch-preferring backfill logic over that
// row's chats), and its own sidebar FolderID/Order through placement (see
// WorkspacePlacementReader) — nil is fine, degrading every row to "" / 0. It
// returns a non-nil empty slice when the input is empty so the envelope
// carries [].
//
// The sort lives HERE, in the converter both the REST list handler and the WS
// snapshot go through, because those are the two answers to the same question
// and a client that got different orders from them would watch its sidebar
// reshuffle on every reconnect.
func WorkspaceDTOList(
	ctx context.Context,
	workspaces []domain.Workspace,
	eligFn func(domain.Workspace) workspace.MergeEligibility,
	owningChatIDFn func(domain.Workspace) string,
	placement WorkspacePlacementReader,
) []WorkspaceDTO {
	dtos := make([]WorkspaceDTO, 0, len(workspaces))
	for _, w := range workspaces {
		dtos = append(dtos, WorkspaceDTOFrom(ctx, w, eligFn(w), owningChatIDFn(w), placement))
	}
	slices.SortFunc(dtos, compareWorkspaceDTOs)
	return dtos
}

// compareWorkspaceDTOs orders workspaces by creation time, then by id. Sidebar
// placement no longer lives on this resource (see WorkspaceDTO.CreatedAt), so
// creation order is the whole ordering this list can offer on its own; the
// row's real tree position comes from its own chat row.
func compareWorkspaceDTOs(
	a WorkspaceDTO,
	b WorkspaceDTO,
) int {
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Compare(b.CreatedAt)
	}
	return strings.Compare(a.ID, b.ID)
}
