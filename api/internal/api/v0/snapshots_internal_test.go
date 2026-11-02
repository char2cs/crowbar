package v0

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
	"github.com/char2cs/crowbar/api/internal/engine"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

var errSnapshotFake = errors.New("snapshot fake")

type errWorkspaceRepo struct {
	workspace.Workspace
}

func (errWorkspaceRepo) List(
	_ context.Context,
) ([]domain.Workspace, error) {
	return nil, errSnapshotFake
}

// errProjectStore is a project store whose FindAll always fails, exercising the
// snapshot's degrade-to-nil path.
type errProjectStore struct {
	store.Store[domain.Project, string]
}

func (errProjectStore) FindAll(
	_ context.Context,
) ([]domain.Project, error) {
	return nil, errSnapshotFake
}

// errRepoStore is a repository store whose FindAll always fails, exercising the
// snapshot's degrade-to-nil path.
type errRepoStore struct {
	store.ScopedStore[domain.Repository, string]
}

func (errRepoStore) FindAll(
	_ context.Context,
) ([]domain.Repository, error) {
	return nil, errSnapshotFake
}

// errReviewThreadRepo is a ReviewThread repo whose ListByWorkspace always
// fails, exercising threadsSnapshot's degrade-to-nil path.
type errReviewThreadRepo struct {
	reviewthread.ReviewThread
}

func (errReviewThreadRepo) ListByWorkspace(
	_ context.Context,
	_ string,
) ([]domain.ReviewThread, error) {
	return nil, errSnapshotFake
}

