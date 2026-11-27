package tree

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/core/readonce"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

type Repository struct {
	ax asynx.Asynx[tree.Tree]
}

func New(ax asynx.Asynx[tree.Tree]) *Repository {
	return &Repository{ax: ax}
}

func (r *Repository) Get(ctx context.Context, projectID string) (tree.Tree, error) {
	return readonce.Do(ctx, "tree:"+projectID, func() (tree.Tree, error) { return r.get(ctx, projectID) })
}

func (r *Repository) get(ctx context.Context, projectID string) (tree.Tree, error) {
	t, err := r.ax.Get(ctx, projectID)
	if errors.Is(err, asynxModels.ErrNotFound) {
		return tree.Empty(projectID), nil
	}
	if err != nil {
		return tree.Tree{}, fmt.Errorf("tree: get %s: %w", projectID, err)
	}
	return tree.Empty(projectID).Merge(t), nil
}

func (r *Repository) Seed(ctx context.Context, t tree.Tree) (tree.Tree, error) {
	return r.send(ctx, seed(t))
}

func (r *Repository) Put(ctx context.Context, projectID string, n tree.Node) (tree.Tree, error) {
	return r.send(ctx, putNode(projectID, n))
}

func (r *Repository) Create(ctx context.Context, projectID string, n tree.Node) (tree.Tree, error) {
	return r.send(ctx, createNode(projectID, n))
}

func (r *Repository) Move(ctx context.Context, projectID, id, parentID string, order int) (tree.Tree, error) {
	return r.send(ctx, moveNode(projectID, id, parentID, order, false))
}

func (r *Repository) Order(ctx context.Context, projectID, id string, order int) (tree.Tree, error) {
	return r.send(ctx, moveNode(projectID, id, "", order, true))
}

func (r *Repository) Delete(ctx context.Context, projectID, id string) (tree.Tree, error) {
	return r.send(ctx, deleteNode(projectID, id))
}

func (r *Repository) PutFolder(ctx context.Context, projectID string, f tree.Folder) (tree.Tree, error) {
	return r.send(ctx, putFolder(projectID, f))
}

func (r *Repository) DeleteFolder(ctx context.Context, projectID, id string) (tree.Tree, error) {
	return r.send(ctx, deleteFolder(projectID, id))
}

func (r *Repository) PutChat(
	ctx context.Context,
	projectID string,
	chatID string,
	p tree.Placement,
) (tree.Tree, error) {
	return r.send(ctx, putChat(projectID, chatID, p))
}

func (r *Repository) OrderChat(ctx context.Context, projectID, chatID string, order int) (tree.Tree, error) {
	return r.send(ctx, orderChat(projectID, chatID, order))
}

func (r *Repository) DropChat(ctx context.Context, projectID, chatID string) (tree.Tree, error) {
	return r.send(ctx, dropChat(projectID, chatID))
}

func (r *Repository) Take(ctx context.Context, projectID, id string) (tree.Tree, error) {
	return r.send(ctx, take(projectID, id))
}

func (r *Repository) Give(ctx context.Context, projectID string, records tree.Tree) (tree.Tree, error) {
	return r.send(ctx, give(projectID, records))
}

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

const schemaVersion = 1

func Schema(b *asynx.Builder[tree.Tree]) {
	b.WithSchemaVersion(schemaVersion)
}

func (r *Repository) WaitPublish() {
	r.ax.WaitPublish()
}
