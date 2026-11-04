package mocks

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// RepositoryStore is a fake store.Store[domain.Repository, string].
type RepositoryStore struct {
	Saved   []domain.Repository
	SaveErr error
	// FindErr, when set, fails FindAll — the read failure that callers such as
	// CheckRepoImportable deliberately degrade past rather than block on.
	FindErr error
	// FindByKeyErr, when set, fails FindByKey only — separate from FindErr so a
	// test can fail the single-row lookup without also breaking FindAll/FindWhere.
	FindByKeyErr error
	// FindByKeyFn, when set, overrides the lookup so a test can tell repeat FindByKey
	// calls for the same id apart.
	FindByKeyFn func(id string) (*domain.Repository, error)
	// Nodes, when set, gets a repo Node row for every NEW repo saved — the
	// row importOneRepo mints alongside it in production, so a seeded repo is
	// never Node-less (all placement lives on the Node model).
	Nodes *NodePlacements
}

// NewRepositoryStore returns an empty RepositoryStore.
func NewRepositoryStore() *RepositoryStore {
	return &RepositoryStore{}
}

// Save UPSERTS by id, mirroring the real GORM store: re-saving a row replaces
// it rather than shadowing it with a duplicate the reads would then have to pick
// between. Anything that renumbers a list writes the same row more than once.
func (s *RepositoryStore) Save(
	ctx context.Context,
	item domain.Repository,
) error {
	if s.SaveErr != nil {
		return s.SaveErr
	}
	for i := range s.Saved {
		if s.Saved[i].ID == item.ID {
			s.Saved[i] = item
			return nil
		}
	}
	s.Saved = append(s.Saved, item)
	if s.Nodes != nil {
		if _, err := s.Nodes.GetNode(ctx, item.ID); err != nil {
			s.Nodes.Rows = append(s.Nodes.Rows, tree.Node{ID: item.ID, Kind: tree.KindRepo})
		}
	}
	return nil
}

func (s *RepositoryStore) Delete(
	ctx context.Context,
	id string,
) error {
	// Remove the row so Saved reflects the net persisted state (a rollback after a
	// Save leaves no row), matching the real store.
	kept := s.Saved[:0]
	for _, r := range s.Saved {
		if r.ID != id {
			kept = append(kept, r)
		}
	}
	s.Saved = kept
	return nil
}

func (s *RepositoryStore) FindByKey(
	ctx context.Context,
	id string,
) (*domain.Repository, error) {
	if s.FindByKeyFn != nil {
		return s.FindByKeyFn(id)
	}
	if s.FindByKeyErr != nil {
		return nil, s.FindByKeyErr
	}
	for i := range s.Saved {
		if s.Saved[i].ID == id {
			return &s.Saved[i], nil
		}
	}
	return nil, nil
}

func (s *RepositoryStore) FindAll(
	ctx context.Context,
) ([]domain.Repository, error) {
	if s.FindErr != nil {
		return nil, s.FindErr
	}
	return s.Saved, nil
}

// FindWhere mirrors the real store's prototype-scoped query: it matches on the
// non-zero fields of match, which for the repository table is the ProjectID the
// sidebar reorder narrows by.
func (s *RepositoryStore) FindWhere(
	ctx context.Context,
	match domain.Repository,
) ([]domain.Repository, error) {
	if s.FindErr != nil {
		return nil, s.FindErr
	}
	rows := make([]domain.Repository, 0, len(s.Saved))
	for _, r := range s.Saved {
		if match.ProjectID != "" && r.ProjectID != match.ProjectID {
			continue
		}
		if match.ID != "" && r.ID != match.ID {
			continue
		}
		rows = append(rows, r)
	}
	return rows, nil
}
