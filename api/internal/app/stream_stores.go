package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
	domainstream "github.com/char2cs/crowbar/api/internal/domain/stream"
)

// publishingProjects publishes every project write to the projects scope: the
// import and delete flows write rows no route broadcasts.
type publishingProjects struct {
	store.Store[domain.Project, string]
	late *lateStream
}

func (s publishingProjects) Save(ctx context.Context, p domain.Project) error {
	if err := s.Store.Save(ctx, p); err != nil {
		return fmt.Errorf("app: save project %s: %w", p.ID, err)
	}
	s.late.hub.Entity(domainstream.Projects, domainstream.KindProject, p.ID, dto.ProjectDTOFrom(p), 0)
	return nil
}

func (s publishingProjects) Delete(ctx context.Context, id string) error {
	if err := s.Store.Delete(ctx, id); err != nil {
		return fmt.Errorf("app: delete project %s: %w", id, err)
	}
	s.late.hub.Gone(domainstream.Projects, domainstream.KindProject, id, 0)
	return nil
}

// publishingRepos is publishingProjects for repos, each at its own placement.
type publishingRepos struct {
	store.ScopedStore[domain.Repository, string]
	late       *lateStream
	placements *dto.Placements
}

func (s publishingRepos) Save(ctx context.Context, r domain.Repository) error {
	if err := s.ScopedStore.Save(ctx, r); err != nil {
		return fmt.Errorf("app: save repo %s: %w", r.ID, err)
	}
	at, err := s.placements.At(ctx, r.ID)
	if err != nil {
		// An unreadable tree lists the repo at the root, as GET does.
		slog.WarnContext(ctx, "app: repo placement unreadable", "repo", r.ID, "err", err)
	}
	s.late.hub.Entity(domainstream.Projects, domainstream.KindRepo, r.ID, dto.RepoDTOFrom(r, at), 0)
	return nil
}

func (s publishingRepos) Delete(ctx context.Context, id string) error {
	if err := s.ScopedStore.Delete(ctx, id); err != nil {
		return fmt.Errorf("app: delete repo %s: %w", id, err)
	}
	s.late.hub.Gone(domainstream.Projects, domainstream.KindRepo, id, 0)
	return nil
}
