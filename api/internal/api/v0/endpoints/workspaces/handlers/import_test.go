package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workspacehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/workspaces/handlers"
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

func (f *fakeRemote) RemoteTrackingBranchExists(_ context.Context, _ string, branch string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return f.exists[branch], nil
}

func importRouter(remote workspacehandlers.RemoteRefs) *gin.Engine {
	reader := &fakeReader{get: domain.Workspace{ID: "w1"}}
	repos := &fakeRepos{repo: &domain.Repository{ID: "r1", ProjectID: "p1", Path: "/repo"}}
	h := workspacehandlers.
		New(reader, &fakeHierarchy{}, repos, &fakeLastErrors{}, fakeWork{}).
		WithRemoteRefs(remote)
	r := gin.New()
	r.POST("/v0/projects/:projectId/repos/:repoId/workspaces/import", h.Import)
	return r
}

const importTarget = "/v0/projects/p1/repos/r1/workspaces/import"

func TestImport_RefusesABranchTheRemoteDoesNotHave(t *testing.T) {
	remote := &fakeRemote{exists: map[string]bool{"feature/real": true}}

	rec := do(importRouter(remote), http.MethodPost, importTarget,
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

	rec := do(importRouter(remote), http.MethodPost, importTarget, `{"branches":["feature/x"]}`)

	assert.Less(t, rec.Code, 400, "a failed refresh must not become a refusal")
}

func TestImport_ImportsWithoutTheCheckWhenTheRefLookupErrors(t *testing.T) {
	remote := &fakeRemote{existsErr: errors.New("bad object")}

	rec := do(importRouter(remote), http.MethodPost, importTarget, `{"branches":["feature/x"]}`)

	assert.Less(t, rec.Code, 400, "a failed ref check must not become a refusal")
}

func TestImport_SkipsTheCheckWithNoRemoteWired(t *testing.T) {
	// A nil remote degrades rather than panicking — the same wiring contract
	// WithRemoteRefs documents.
	rec := do(importRouter(nil), http.MethodPost, importTarget, `{"branches":["feature/x"]}`)

	assert.Less(t, rec.Code, 400)
}
