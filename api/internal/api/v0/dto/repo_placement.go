package dto

import (
	"context"
	"fmt"
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

type Placements struct {
	at     func(ctx context.Context, repoID string) (tree.Node, error)
	log    func(ctx context.Context, projectID string, err error)
	logged sync.Map
}

func NewPlacements(
	at func(ctx context.Context, repoID string) (tree.Node, error),
	log func(ctx context.Context, projectID string, err error),
) *Placements {
	return &Placements{at: at, log: log}
}

func (p *Placements) Of(ctx context.Context, rows []domain.Repository) map[string]RepoPlacement {
	placements := make(map[string]RepoPlacement, len(rows))
	unreadable := map[string]bool{}
	for _, r := range rows {
		if unreadable[r.ProjectID] {
			continue
		}
		at, err := p.At(ctx, r.ID)
		if err != nil {
			unreadable[r.ProjectID] = true
			p.report(ctx, r.ProjectID, err)
			continue
		}
		placements[r.ID] = at
	}
	for _, r := range rows {
		if unreadable[r.ProjectID] {
			placements[r.ID] = RepoPlacement{}
		}
	}
	return placements
}

func (p *Placements) At(ctx context.Context, repoID string) (RepoPlacement, error) {
	n, err := tree.OrUnplaced(p.at(ctx, repoID))
	if err != nil {
		return RepoPlacement{}, fmt.Errorf("placement of %s: %w", repoID, err)
	}
	return RepoPlacement{FolderID: n.ParentID, Order: n.Order}, nil
}

func (p *Placements) report(ctx context.Context, projectID string, err error) {
	if _, seen := p.logged.LoadOrStore(projectID, true); !seen {
		p.log(ctx, projectID, err)
	}
}
