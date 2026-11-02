package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// fakeUpdater records the UpdateRepo call and returns a canned repo/error so the
// Patch handler's HTTP contract can be pinned without a real usecase.
type fakeUpdater struct {
	repo   domain.Repository
	err    error
	gotID  string
	got    project.RepoUpdate
	called bool
}

func (f *fakeUpdater) UpdateRepo(
	_ context.Context,
	repoID string,
	in project.RepoUpdate,
) error {
	f.called = true
	f.gotID = repoID
	f.got = in
	return f.err
}

// patchRouter mounts PATCH .../repos/:repoId on a handler backed by the
// supplied updater (nil = the bare handler with no updater wired).
func patchRouter(
	t *testing.T,
	updater repohandlers.RepoUpdater,
) *gin.Engine {
	t.Helper()
	home := t.TempDir()
	h := repohandlers.NewWithDeps(&fakeStore{}, nil, nil, nil, func() (string, error) { return home, nil }, nil).
		WithUpdater(updater)
	r := gin.New()
	r.PATCH("/v0/projects/:projectId/repos/:repoId", h.Patch)
	return r
}

func doPatch(
	r *gin.Engine,
	target string,
	body any,
) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, target, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

// TestPatchRepo_RenamePassesTheTrimmedName pins the happy path: a valid rename
// passes the trimmed name + repoId to the updater and answers 204.
func TestPatchRepo_RenamePassesTheTrimmedName(t *testing.T) {
	upd := &fakeUpdater{repo: domain.Repository{ID: "r1", ProjectID: "p1", Name: "Renamed Repo"}}
	rec := doPatch(patchRouter(t, upd), "/v0/projects/p1/repos/r1",
		map[string]string{"name": "  Renamed Repo  "})

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.String())
	assert.Equal(t, "r1", upd.gotID)
	require.NotNil(t, upd.got.Name)
	assert.Equal(t, "Renamed Repo", *upd.got.Name, "the handler trims surrounding whitespace")
}

// fakeNodeReader is a fixed map of id -> tree.Node, so a test can pin the
// Order/FolderID a RepoDTO reads off the repo's own Node row without standing
// up the real repository.
type fakeNodeReader map[string]tree.Node

// positionReader is what a test's placements read positions from.
type positionReader interface {
	GetNode(ctx context.Context, id string) (tree.Node, error)
}

func placementsOver(nodes positionReader) *dto.Placements {
	return dto.NewPlacements(nodes.GetNode, func(context.Context, string, error) {})
}

func (f fakeNodeReader) GetNode(_ context.Context, id string) (tree.Node, error) {
	n, ok := f[id]
	if !ok {
		return tree.Node{}, problem.ErrNotFound
	}
	return n, nil
}

// TestPatchRepo_OrderOnlyNeedsNoName pins that the PATCH is genuinely partial: a
// reorder carries no name, and the old name-is-required rule would have 400'd
// every drag.
func TestPatchRepo_OrderOnlyNeedsNoName(t *testing.T) {
	upd := &fakeUpdater{repo: domain.Repository{ID: "r1", ProjectID: "p1"}}
	rec := doPatch(patchRouter(t, upd), "/v0/projects/p1/repos/r1", map[string]any{"order": 2})

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Nil(t, upd.got.Name, "an order-only PATCH must not synthesise a rename")
	require.NotNil(t, upd.got.Order)
	assert.Equal(t, 2, *upd.got.Order)
}

// TestPatchRepo_EmptyName_400 pins synchronous name validation: a blank name
// (whitespace only) is rejected 400 before the updater runs.
func TestPatchRepo_EmptyName_400(t *testing.T) {
	upd := &fakeUpdater{}
	rec := doPatch(patchRouter(t, upd), "/v0/projects/p1/repos/r1",
		map[string]string{"name": "   "})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.False(t, upd.called, "the updater must not run on an invalid name")
}

