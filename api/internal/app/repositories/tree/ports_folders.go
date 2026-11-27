package tree

import (
	"cmp"
	"context"
	"errors"
	"slices"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

func (p *Ports) FindByKey(ctx context.Context, id string) (*tree.Folder, error) {
	t, ok, err := p.treeOf(ctx, id)
	if err != nil || !ok {
		return nil, err
	}
	f, exists := t.Folders[id]
	if !exists {
		return nil, nil
	}
	return &f, nil
}

func (p *Ports) FindAll(ctx context.Context) ([]tree.Folder, error) {
	trees, err := p.all(ctx)
	if err != nil {
		return nil, err
	}
	var out []tree.Folder
	for _, t := range trees {
		for _, f := range t.Folders {
			out = append(out, f)
		}
	}
	slices.SortFunc(out, func(a, b tree.Folder) int { return cmp.Compare(a.ID, b.ID) })
	return out, nil
}

func (p *Ports) Save(ctx context.Context, f tree.Folder) error {
	at, err := p.folderTree(ctx, f)
	if err != nil {
		return err
	}
	return p.writeTo(ctx, f.ID, at, func() error { _, err := p.repo.PutFolder(ctx, at, f); return err })
}

func (p *Ports) folderTree(ctx context.Context, f tree.Folder) (string, error) {
	at, held := p.holder(f.ID)
	if held && at != tree.Unplaced {
		return at, nil
	}
	scope, kind := f.HomeID, tree.KindWorkspace
	if f.RepoID != "" {
		scope, kind = f.RepoID, tree.KindRepo
	}
	project, err := p.locate.ProjectOf(ctx, scope, kind)
	if err != nil || project == "" {
		return tree.Unplaced, err
	}
	if held {
		if err := p.move(ctx, f.ID, at, project); err != nil {
			return "", err
		}
	}
	return project, nil
}

func (p *Ports) Delete(ctx context.Context, id string) error {
	t, ok, err := p.treeOf(ctx, id)
	if err != nil || !ok {
		return err
	}
	if _, exists := t.Folders[id]; !exists {
		return nil
	}
	out, err := p.repo.DeleteFolder(ctx, t.ProjectID, id)
	if errors.Is(err, tree.ErrUnknownNode) {
		return nil
	}
	if err != nil {
		return err
	}
	p.forgetIfEmpty(id, out)
	return nil
}