// newAppAndEngine builds a real app over a real engine, both torn down at
// cleanup: app.Close stops the asynx pools and t.Context's cancel stops the
// background sweeps, so no test leaks goroutines into the next.
func newAppAndEngine(
	t *testing.T,
) (*app.Container, *engine.Container) {
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

func newAppForSnapshot(
	t *testing.T,
) *app.Container {
	t.Helper()
	a, _ := newAppAndEngine(t)
	return a
}

func seedWorkspace(
	t *testing.T,
	a *app.Container,
	id string,
	projectID string,
	repoID string,
	branch string,
	parentID string,
) {
	t.Helper()
	_, err := a.Repositories.Workspace.Create(
		context.Background(),
		workspace.CreateInput{
			ID:           id,
			ProjectID:    projectID,
			RepoID:       repoID,
			Branch:       branch,
			ParentID:     parentID,
			Provisioning: domain.WorkspacePlaceholder,
		},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)
	// The workspace store projection is async (Send, not SendWait): drain every
	// per-type asynx instance so the new row is visible in the read model, then
	// assert its presence synchronously before the snapshot reads it.
	a.Repositories.WaitQuiescent()
	rows, err := a.Repositories.Workspace.List(context.Background())
	require.NoError(t, err)
	found := false
	for _, r := range rows {
		if r.ID == id {
			found = true
			break
		}
	}
	require.True(t, found, "seeded workspace %q must be visible in the read model after drain", id)
}

// TestProjectSnapshot proves the Projects snapshot-on-subscribe (03 §1a)
// returns every project row as wire DTOs. Projects sit at the top of the
// hierarchy, so a list-level (empty) or project-level scope both snapshot the
// full project set; the per-client predicate then filters by prefix.
func TestProjectSnapshot(t *testing.T) {
	a := newAppForSnapshot(t)
	ctx := context.Background()
	require.NoError(t, a.GORM.Projects.Save(ctx, domain.Project{ID: "p1", Name: "Alpha"}))
	require.NoError(t, a.GORM.Projects.Save(ctx, domain.Project{ID: "p2", Name: "Beta"}))

	got := projectSnapshot(a)("")
	require.Len(t, got, 2)

	byID := map[string]string{}
	for _, d := range got {
		byID[d.ID] = d.Name
	}
	assert.Equal(t, "Alpha", byID["p1"])
	assert.Equal(t, "Beta", byID["p2"])
}

// TestProjectSnapshot_ListErrorReturnsNil proves a failed project list degrades
// to a nil snapshot rather than panicking.
func TestProjectSnapshot_ListErrorReturnsNil(t *testing.T) {
	a := newAppForSnapshot(t)
	a.GORM.Projects = errProjectStore{}
	assert.Nil(t, projectSnapshot(a)(""))
}

// TestRepoSnapshot proves the Repos snapshot-on-subscribe (03 §1a) returns the
// repos under the project parsed from the client's subscription prefix ("p/..."),
// as wire DTOs. Repos under a sibling project are excluded.
func TestRepoSnapshot(t *testing.T) {
	a := newAppForSnapshot(t)
	ctx := context.Background()
	require.NoError(t, a.GORM.Repositories.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1", Name: "one"}))
	require.NoError(t, a.GORM.Repositories.Save(ctx, domain.Repository{ID: "r2", ProjectID: "p1", Name: "two"}))
	require.NoError(t, a.GORM.Repositories.Save(ctx, domain.Repository{ID: "r3", ProjectID: "p2", Name: "three"}))

	got := repoSnapshot(a)("p1")
	require.Len(t, got, 2)

	byID := map[string]string{}
	for _, d := range got {
		byID[d.ID] = d.ProjectID
	}
	assert.Equal(t, "p1", byID["r1"])
	assert.Equal(t, "p1", byID["r2"])
	_, hasR3 := byID["r3"]
	assert.False(t, hasR3, "sibling-project repo must be excluded")
}

// TestRepoSnapshot_EmptyScopeReturnsAll proves a list-level (empty) scope keeps
// every repo: an empty project component matches all projects.
func TestRepoSnapshot_EmptyScopeReturnsAll(t *testing.T) {
	a := newAppForSnapshot(t)
	ctx := context.Background()
	require.NoError(t, a.GORM.Repositories.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	require.NoError(t, a.GORM.Repositories.Save(ctx, domain.Repository{ID: "r3", ProjectID: "p2"}))

	got := repoSnapshot(a)("")
	assert.Len(t, got, 2)
}

// TestRepoSnapshot_ListErrorReturnsNil proves a failed repo list degrades to a
// nil snapshot.
func TestRepoSnapshot_ListErrorReturnsNil(t *testing.T) {
	a := newAppForSnapshot(t)
	a.GORM.Repositories = errRepoStore{}
	assert.Nil(t, repoSnapshot(a)("p1"))
}

// A list-level subscriber still receives every repo when one project's tree
// cannot be read: that project's repos are at the root.
func TestRepoSnapshot_AnUnreadableTreeOnlyUnplacesItsOwnProject(t *testing.T) {
	a := newAppForSnapshot(t)
	ctx := context.Background()
	require.NoError(t, a.GORM.Repositories.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1", Name: "one"}))
	require.NoError(t, a.GORM.Repositories.Save(ctx, domain.Repository{ID: "r3", ProjectID: "p2", Name: "three"}))

	failingForR1 := func(ctx context.Context, id string) (tree.Node, error) {
		if id == "r1" {
			return tree.Node{}, errors.New("tree: read: disk gone")
		}
		return a.Repositories.Placement.GetNode(ctx, id)
	}

	a.RepoPlacements = dto.NewPlacements(failingForR1, func(context.Context, string, error) {})

	got := repoSnapshot(a)("")

	ids := make([]string, 0, len(got))
	for _, d := range got {
		ids = append(ids, d.ID)
	}
	assert.ElementsMatch(t, []string{"r1", "r3"}, ids)
}

// The REST repo list and the stream's repo snapshot read positions through the
// app's one Placements, so a broken project is logged once per boot, not once each.
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
	require.Len(t, reposDef(a).Snapshot(""), 1)

	assert.Equal(t, 2, reads, "both lists read through the app's Placements")
	assert.Equal(t, 1, logged)
}