// TestPatchRepo_NotFound_404 pins that an unknown repo (updater returns
// ErrNotFound) surfaces as a 404, not a 500.
func TestPatchRepo_NotFound_404(t *testing.T) {
	upd := &fakeUpdater{err: problem.ErrNotFound}
	rec := doPatch(patchRouter(t, upd), "/v0/projects/p1/repos/missing",
		map[string]string{"name": "Whatever"})

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestPatchRepo_NoUpdater_500 pins the bare-handler fallback: with no updater
// wired the endpoint answers 500 rather than panicking.
func TestPatchRepo_NoUpdater_500(t *testing.T) {
	rec := doPatch(patchRouter(t, nil), "/v0/projects/p1/repos/r1",
		map[string]string{"name": "Whatever"})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// The patch endpoint takes the same unsanitised name the create endpoint does,
// and feeds it to the same slug fallback — so it needs the same guard, or a
// rename walks an existing repo's future worktrees out of the crowbar home.
func TestPatchRepo_RejectsNamesThatEscapeTheCrowbarHome(t *testing.T) {
	for _, c := range traversalNames {
		t.Run(c.name, func(t *testing.T) {
			upd := &fakeUpdater{}
			rec := doPatch(patchRouter(t, upd), "/v0/projects/p1/repos/r1",
				map[string]string{"name": c.value})

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.False(t, upd.called, "the updater must not run on an unsafe name")
		})
	}
}

func TestPatchRepo_MalformedBody_400(t *testing.T) {
	upd := &fakeUpdater{}
	r := patchRouter(t, upd)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/v0/projects/p1/repos/r1", bytes.NewReader([]byte("{")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.False(t, upd.called)
}

// A handler built without a crowbar home would resolve one of its own later,
// so both constructors refuse it up front.
func TestNew_NilCrowbarHomePanics(t *testing.T) {
	assert.Panics(t, func() { repohandlers.New(&fakeStore{}, nil) })
	assert.Panics(t, func() { repohandlers.NewWithDeps(&fakeStore{}, nil, nil, nil, nil, nil) })
}

// brokenNodes fails every position read the way an unreadable tree does.
type brokenNodes struct{}

func (brokenNodes) GetNode(context.Context, string) (tree.Node, error) {
	return tree.Node{}, fmt.Errorf("tree: read p1: %w", errors.New("disk gone"))
}

// brokenProject fails the position reads of p1's repo only.
type brokenProject struct{}

func (brokenProject) GetNode(_ context.Context, id string) (tree.Node, error) {
	if id == "r1" {
		return tree.Node{}, fmt.Errorf("tree: read p1: %w", errors.New("disk gone"))
	}
	return tree.Node{ParentID: "f2", Order: 3}, nil
}

func placementRouter(t *testing.T, nodes positionReader) *gin.Engine {
	t.Helper()
	repo := domain.Repository{ID: "r1", ProjectID: "p1", Name: "one"}
	other := domain.Repository{ID: "r2", ProjectID: "p2", Name: "two"}
	store := &fakeStore{byKey: &repo, all: []domain.Repository{repo, other}}
	h := repohandlers.NewWithDeps(store, nil, nil, nil,
		func() (string, error) { return t.TempDir(), nil }, placementsOver(nodes))
	r := gin.New()
	r.GET("/v0/repos", h.List)
	r.GET("/v0/projects/:projectId/repos/:repoId", h.Detail)
	return r
}

// A failed position read of the one repo asked for fails that read, rather
// than answer the repo silently moved to the root.
func TestDetail_AFailedPositionReadFailsTheRead(t *testing.T) {
	r := placementRouter(t, brokenNodes{})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v0/projects/p1/repos/r1", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.JSONEq(t, `{"success":false,"code":"internal","error":"repos: placement of r1: tree: read p1: disk gone"}`,
		rec.Body.String())
}

// One project's unreadable tree does not fail the list of every project: its
// repos are listed at the root, the others where they are.
func TestList_AnUnreadableTreeOnlyUnplacesItsOwnProject(t *testing.T) {
	r := placementRouter(t, brokenProject{})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v0/repos", nil))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var env struct{ Data []dto.RepoDTO }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	placed := map[string][2]any{}
	for _, d := range env.Data {
		placed[d.ID] = [2]any{d.FolderID, d.Order}
	}
	assert.Equal(t, map[string][2]any{"r1": {"", 0}, "r2": {"f2", 3}}, placed)
}

func TestDetail_ARepoWithNoPositionRowYetSitsAtTheRoot(t *testing.T) {
	r := placementRouter(t, fakeNodeReader{})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v0/projects/p1/repos/r1", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var env struct{ Data dto.RepoDTO }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.Empty(t, env.Data.FolderID)
	assert.Equal(t, 0, env.Data.Order)
}
