package mocks

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type ProjectStore struct {
	Saved      []domain.Project
	SaveErr    error
	FindErr    error
	FindAllErr error
}

func NewProjectStore() *ProjectStore {
	return &ProjectStore{}
}

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
