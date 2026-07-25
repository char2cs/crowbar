package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos/handlers"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// fakeRenamer records the RenameRepo call and returns a canned repo/error so the
// Rename handler's HTTP contract can be pinned without a real usecase.
type fakeRenamer struct {
	repo    domain.Repository
	err     error
	gotID   string
	gotName string
	called  bool
}

func (f *fakeRenamer) RenameRepo(
	_ context.Context,
	repoID string,
	name string,
) (domain.Repository, error) {
	f.called = true
	f.gotID = repoID
	f.gotName = name
	return f.repo, f.err
}

// renameRouter mounts PATCH .../repos/:repoId on a handler whose broadcast
// fan-out is captured into frames, backed by the supplied renamer (nil = the
// bare handler with no renamer wired).
func renameRouter(
	renamer repohandlers.RepoRenamer,
	frames *[]dto.RepoDTO,
) *gin.Engine {
	h := repohandlers.NewWithDeps(&fakeStore{}, nil, nil, func(d dto.RepoDTO) {
		*frames = append(*frames, d)
	}).WithRenamer(renamer)
	r := gin.New()
	r.PATCH("/v0/projects/:projectId/repos/:repoId", h.Rename)
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

// TestRenameRepo_BroadcastsRenamedDTO pins the happy path: a valid rename passes
// the trimmed name + repoId to the renamer, answers 204, and broadcasts the
// updated RepoDTO on the repos WS stream (the sidebar refresh, not the response).
func TestRenameRepo_BroadcastsRenamedDTO(t *testing.T) {
	ren := &fakeRenamer{repo: domain.Repository{
		ID: "r1", ProjectID: "p1", Name: "Renamed Repo", AvatarLabel: "R",
	}}
	var frames []dto.RepoDTO
	rec := doPatch(renameRouter(ren, &frames), "/v0/projects/p1/repos/r1",
		map[string]string{"name": "  Renamed Repo  "})

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.String())
	assert.Equal(t, "r1", ren.gotID)
	assert.Equal(t, "Renamed Repo", ren.gotName, "the handler trims surrounding whitespace")
	require.Len(t, frames, 1, "a rename must broadcast the updated RepoDTO")
	assert.Equal(t, "Renamed Repo", frames[0].Name)
	assert.Equal(t, "R", frames[0].AvatarLabel)
}

// TestRenameRepo_EmptyName_400 pins synchronous name validation: a blank name
// (whitespace only) is rejected 400 before the renamer runs, and nothing is
// broadcast.
func TestRenameRepo_EmptyName_400(t *testing.T) {
	ren := &fakeRenamer{}
	var frames []dto.RepoDTO
	rec := doPatch(renameRouter(ren, &frames), "/v0/projects/p1/repos/r1",
		map[string]string{"name": "   "})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.False(t, ren.called, "the renamer must not run on an invalid name")
	assert.Empty(t, frames)
}

// TestRenameRepo_NotFound_404 pins that an unknown repo (renamer returns
// ErrNotFound) surfaces as a 404, not a 500.
func TestRenameRepo_NotFound_404(t *testing.T) {
	ren := &fakeRenamer{err: apperr.ErrNotFound}
	var frames []dto.RepoDTO
	rec := doPatch(renameRouter(ren, &frames), "/v0/projects/p1/repos/missing",
		map[string]string{"name": "Whatever"})

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, frames, "a failed rename broadcasts nothing")
}

// TestRenameRepo_NoRenamer_500 pins the bare-handler fallback: with no renamer
// wired the endpoint answers 500 rather than panicking.
func TestRenameRepo_NoRenamer_500(t *testing.T) {
	var frames []dto.RepoDTO
	rec := doPatch(renameRouter(nil, &frames), "/v0/projects/p1/repos/r1",
		map[string]string{"name": "Whatever"})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Empty(t, frames)
}

// The rename endpoint takes the same unsanitised name the create endpoint does,
// and feeds it to the same slug fallback — so it needs the same guard, or a
// rename walks an existing repo's future worktrees out of the crowbar home.
func TestRenameRepo_RejectsNamesThatEscapeTheCrowbarHome(t *testing.T) {
	for _, c := range traversalNames {
		t.Run(c.name, func(t *testing.T) {
			ren := &fakeRenamer{}
			var frames []dto.RepoDTO
			rec := doPatch(renameRouter(ren, &frames), "/v0/projects/p1/repos/r1",
				map[string]string{"name": c.value})

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.False(t, ren.called, "the renamer must not run on an unsafe name")
			assert.Empty(t, frames)
		})
	}
}
