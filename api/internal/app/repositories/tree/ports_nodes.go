package tree

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// The position rows, by id, as the placement usecases read and write them.

// Create mints id's position row; one that already exists is refused.
func (p *Ports) Create(
	ctx context.Context,
	id string,
	kind tree.Kind,
	parentID string,
	order int,
) (tree.Node, error) {
	at, err := p.placeIn(ctx, id, kind, "", parentID)
	if err != nil {
		return tree.Node{}, err
	}
	n := tree.Node{ID: id, Kind: kind, ParentID: parentID, Order: order}
	if _, err := p.repo.Create(ctx, at, n); err != nil {
		return tree.Node{}, err
	}
	p.record(id, at)
	return n, nil
}

// GetNode is id's position row, or tree.ErrUnknownNode.
func (p *Ports) GetNode(ctx context.Context, id string) (tree.Node, error) {
	t, ok, err := p.treeOf(ctx, id)
	if err != nil {
		return tree.Node{}, err
	}
	n, exists := t.Nodes[id]
	if !ok || !exists {
		return tree.Node{}, fmt.Errorf("tree: node %s: %w", id, tree.ErrUnknownNode)
	}
	return n, nil
}

// ListByParent is every position row under parentID, in every tree: a ""
// parent is each scope's own root, shared by every project.
func (p *Ports) ListByParent(ctx context.Context, parentID string) ([]tree.Node, error) {
	all, err := p.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, n := range all {
		if n.ParentID == parentID {
			out = append(out, n)
		}
	}
	return out, nil
}

// ListAll is every position row in one read.
func (p *Ports) ListAll(ctx context.Context) ([]tree.Node, error) {
	trees, err := p.all(ctx)
	if err != nil {
		return nil, err
	}
	var out []tree.Node
	for _, t := range trees {
		for _, n := range t.Nodes {
			out = append(out, n)
		}
	}
	return sortNodes(out), nil
}

// SetOrder writes id's index and keeps its parent as the tree holds it when
// the write lands.
func (p *Ports) SetOrder(ctx context.Context, id string, order int) error {
	at, held := p.holder(id)
	if !held {
		return noRow("set order", id)
	}
	if _, err := p.repo.Order(ctx, at, id, order); err != nil {
		return fmt.Errorf("tree: set order %s: %w", id, err)
	}
	return nil
}

// SetPlacement writes id's parent and index.
func (p *Ports) SetPlacement(ctx context.Context, id, parentID string, order int) error {
	at, held := p.holder(id)
	if !held {
		return noRow("set placement", id)
	}
	if _, err := p.repo.Move(ctx, at, id, parentID, order); err != nil {
		return fmt.Errorf("tree: set placement %s: %w", id, err)
	}
	return nil
}

// Forget removes id's position row; a row that is not there is no error.
func (p *Ports) Forget(ctx context.Context, id string) error {
	t, ok, err := p.treeOf(ctx, id)
	if err != nil || !ok {
		return err
	}
	if _, exists := t.Nodes[id]; !exists {
		return nil
	}
	out, err := p.repo.Delete(ctx, t.ProjectID, id)
	if errors.Is(err, tree.ErrUnknownNode) {
		return nil // a concurrent delete removed it first
	}
	if err != nil {
		return err
	}
	p.forgetIfEmpty(id, out)
	return nil
}
