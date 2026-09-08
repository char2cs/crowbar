package tree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// The home-scoped (RepoID == "") placement surface (2026-09-08
// sidebar-placement-unification Task 5): Folders is identity, Nodes is
// position — split apart the same way domain.Folder/domain.Node are.

// Folders is the plain-GORM identity surface a home-scoped folder's name now
// lives on: RepoID == "" only, forever — repo-scoped folders (RepoID != "")
// stay ChatTypeFolder Chat rows until Task 8. A folder's POSITION is never
// read or written here — see Nodes below.
type Folders interface {
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.Folder, error)
	// FindAll answers every home folder the daemon knows, filtered in memory
	// to RepoID == "" by the caller: "" is FindWhere's own zero value, which
	// per store.ScopedStore's doc can never be expressed as a query filter.
	FindAll(
		ctx context.Context,
	) ([]domain.Folder, error)
	Save(
		ctx context.Context,
		f domain.Folder,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
}

// Nodes is the position surface a home-scoped chat or folder's placement now
// goes through instead of Chat.SetOrder/.SetPlacement, mirroring project.go's
// own NodePlacements port for repos — the SAME Node rows a repo's own
// placement already writes, which is the whole point: every home-scope
// sibling (chat, folder, repo) now shares one Node.ListByParent read, no
// cross-aggregate merge required (see plan.go/home_forest.go's
// mergeHomeForest).
type Nodes interface {
	Create(
		ctx context.Context,
		id string,
		kind domain.NodeKind,
		parentID string,
		order int,
	) (domain.Node, error)
	GetNode(
		ctx context.Context,
		id string,
	) (domain.Node, error)
	ListByParent(
		ctx context.Context,
		parentID string,
	) ([]domain.Node, error)
	SetOrder(
		ctx context.Context,
		id string,
		order int,
	) error
	SetPlacement(
		ctx context.Context,
		id string,
		parentID string,
		order int,
	) error
	// Forget purges a Node row outright. Used ONLY to unwind a Create that a
	// just-failed home-folder create is taking back out — never to delete a
	// live, in-use row.
	Forget(
		ctx context.Context,
		id string,
	) error
}

// WorkspaceGitStatus is the narrow read port DeletePreview needs off the
// workspace usecase: each workspace's own already-synced Added/Deleted
// working-tree counts (00 §5.3) — the same numbers the sidebar itself
// renders, never a live git call. A preview runs before every idle delete
// confirm, so it has to stay as cheap as the read model it draws from.
//
// It is not a home-only port (DeletePreview needs it for every scope) — it
// lives in this file only because RepoIDsForHome (below) is, and moving the
// whole interface here keeps types.go under this package's own 500-line
// layering ceiling.
type WorkspaceGitStatus interface {
	WorkingTreeSummary(
		ctx context.Context,
		workspaceID string,
	) (added, deleted int, err error)
	// RepoOf answers the repo a workspace belongs to — "" for the project-home
	// workspace, a real repo id otherwise (domain.Workspace.RepoID, straight
	// off the same Get the adapter already makes for WorkingTreeSummary, no
	// new dependency). checkFolderContainer's golden rule uses it to resolve
	// the scope on the OTHER side of a folder-under-workspace-owning-row
	// containment check: a folder's own scope is its stored RepoID (or, for a
	// folder-under-folder check, the parent folder's own RepoID — no lookup
	// needed there at all), but a folder filed under a BRANCH or forked CHAT
	// row has to resolve that row's WorkspaceID back to a repo id to compare
	// against.
	RepoOf(
		ctx context.Context,
		workspaceID string,
	) (repoID string, err error)
	// RepoIDsForHome answers every repo id belonging to the SAME project as
	// home workspace homeWorkspaceID — this package's own counterpart to
	// project.go's repoIDSet. See mergeHomeForest's own doc (home_forest.go)
	// for the cross-project leak this closes and the folder-CRUD half it
	// deliberately does not.
	RepoIDsForHome(
		ctx context.Context,
		homeWorkspaceID string,
	) (map[string]bool, error)
}
