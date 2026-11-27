package tree

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

type Ports struct {
	repo   *Repository
	locate Locator

	mu    sync.Mutex
	index map[string]string
	trees map[string]bool
}

func NewPorts(
	ctx context.Context,
	repo *Repository,
	locate Locator,
	treeIDs []string,
) (*Ports, error) {
	p := &Ports{repo: repo, locate: locate, index: map[string]string{}, trees: map[string]bool{tree.Unplaced: true}}
	for _, id := range append([]string{tree.Unplaced}, treeIDs...) {
		t, err := repo.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		p.remember(t)
	}
	return p, nil
}

func (p *Ports) Seed(ctx context.Context, t tree.Tree) (tree.Tree, error) {
	out, err := p.repo.Seed(ctx, p.unheld(t))
	if err != nil {
		return tree.Tree{}, err
	}
	p.remember(out)
	return out, nil
}

func (p *Ports) unheld(t tree.Tree) tree.Tree {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := t.Clone()
	for id, at := range p.index {
		if at != t.ProjectID {
			delete(out.Nodes, id)
			delete(out.Folders, id)
			delete(out.Chats, id)
		}
	}
	return out
}

func (p *Ports) Tree(ctx context.Context, projectID string) (tree.Tree, error) {
	return p.repo.Get(ctx, projectID)
}

func (p *Ports) remember(t tree.Tree) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.trees[t.ProjectID] = true
	for _, ids := range [][]string{keys(t.Nodes), keys(t.Folders), keys(t.Chats)} {
		for _, id := range ids {
			if at, ok := p.index[id]; !ok || at == tree.Unplaced {
				p.index[id] = t.ProjectID
			}
		}
	}
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	return out
}

func (p *Ports) holder(id string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	at, ok := p.index[id]
	return at, ok
}

func (p *Ports) record(id, treeID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.index[id] = treeID
	p.trees[treeID] = true
}

func (p *Ports) writeTo(ctx context.Context, id, at string, write func() error) error {
	_, held := p.holder(id)
	p.record(id, at)
	err := write()
	if err == nil || held {
		return err
	}
	if t, gerr := p.repo.Get(ctx, at); gerr == nil {
		p.forgetIfEmpty(id, t)
	}
	return err
}

func (p *Ports) forgetIfEmpty(id string, t tree.Tree) {
	if t.Holds(id) {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.index[id] == t.ProjectID {
		delete(p.index, id)
	}
}

func (p *Ports) treeOf(ctx context.Context, id string) (tree.Tree, bool, error) {
	at, ok := p.holder(id)
	if !ok {
		return tree.Tree{}, false, nil
	}
	t, err := p.repo.Get(ctx, at)
	return t, err == nil, err
}

func (p *Ports) placeIn(
	ctx context.Context,
	id string,
	kind tree.Kind,
	workspaceID string,
	parentID string,
) (string, error) {
	at, held := p.holder(id)
	if held && at != tree.Unplaced {
		return at, nil
	}
	project, err := p.projectFor(ctx, id, kind, workspaceID, parentID)
	if err != nil || project == "" {
		return tree.Unplaced, err
	}
	if held {
		if err := p.move(ctx, id, at, project); err != nil {
			return "", err
		}
	}
	return project, nil
}

func (p *Ports) projectFor(
	ctx context.Context,
	id string,
	kind tree.Kind,
	workspaceID string,
	parentID string,
) (string, error) {
	if workspaceID != "" {
		return p.locate.ProjectOf(ctx, workspaceID, tree.KindWorkspace)
	}
	project, err := p.locate.ProjectOf(ctx, id, kind)
	if err != nil || project != "" || parentID == "" {
		return project, err
	}
	if at, ok := p.holder(parentID); ok && at != tree.Unplaced {
		return at, nil
	}
	return p.locate.ProjectOf(ctx, parentID, "")
}

func (p *Ports) move(ctx context.Context, id, from, to string) error {
	src, err := p.repo.Get(ctx, from)
	if err != nil {
		return err
	}
	_, taken := src.Without(id)
	if _, err := p.repo.Give(ctx, to, taken); err != nil {
		return err
	}
	p.record(id, to)
	if _, err := p.repo.Take(ctx, from, id); err != nil {
		return err
	}
	return nil
}

func (p *Ports) all(ctx context.Context) ([]tree.Tree, error) {
	p.mu.Lock()
	ids := keys(p.trees)
	p.mu.Unlock()
	slices.Sort(ids)
	out := make([]tree.Tree, 0, len(ids))
	for _, id := range ids {
		t, err := p.repo.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func sortNodes(nodes []tree.Node) []tree.Node {
	slices.SortFunc(nodes, func(a, b tree.Node) int { return cmp.Compare(a.ID, b.ID) })
	return nodes
}

func noRow(op, id string) error {
	return fmt.Errorf("tree: %s: no node %s: %w", op, id, asynxModels.ErrValidation)
}

func (p *Ports) WaitPublish() {
	p.repo.WaitPublish()
}
