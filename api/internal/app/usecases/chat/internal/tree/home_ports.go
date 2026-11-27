package tree

import (
	"context"
	"time"

	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

type Folders interface {
	FindByKey(
		ctx context.Context,
		id string,
	) (*dtree.Folder, error)
	FindAll(
		ctx context.Context,
	) ([]dtree.Folder, error)
	Save(
		ctx context.Context,
		f dtree.Folder,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
}

type Nodes interface {
	Create(
		ctx context.Context,
		id string,
		kind dtree.Kind,
		parentID string,
		order int,
	) (dtree.Node, error)
	GetNode(
		ctx context.Context,
		id string,
	) (dtree.Node, error)
	ListByParent(
		ctx context.Context,
		parentID string,
	) ([]dtree.Node, error)
	ListAll(
		ctx context.Context,
	) ([]dtree.Node, error)
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
	Forget(
		ctx context.Context,
		id string,
	) error
}

type WorkspaceGitStatus interface {
	RepoOf(
		ctx context.Context,
		workspaceID string,
	) (repoID string, err error)
	OutlivesItsChats(
		ctx context.Context,
		workspaceID string,
	) (bool, error)
	DefaultWorkspaceOf(
		ctx context.Context,
		repoID string,
	) (string, error)
	Exists(
		ctx context.Context,
		workspaceID string,
	) (bool, error)
	Provisioned(
		ctx context.Context,
		workspaceID string,
	) (bool, error)
	RepoIDsForHome(
		ctx context.Context,
		homeWorkspaceID string,
	) (map[string]bool, error)
	HomeOfRepo(
		ctx context.Context,
		repoID string,
	) (string, error)
	RendersAsBranch(
		ctx context.Context,
		workspaceID string,
	) (bool, error)
	CreatedAtOf(
		ctx context.Context,
		workspaceID string,
	) (time.Time, error)
	BranchRowsOf(
		ctx context.Context,
		repoID string,
	) ([]string, error)
	VisibleForkParent(
		ctx context.Context,
		workspaceID string,
	) (forkParentID string, err error)
}
