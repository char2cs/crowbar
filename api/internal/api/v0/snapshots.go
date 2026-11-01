package v0

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// parseRepoScope splits a hierarchical subscription prefix ("p", "p/r", or
// "p/r/w") into its projectID and repoID. A scope with fewer segments yields
// empty components, which its callers treat as "match all" so a project-level
// or global subscription still snapshots its subtree.
func parseRepoScope(
	scope string,
) (string, string) {
	if scope == "" {
		return "", ""
	}
	segs := strings.Split(scope, "/")
	projectID := segs[0]
	repoID := ""
	if len(segs) > 1 {
		repoID = segs[1]
	}
	return projectID, repoID
}

// projectSnapshot builds the Projects snapshot-on-subscribe source (03 §1a) as
// wire DTOs. Projects sit at the top of the hierarchy, so the scope is either
// empty (list-level subscription) or a bare project id (project-level): either
// way the snapshot returns the full project set and the per-client prefix
// predicate filters it down. A failed list degrades to a nil snapshot.
func projectSnapshot(
	appContainer *app.Container,
) func(scope string) []dto.ProjectDTO {
	return func(_ string) []dto.ProjectDTO {
		rows, err := appContainer.GORM.Projects.FindAll(context.Background())
		if err != nil {
			return nil
		}
		return dto.ProjectDTOList(rows)
	}
}

// repoSnapshot builds the Repos snapshot-on-subscribe source (03 §1a) as wire
// DTOs, scoped to the project parsed from the connecting client's subscription
// prefix ("p/..."). An empty project component matches every project, so a
// list-level subscription snapshots every repo. A failed list degrades to a nil
// snapshot.
func repoSnapshot(
	appContainer *app.Container,
) func(scope string) []dto.RepoDTO {
	return func(scope string) []dto.RepoDTO {
		ctx := context.Background()
		projectID, _ := parseRepoScope(scope)
		rows, err := appContainer.GORM.Repositories.FindAll(ctx)
		if err != nil {
			return nil
		}
		rows = scopeReposToProject(rows, projectID)
		placements, err := repoPlacements(ctx, appContainer, rows)
		if err != nil {
			slog.ErrorContext(ctx, "stream: repo snapshot", "err", err)
			return nil
		}
		return dto.RepoDTOList(rows, placements)
	}
}

// repoPlacements resolves every repo's own sidebar position from its Node row
// (see dto.RepoPlacement); a repo with no row yet sits at the root's first slot.
func repoPlacements(
	ctx context.Context,
	appContainer *app.Container,
	rows []domain.Repository,
) (map[string]dto.RepoPlacement, error) {
	placements := make(map[string]dto.RepoPlacement, len(rows))
	for _, r := range rows {
		n, err := tree.OrUnplaced(appContainer.Repositories.Placement.GetNode(ctx, r.ID))
		if err != nil {
			return nil, fmt.Errorf("repo snapshot: placement of %s: %w", r.ID, err)
		}
		placements[r.ID] = dto.RepoPlacement{FolderID: n.ParentID, Order: n.Order}
	}
	return placements, nil
}

// scopeReposToProject filters rows to those under the given projectID. An empty
// projectID matches every repo, so a list-level scope keeps the full set.
func scopeReposToProject(
	rows []domain.Repository,
	projectID string,
) []domain.Repository {
	if projectID == "" {
		return rows
	}
	out := make([]domain.Repository, 0, len(rows))
	for _, r := range rows {
		if r.ProjectID != projectID {
			continue
		}
		out = append(out, r)
	}
	return out
}
