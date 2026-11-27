package mocks

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

type RepositoryStore struct {
	Saved        []domain.Repository
	SaveErr      error
	FindErr      error
	FindByKeyErr error
	FindByKeyFn  func(id string) (*domain.Repository, error)
	Nodes        *NodePlacements
}

func NewRepositoryStore() *RepositoryStore {
	return &RepositoryStore{}
}

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
	s.Saved = without(s.Saved, id, func(r domain.Repository) string { return r.ID })
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
