package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos/handlers"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
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
) (domain.Repository, error) {
	f.called = true
	f.gotID = repoID
	f.got = in
	return f.repo, f.err
}

// patchRouter mounts PATCH .../repos/:repoId on a handler whose broadcast
// fan-out is captured into frames, backed by the supplied updater (nil = the
// bare handler with no updater wired).
func patchRouter(
	t *testing.T,
	updater repohandlers.RepoUpdater,
	frames *[]dto.RepoDTO,
) *gin.Engine {
	t.Helper()
	// Inject a throwaway crowbar home. The default resolver is the process-global
	// one, so without this the icon paths a broadcast derives land in the
	// developer's real ~/.crowbar — this test moved a repo to project "p2" and
	// created a projects/p2 directory there, which is how it was found.
	home := t.TempDir()
	h := repohandlers.NewWithDeps(&fakeStore{}, nil, nil, func(d dto.RepoDTO) {
		*frames = append(*frames, d)
	}).WithUpdater(updater).
		WithIconStorage(func() (string, error) { return home, nil }, nil)
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

// TestPatchRepo_BroadcastsRenamedDTO pins the happy path: a valid rename passes
// the trimmed name + repoId to the updater, answers 204, and broadcasts the
// updated RepoDTO on the repos WS stream (the sidebar refresh, not the response).
func TestPatchRepo_BroadcastsRenamedDTO(t *testing.T) {
	upd := &fakeUpdater{repo: domain.Repository{
		ID: "r1", ProjectID: "p1", Name: "Renamed Repo", AvatarLabel: "R",
	}}
	var frames []dto.RepoDTO
	rec := doPatch(patchRouter(t, upd, &frames), "/v0/projects/p1/repos/r1",
		map[string]string{"name": "  Renamed Repo  "})

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.String())
	assert.Equal(t, "r1", upd.gotID)
	require.NotNil(t, upd.got.Name)
	assert.Equal(t, "Renamed Repo", *upd.got.Name, "the handler trims surrounding whitespace")
	require.Len(t, frames, 1, "a rename must broadcast the updated RepoDTO")
	assert.Equal(t, "Renamed Repo", frames[0].Name)
	assert.Equal(t, "R", frames[0].AvatarLabel)
}

// TestPatchRepo_OrderOnlyNeedsNoName pins that the PATCH is genuinely partial: a
// reorder carries no name, and the old name-is-required rule would have 400'd
// every drag.
func TestPatchRepo_OrderOnlyNeedsNoName(t *testing.T) {
	upd := &fakeUpdater{repo: domain.Repository{ID: "r1", ProjectID: "p1", Order: 2}}
	var frames []dto.RepoDTO
	rec := doPatch(patchRouter(t, upd, &frames), "/v0/projects/p1/repos/r1",
		map[string]any{"order": 2})

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Nil(t, upd.got.Name, "an order-only PATCH must not synthesise a rename")
	require.NotNil(t, upd.got.Order)
	assert.Equal(t, 2, *upd.got.Order)
	require.Len(t, frames, 1)
	assert.Equal(t, 2, frames[0].Order, "the broadcast carries the new order")
}

// TestPatchRepo_ProjectMovePassesTargetThrough pins that projectId reaches the
// usecase, which is what carries the repo's workspaces across with it.
func TestPatchRepo_ProjectMovePassesTargetThrough(t *testing.T) {
	upd := &fakeUpdater{repo: domain.Repository{ID: "r1", ProjectID: "p2"}}
	var frames []dto.RepoDTO
	rec := doPatch(patchRouter(t, upd, &frames), "/v0/projects/p1/repos/r1",
		map[string]any{"projectId": "p2"})

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.NotNil(t, upd.got.ProjectID)
	assert.Equal(t, "p2", *upd.got.ProjectID)
	require.Len(t, frames, 1)
	assert.Equal(t, "p2", frames[0].ProjectID)
}

// TestPatchRepo_EmptyName_400 pins synchronous name validation: a blank name
// (whitespace only) is rejected 400 before the updater runs, and nothing is
// broadcast.
func TestPatchRepo_EmptyName_400(t *testing.T) {
	upd := &fakeUpdater{}
	var frames []dto.RepoDTO
	rec := doPatch(patchRouter(t, upd, &frames), "/v0/projects/p1/repos/r1",
		map[string]string{"name": "   "})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.False(t, upd.called, "the updater must not run on an invalid name")
	assert.Empty(t, frames)
}

// TestPatchRepo_NotFound_404 pins that an unknown repo (updater returns
// ErrNotFound) surfaces as a 404, not a 500.
func TestPatchRepo_NotFound_404(t *testing.T) {
	upd := &fakeUpdater{err: apperr.ErrNotFound}
	var frames []dto.RepoDTO
	rec := doPatch(patchRouter(t, upd, &frames), "/v0/projects/p1/repos/missing",
		map[string]string{"name": "Whatever"})

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, frames, "a failed update broadcasts nothing")
}

// TestPatchRepo_NoUpdater_500 pins the bare-handler fallback: with no updater
// wired the endpoint answers 500 rather than panicking.
func TestPatchRepo_NoUpdater_500(t *testing.T) {
	var frames []dto.RepoDTO
	rec := doPatch(patchRouter(t, nil, &frames), "/v0/projects/p1/repos/r1",
		map[string]string{"name": "Whatever"})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Empty(t, frames)
}

