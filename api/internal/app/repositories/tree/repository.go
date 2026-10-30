// Package tree stores the Tree aggregate: one per project (aggregate id = the
// project id), holding where every sidebar row of that project hangs. Every
// write is one command against the whole project tree, sent with SendWait, and
// Get is the only read.
package tree

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// Repository is the Tree aggregate's store.
type Repository struct {
	ax asynx.Asynx[tree.Tree]
}

// New builds the repository over the Tree's asynx instance.
func New(ax asynx.Asynx[tree.Tree]) *Repository {
	return &Repository{ax: ax}
}

// Get is projectID's tree; a project nothing was placed in has an empty one.
func (r *Repository) Get(ctx context.Context, projectID string) (tree.Tree, error) {
	t, err := r.ax.Get(ctx, projectID)
	if errors.Is(err, asynxModels.ErrNotFound) {
		return tree.Empty(projectID), nil
	}
	if err != nil {
		return tree.Tree{}, fmt.Errorf("tree: get %s: %w", projectID, err)
	}
	return tree.Empty(projectID).Merge(t), nil
}

// Seed merges t into its project's tree: every record it lacks is added, every
// record it has is kept as written.
func (r *Repository) Seed(ctx context.Context, t tree.Tree) (tree.Tree, error) {
	return r.send(ctx, seed(t))
}

// Put creates or repositions a node.
func (r *Repository) Put(ctx context.Context, projectID string, n tree.Node) (tree.Tree, error) {
	return r.send(ctx, putNode(projectID, n))
}

// Delete removes a node; one the tree does not hold is tree.ErrUnknownNode.
func (r *Repository) Delete(ctx context.Context, projectID, id string) (tree.Tree, error) {
	return r.send(ctx, deleteNode(projectID, id))
}

// PutFolder writes a folder's identity.
func (r *Repository) PutFolder(ctx context.Context, projectID string, f tree.Folder) (tree.Tree, error) {
	return r.send(ctx, putFolder(projectID, f))
}

// DeleteFolder removes a folder's identity; an unknown one is tree.ErrUnknownNode.
func (r *Repository) DeleteFolder(ctx context.Context, projectID, id string) (tree.Tree, error) {
	return r.send(ctx, deleteFolder(projectID, id))
}

// PutChat writes a chat's own placement.
func (r *Repository) PutChat(
	ctx context.Context,
	projectID string,
	chatID string,
	p tree.Placement,
) (tree.Tree, error) {
	return r.send(ctx, putChat(projectID, chatID, p))
}

// DropChat removes a chat's own placement; none is no error.
func (r *Repository) DropChat(ctx context.Context, projectID, chatID string) (tree.Tree, error) {
	return r.send(ctx, dropChat(projectID, chatID))
}

// Take removes every record of id; none is no error.
func (r *Repository) Take(ctx context.Context, projectID, id string) (tree.Tree, error) {
	return r.send(ctx, take(projectID, id))
}

// Give writes records into projectID's tree over what it holds for their ids.
func (r *Repository) Give(ctx context.Context, projectID string, records tree.Tree) (tree.Tree, error) {
	return r.send(ctx, give(projectID, records))
}

// Forget erases projectID's tree; a project without one is no error.
func (r *Repository) Forget(ctx context.Context, projectID string) error {
	err := r.ax.Forget(ctx, projectID)
	if err != nil && !errors.Is(err, asynxModels.ErrValidation) {
		return fmt.Errorf("tree: forget %s: %w", projectID, err)
	}
	return nil
}

func (r *Repository) send(ctx context.Context, c edit) (tree.Tree, error) {
	evt, err := r.ax.SendWait(ctx, c)
	if err != nil {
		return tree.Tree{}, fmt.Errorf("tree: %s %s: %w", c.op, c.projectID, err)
	}
	return evt.Aggregate, nil
}
