package mocks

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// FolderStore is a fake store.Store[tree.Folder, string], where a
// home-scoped folder's name lives.
type FolderStore struct {
	Saved        []tree.Folder
	SaveErr      error
	FindErr      error
	FindByKeyErr error
	DeleteErr    error
	Deleted      []string
}

// NewFolderStore returns an empty FolderStore.
func NewFolderStore() *FolderStore {
	return &FolderStore{}
}

func (s *FolderStore) Save(
	ctx context.Context,
	item tree.Folder,
) error {
	if s.SaveErr != nil {
		return s.SaveErr
	}
	s.Saved = upsert(s.Saved, item, func(f tree.Folder) string { return f.ID })
	return nil
}

func (s *FolderStore) Delete(
	ctx context.Context,
	id string,
) error {
	if s.DeleteErr != nil {
		return s.DeleteErr
	}
	s.Deleted = append(s.Deleted, id)
	s.Saved = without(s.Saved, id, func(f tree.Folder) string { return f.ID })
	return nil
}

// FindByKey returns a pointer to a COPY: Delete compacts s.Saved in place, which would
// overwrite a pointer into the array with another row's data.
func (s *FolderStore) FindByKey(
	ctx context.Context,
	id string,
) (*tree.Folder, error) {
	if s.FindByKeyErr != nil {
		return nil, s.FindByKeyErr
	}
	for i := range s.Saved {
		if s.Saved[i].ID == id {
			found := s.Saved[i]
			return &found, nil
		}
	}
	return nil, nil
}

func (s *FolderStore) FindAll(
	ctx context.Context,
) ([]tree.Folder, error) {
	if s.FindErr != nil {
		return nil, s.FindErr
	}
	return s.Saved, nil
}