// The patch endpoint takes the same unsanitised name the create endpoint does,
// and feeds it to the same slug fallback — so it needs the same guard, or a
// rename walks an existing repo's future worktrees out of the crowbar home.
func TestPatchRepo_RejectsNamesThatEscapeTheCrowbarHome(t *testing.T) {
	for _, c := range traversalNames {
		t.Run(c.name, func(t *testing.T) {
			upd := &fakeUpdater{}
			var frames []dto.RepoDTO
			rec := doPatch(patchRouter(t, upd, &frames), "/v0/projects/p1/repos/r1",
				map[string]string{"name": c.value})

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.False(t, upd.called, "the updater must not run on an unsafe name")
			assert.Empty(t, frames)
		})
	}
}

// A repo that changes projects takes its ENTITY DIRECTORY with it — the icon
// store, keyed by <home>/projects/<projectId>/<repoId>. Left behind, the icon
// 404s from under the new path and the old directory outlives every way of
// reaching it.
func TestPatchRepo_ProjectMoveRelocatesTheEntityDir(t *testing.T) {
	home := t.TempDir()
	from := filepath.Join(home, "projects", "p1", "r1")
	require.NoError(t, os.MkdirAll(from, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(from, "icon"), []byte("bytes"), 0o600))

	upd := &fakeUpdater{repo: domain.Repository{ID: "r1", ProjectID: "p2"}}
	h := repohandlers.NewWithDeps(&fakeStore{}, nil, nil, func(dto.RepoDTO) {}).
		WithUpdater(upd).
		WithIconStorage(func() (string, error) { return home, nil }, nil)
	r := gin.New()
	r.PATCH("/v0/projects/:projectId/repos/:repoId", h.Patch)

	rec := doPatch(r, "/v0/projects/p1/repos/r1", map[string]any{"projectId": "p2"})
	require.Equal(t, http.StatusNoContent, rec.Code)

	moved, err := os.ReadFile(filepath.Join(home, "projects", "p2", "r1", "icon"))
	require.NoError(t, err, "the icon follows the repo to its new project")
	assert.Equal(t, "bytes", string(moved))
	_, err = os.Stat(from)
	assert.True(t, os.IsNotExist(err), "nothing is left behind at the old path")
}

// A repo with no entity dir yet (no custom icon) must move cleanly rather than
// fail on a rename with nothing to rename.
func TestPatchRepo_ProjectMoveToleratesAMissingEntityDir(t *testing.T) {
	home := t.TempDir()
	upd := &fakeUpdater{repo: domain.Repository{ID: "r1", ProjectID: "p2"}}
	h := repohandlers.NewWithDeps(&fakeStore{}, nil, nil, func(dto.RepoDTO) {}).
		WithUpdater(upd).
		WithIconStorage(func() (string, error) { return home, nil }, nil)
	r := gin.New()
	r.PATCH("/v0/projects/:projectId/repos/:repoId", h.Patch)

	rec := doPatch(r, "/v0/projects/p1/repos/r1", map[string]any{"projectId": "p2"})
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

// A rename never touches the entity dir: only a project change moves it.
func TestPatchRepo_RenameLeavesTheEntityDirAlone(t *testing.T) {
	home := t.TempDir()
	at := filepath.Join(home, "projects", "p1", "r1")
	require.NoError(t, os.MkdirAll(at, 0o755))

	upd := &fakeUpdater{repo: domain.Repository{ID: "r1", ProjectID: "p1", Name: "new"}}
	h := repohandlers.NewWithDeps(&fakeStore{}, nil, nil, func(dto.RepoDTO) {}).
		WithUpdater(upd).
		WithIconStorage(func() (string, error) { return home, nil }, nil)
	r := gin.New()
	r.PATCH("/v0/projects/:projectId/repos/:repoId", h.Patch)

	rec := doPatch(r, "/v0/projects/p1/repos/r1", map[string]any{"name": "new"})
	require.Equal(t, http.StatusNoContent, rec.Code)
	_, err := os.Stat(at)
	assert.NoError(t, err)
}

func TestPatchRepo_MalformedBody_400(t *testing.T) {
	upd := &fakeUpdater{}
	var frames []dto.RepoDTO
	r := patchRouter(t, upd, &frames)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/v0/projects/p1/repos/r1", bytes.NewReader([]byte("{")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.False(t, upd.called)
}

// A home that cannot be resolved is not a reason to fail an update that already
// committed: the icon degrades to the generated avatar and the move stands.
func TestPatchRepo_ProjectMoveSurvivesAnUnresolvableHome(t *testing.T) {
	upd := &fakeUpdater{repo: domain.Repository{ID: "r1", ProjectID: "p2"}}
	var frames []dto.RepoDTO
	h := repohandlers.NewWithDeps(&fakeStore{}, nil, nil, func(d dto.RepoDTO) {
		frames = append(frames, d)
	}).
		WithUpdater(upd).
		WithIconStorage(func() (string, error) { return "", errors.New("no home") }, nil)
	r := gin.New()
	r.PATCH("/v0/projects/:projectId/repos/:repoId", h.Patch)

	rec := doPatch(r, "/v0/projects/p1/repos/r1", map[string]any{"projectId": "p2"})

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Len(t, frames, 1, "the committed move is still broadcast")
}
