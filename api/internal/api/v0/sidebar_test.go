package v0

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	chathandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

func sidebarIDs(t *testing.T, side appstream.Sidebar, kind stream.EntityKind) []string {
	t.Helper()
	got, err := side.Entities(context.Background())
	require.NoError(t, err)
	var ids []string
	for _, e := range got {
		if e.Kind == kind {
			ids = append(ids, e.ID)
		}
	}
	return ids
}

// One project's unreadable tree lists its repos at the root; every other
// project's repos keep their place, as the REST list does.
func TestSidebar_AnUnreadableTreeOnlyUnplacesItsOwnProject(t *testing.T) {
	a := newAppForSnapshot(t)
	ctx := context.Background()
	require.NoError(t, a.GORM.Projects.Save(ctx, domain.Project{ID: "p1", Name: "one"}))
	require.NoError(t, a.GORM.Projects.Save(ctx, domain.Project{ID: "p2", Name: "two"}))
	require.NoError(t, a.GORM.Repositories.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1", Name: "one"}))
	require.NoError(t, a.GORM.Repositories.Save(ctx, domain.Repository{ID: "r3", ProjectID: "p2", Name: "three"}))
	a.RepoPlacements = dto.NewPlacements(func(ctx context.Context, id string) (tree.Node, error) {
		if id == "r1" {
			return tree.Node{}, errors.New("tree: read: disk gone")
		}
		return a.Repositories.Placement.GetNode(ctx, id)
	}, func(context.Context, string, error) {})

	side := NewSidebar(a)

	assert.ElementsMatch(t, []string{"p1", "p2"}, sidebarIDs(t, side, stream.KindProject))
	assert.ElementsMatch(t, []string{"r1", "r3"}, sidebarIDs(t, side, stream.KindRepo))
}

// A deleted workspace's tombstone row is no sidebar entity: no reader draws one.
func TestSidebar_ADeletedWorkspaceIsNoEntity(t *testing.T) {
	rows := chathandlers.Rows{Workspaces: []dto.WorkspaceDTO{
		{ID: "live", Status: domain.WorkspaceStatusNew},
		{ID: "gone", Status: domain.WorkspaceStatusDeleted},
	}}

	got := appendRows(nil, rows, "p1", "r1")

	require.Len(t, got, 1)
	assert.Equal(t, "live", got[0].ID)
}
