package tree

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

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
	if err := p.writeTo(ctx, id, at, func() error { _, err := p.repo.Create(ctx, at, n); return err }); err != nil {
		return tree.Node{}, err
	}
	return n, nil
}

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
		return nil
	}
	if err != nil {
		return err
	}
	p.forgetIfEmpty(id, out)
	return nil
}
