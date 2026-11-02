package v0

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	chathandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
	"github.com/char2cs/crowbar/api/internal/app"
	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
	"github.com/char2cs/crowbar/api/internal/engine"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

// newAppAndEngine builds a real app over a real engine, both torn down at
// cleanup, so no test leaks goroutines into the next.
func newAppAndEngine(t *testing.T) (*app.Container, *engine.Container) {
	t.Helper()
	ctx := t.Context()
	eng, err := engine.New(ctx)
	require.NoError(t, err)
	t.Cleanup(eng.Close)
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	a, err := app.New(ctx, eng, adapters, migratetest.NoPostOpen(), EncodeEntity, NewSidebar)
	require.NoError(t, err)
	t.Cleanup(a.Close)
	return a, eng
}

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
	a, _ := newAppAndEngine(t)
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

// The REST repo list and the projects scope's snapshot read positions through
// the app's one Placements, so a broken project is logged once per boot.
func TestRepoLists_ShareOnePlacementsSoABrokenProjectLogsOnce(t *testing.T) {
	a, eng := newAppAndEngine(t)
	ctx := context.Background()
	require.NoError(t, a.GORM.Repositories.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1", Name: "one"}))
	reads, logged := 0, 0
	a.RepoPlacements = dto.NewPlacements(func(context.Context, string) (tree.Node, error) {
		reads++
		return tree.Node{}, errors.New("tree: read: disk gone")
	}, func(context.Context, string, error) { logged++ })
	r := gin.New()
	New(a, eng).Register(r.Group("/v0"))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/v0/projects/p1/repos", nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, []string{"r1"}, sidebarIDs(t, NewSidebar(a), stream.KindRepo))

	assert.Equal(t, 2, reads, "both lists read through the app's Placements")
	assert.Equal(t, 1, logged)
}
