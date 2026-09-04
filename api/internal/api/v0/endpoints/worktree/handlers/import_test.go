package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	worktreehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/worktree/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Import refuses a branch that is not on the remote SYNCHRONOUSLY, on the
// request path. The alternative is what it replaced: the batch fails deep in the
// background, where its only channel is runAsync's blank workspace id — a no-op
// in broadcastLastError — and the caller's optimistic row spins forever.
//
// The degradation paths matter as much as the refusal. A fetch that fails, or a
// ref check that errors, must NOT turn into a refusal: the remote being
// unreachable is not evidence that a branch is absent, and importing without the
// check is the weaker but correct answer.
type fakeRemote struct {
	fetchErr   error
	exists     map[string]bool
	existsErr  error
	fetchCalls int
}

func (f *fakeRemote) FetchPrune(_ context.Context, _ string) error {
	f.fetchCalls++
	return f.fetchErr
}

func (f *fakeRemote) RemoteTrackingBranchExists(_ context.Context, _, branch string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return f.exists[branch], nil
}

func importRouterWith(
	remote worktreehandlers.RemoteRefs,
	hierarchy *fakeHierarchy,
) *gin.Engine {
	repos := &fakeRepos{repo: &domain.Repository{
		ID:            "r1",
		ProjectID:     "p1",
		Path:          "/repo",
		RemoteURL:     "git@example.com:acme/app.git",
		DefaultBranch: "main",
	}}
	h := worktreehandlers.
		New(&fakeReader{}, hierarchy, repos, &fakeLastErrors{}, fakeWork{}).
		WithRemoteRefs(remote)
	r := gin.New()
	r.POST("/v0/projects/:projectId/repos/:repoId/chats/import-batch", h.Import)
	return r
}

func importRouter(remote worktreehandlers.RemoteRefs) *gin.Engine {
	return importRouterWith(remote, &fakeHierarchy{})
}

const importBatchTarget = "/v0/projects/p1/repos/r1/chats/import-batch"

// The proof the batch route has to carry is that it resolves the repo facts off
// :repoId and hands CreateFromImport the identical input — not that it merely
// answers 202. A loop over POST .../chats would answer 202 too, and drop the
// PR-graph parenting, the missing-ancestor creation and the held-branch
// placeholder that only CreateFromImport performs.
func TestImport_TheBatchRouteHandsCreateFromImportTheRepoFacts(t *testing.T) {
	hierarchy := &fakeHierarchy{importDone: make(chan struct{})}
	remote := &fakeRemote{exists: map[string]bool{"feature/x": true}}

	rec := do(importRouterWith(remote, hierarchy), http.MethodPost, importBatchTarget,
		`{"branches":["feature/x"]}`)

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	<-hierarchy.importDone
	assert.Equal(t, workspace.ImportInput{
		RepoID:        "r1",
		ProjectID:     "p1",
		RepoPath:      "/repo",
		RemoteURL:     "git@example.com:acme/app.git",
		DefaultBranch: "main",
		Branches:      []string{"feature/x"},
	}, hierarchy.gotImport)
}

// The default branch is refused on the relocated route for the same reason it
// always was: the chain walk terminates there, so a 202 would promise work that
// creates nothing and leave the caller's optimistic row spinning.
func TestImport_TheRelocatedRouteStillRefusesTheDefaultBranch(t *testing.T) {
	rec := do(importRouter(&fakeRemote{exists: map[string]bool{"main": true}}),
		http.MethodPost, importBatchTarget, `{"branches":["main"]}`)

	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "main")
}

func TestImport_RefusesABranchTheRemoteDoesNotHave(t *testing.T) {
	remote := &fakeRemote{exists: map[string]bool{"feature/real": true}}

	rec := do(importRouter(remote), http.MethodPost, importBatchTarget,
		`{"branches":["feature/real","feature/ghost"]}`)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "feature/ghost")
	// Refreshed first: the check is against the remote as it is NOW, not as the
	// clone last heard it.
	assert.Equal(t, 1, remote.fetchCalls)
}

func TestImport_ImportsWithoutTheCheckWhenTheFetchFails(t *testing.T) {
	// An unreachable remote is not evidence that a branch is absent.
	remote := &fakeRemote{fetchErr: errors.New("network down")}

	rec := do(importRouter(remote), http.MethodPost, importBatchTarget, `{"branches":["feature/x"]}`)

	assert.Less(t, rec.Code, 400, "a failed refresh must not become a refusal")
}

func TestImport_ImportsWithoutTheCheckWhenTheRefLookupErrors(t *testing.T) {
	remote := &fakeRemote{existsErr: errors.New("bad object")}

	rec := do(importRouter(remote), http.MethodPost, importBatchTarget, `{"branches":["feature/x"]}`)

	assert.Less(t, rec.Code, 400, "a failed ref check must not become a refusal")
}

func TestImport_SkipsTheCheckWithNoRemoteWired(t *testing.T) {
	// A nil remote degrades rather than panicking — the same wiring contract
	// WithRemoteRefs documents.
	rec := do(importRouter(nil), http.MethodPost, importBatchTarget, `{"branches":["feature/x"]}`)

	assert.Less(t, rec.Code, 400)
}
