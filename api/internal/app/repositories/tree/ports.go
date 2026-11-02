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

// Ports serves today's id-keyed placement reads and writes (the position rows,
// the folder identities, each chat's own placement) over the per-project Trees.
// It keeps which tree holds each id: filled from every tree on open, kept on
// every write. A row whose project cannot be named yet sits in tree.Unplaced
// until a later write names it.
type Ports struct {
	repo   *Repository
	locate Locator

	mu    sync.Mutex
	index map[string]string // id → the tree holding its records
	trees map[string]bool
}

// NewPorts indexes every tree in treeIDs (each project's, and tree.Unplaced).
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

// Seed merges t into its project's tree and indexes what it holds. It leaves
// out every id another tree holds: a write placed it there since t was read.
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

// Tree is projectID's tree as stored.
func (p *Ports) Tree(ctx context.Context, projectID string) (tree.Tree, error) {
	return p.repo.Get(ctx, projectID)
}

func (p *Ports) remember(t tree.Tree) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.trees[t.ProjectID] = true
	for _, ids := range [][]string{keys(t.Nodes), keys(t.Folders), keys(t.Chats)} {
		for _, id := range ids {
			// A row a project names wins over its Unplaced copy left by a cut-short move.
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

// writeTo runs write, which may be tree at's first record of id, with id
// indexed first: the record is readable once committed, before write returns,
// so another write that read it must find where it is held.
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

// forgetIfEmpty drops id from the index once t holds nothing of it.
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

// treeOf is the tree holding id, or "" when none does.
func (p *Ports) treeOf(ctx context.Context, id string) (tree.Tree, bool, error) {
	at, ok := p.holder(id)
	if !ok {
		return tree.Tree{}, false, nil
	}
	t, err := p.repo.Get(ctx, at)
	return t, err == nil, err
}

// placeIn is the tree a write of id goes to: the one holding it, unless that is
// Unplaced and a project can now be named, in which case its records move.
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

// move carries every record of id from one tree to another: written to the
// target first, so a move cut short leaves a copy, never nothing.
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

// all is every indexed tree, read once.
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

// WaitPublish waits until every placement written so far was announced.
func (p *Ports) WaitPublish() {
	p.repo.WaitPublish()
}
