package mocks

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ProjectStore is a fake store.Store[domain.Project, string].
type ProjectStore struct {
	Saved   []domain.Project
	SaveErr error
	FindErr error
	// FindAllErr, when set, fails FindAll independently of FindErr (which only
	// guards FindByKey) — so a test can fail the list read without also
	// breaking every by-id lookup on the same store.
	FindAllErr error
}

// NewProjectStore returns an empty ProjectStore.
func NewProjectStore() *ProjectStore {
	return &ProjectStore{}
}

// Save UPSERTS by id, mirroring the real GORM store: re-saving a row replaces
// it rather than shadowing it with a duplicate the reads would then have to pick
// between. Anything that renumbers a list writes the same row more than once.
func (s *ProjectStore) Save(
	ctx context.Context,
	item domain.Project,
) error {
	if s.SaveErr != nil {
		return s.SaveErr
	}
	s.Saved = upsert(s.Saved, item, func(p domain.Project) string { return p.ID })
	return nil
}

// Delete removes the row so Saved reflects the net persisted state (a
// rollback after a Save leaves no row), mirroring RepositoryStore.Delete and
// the real GORM store.
func (s *ProjectStore) Delete(
	ctx context.Context,
	id string,
) error {
	s.Saved = without(s.Saved, id, func(p domain.Project) string { return p.ID })
	return nil
}

func (s *ProjectStore) FindByKey(
	ctx context.Context,
	id string,
) (*domain.Project, error) {
	if s.FindErr != nil {
		return nil, s.FindErr
	}
	for i := range s.Saved {
		if s.Saved[i].ID == id {
			return &s.Saved[i], nil
		}
	}
	return nil, nil
}

func (s *ProjectStore) FindAll(
	ctx context.Context,
) ([]domain.Project, error) {
	if s.FindAllErr != nil {
		return nil, s.FindAllErr
	}
	return s.Saved, nil
}
