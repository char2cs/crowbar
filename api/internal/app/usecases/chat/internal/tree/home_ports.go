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
