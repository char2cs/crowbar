package tree

import (
	"cmp"
	"context"
	"errors"
	"slices"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// The folders' identities, by id, as the placement usecases read and write them.

// FindByKey is folder id's identity, or nil when there is none.
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

// FindAll is every folder's identity.
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

// Save writes f's identity in the tree of the project its scope names.
func (p *Ports) Save(ctx context.Context, f tree.Folder) error {
	at, err := p.folderTree(ctx, f)
	if err != nil {
		return err
	}
	return p.writeTo(ctx, f.ID, at, func() error { _, err := p.repo.PutFolder(ctx, at, f); return err })
}

// folderTree is the tree f is kept in: the one holding it, unless that is
// Unplaced and f's scope now names a project, in which case its records move.
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

// Delete removes folder id's identity; one that is not there is no error.
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
		return nil // a concurrent delete removed it first
	}
	if err != nil {
		return err
	}
	p.forgetIfEmpty(id, out)
	return nil
}
