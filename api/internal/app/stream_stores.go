package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
	domainstream "github.com/char2cs/crowbar/api/internal/domain/stream"
)

type publishingProjects struct {
	store.Store[domain.Project, string]
	late *lateStream
	mu   sync.Mutex
}

func (s *publishingProjects) Save(ctx context.Context, p domain.Project) error {
	if err := s.Store.Save(ctx, p); err != nil {
		return fmt.Errorf("app: save project %s: %w", p.ID, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if row, err := s.FindByKey(ctx, p.ID); err == nil && row != nil {
		s.late.hub.Entity(domainstream.Projects, domainstream.KindProject, p.ID, dto.ProjectDTOFrom(*row), 0)
	}
	return nil
}

func (s *publishingProjects) Delete(ctx context.Context, id string) error {
	if err := s.Store.Delete(ctx, id); err != nil {
		return fmt.Errorf("app: delete project %s: %w", id, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.late.hub.Gone(domainstream.Projects, domainstream.KindProject, id, 0)
	return nil
}

type publishingRepos struct {
	store.ScopedStore[domain.Repository, string]
	entities *repoEntities
}

func (s publishingRepos) Save(ctx context.Context, r domain.Repository) error {
	if err := s.ScopedStore.Save(ctx, r); err != nil {
		return fmt.Errorf("app: save repo %s: %w", r.ID, err)
	}
	s.entities.publish(ctx, r.ID)
	return nil
}

func (s publishingRepos) Delete(ctx context.Context, id string) error {
	if err := s.ScopedStore.Delete(ctx, id); err != nil {
		return fmt.Errorf("app: delete repo %s: %w", id, err)
	}
	s.entities.gone(id)
	return nil
}

type repoEntities struct {
	late       *lateStream
	rows       store.ScopedStore[domain.Repository, string]
	placements *dto.Placements
	mu         sync.Mutex
}

func (p *repoEntities) publish(ctx context.Context, repoID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, err := p.rows.FindByKey(ctx, repoID)
	if err != nil || r == nil {
		return
	}
	at, err := p.placements.At(ctx, repoID)
	if err != nil {
		slog.WarnContext(ctx, "app: repo placement unreadable", "repo", repoID, "err", err)
	}
	p.late.hub.Entity(domainstream.Projects, domainstream.KindRepo, repoID, dto.RepoDTOFrom(*r, at), 0)
}

func (p *repoEntities) gone(repoID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.late.hub.Gone(domainstream.Projects, domainstream.KindRepo, repoID, 0)
}
