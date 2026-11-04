package mocks

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// TerminalProfileStore is a fake store.Store[domain.TerminalProfile, string].
type TerminalProfileStore struct {
	Saved   []domain.TerminalProfile
	Deleted []string

	SaveErr    error
	DeleteErr  error
	FindAllErr error
}

// NewTerminalProfileStore returns an empty TerminalProfileStore.
func NewTerminalProfileStore() *TerminalProfileStore {
	return &TerminalProfileStore{}
}

func (s *TerminalProfileStore) Save(
	ctx context.Context,
	item domain.TerminalProfile,
) error {
	if s.SaveErr != nil {
		return s.SaveErr
	}
	s.Saved = append(s.Saved, item)
	return nil
}

func (s *TerminalProfileStore) Delete(
	ctx context.Context,
	id string,
) error {
	if s.DeleteErr != nil {
		return s.DeleteErr
	}
	s.Deleted = append(s.Deleted, id)
	return nil
}

func (s *TerminalProfileStore) FindByKey(
	ctx context.Context,
	id string,
) (*domain.TerminalProfile, error) {
	for i := range s.Saved {
		if s.Saved[i].ID == id {
			return &s.Saved[i], nil
		}
	}
	return nil, nil
}

func (s *TerminalProfileStore) FindAll(
	ctx context.Context,
) ([]domain.TerminalProfile, error) {
	if s.FindAllErr != nil {
		return nil, s.FindAllErr
	}
	return s.Saved, nil
}
