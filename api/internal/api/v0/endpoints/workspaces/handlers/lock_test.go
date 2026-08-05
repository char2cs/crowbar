package handlers_test

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// POST .../workspaces/:wsId/lock — the user's own lock decision.
//
// It answers SYNCHRONOUSLY, unlike the git-backed operations around it: one
// aggregate write with no git in it, and every way it can be refused is
// something the user has to see while the menu they fired it from is still up.
// A 202 would strand those behind a LastError frame nobody is watching.
func lockRouter(reader *fakeReader) *gin.Engine {
	return newRouter(reader, &fakeHierarchy{}, &fakeRepos{})
}

func TestLock_LocksAWorkspace(t *testing.T) {
	reader := &fakeReader{get: domain.Workspace{ID: "w1"}}

	rec := do(lockRouter(reader), http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/w1/lock", `{"locked":true}`)

	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	require.Equal(t, 1, reader.lockCalls)
	require.NotNil(t, reader.gotLocked)
	assert.True(t, *reader.gotLocked)
}

func TestLock_UnlocksAProtectedBranch(t *testing.T) {
	// The whole point of the override: `main` used to be locked with no way out.
	reader := &fakeReader{get: domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusLocked}}

	rec := do(lockRouter(reader), http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/w1/lock", `{"locked":false}`)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.NotNil(t, reader.gotLocked)
	assert.False(t, *reader.gotLocked)
}

func TestLock_OmittedLockedHandsTheQuestionBackToTheProvider(t *testing.T) {
	// nil is a THIRD answer, not a synonym for false: it clears the override so
	// the branch reverts to locked iff the provider protects it. A non-pointer
	// field would make "no opinion" indistinguishable from "unlock".
	reader := &fakeReader{get: domain.Workspace{ID: "w1"}}

	rec := do(lockRouter(reader), http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/w1/lock", `{}`)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, 1, reader.lockCalls)
	assert.Nil(t, reader.gotLocked)
}

func TestLock_RejectsAMalformedBody(t *testing.T) {
	reader := &fakeReader{get: domain.Workspace{ID: "w1"}}

	rec := do(lockRouter(reader), http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/w1/lock", `{"locked":`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, reader.lockCalls, "a malformed body must never reach the store")
}

func TestLock_404sOnAWorkspaceThatIsNotThere(t *testing.T) {
	reader := &fakeReader{getErr: apperr.ErrNotFound}

	rec := do(lockRouter(reader), http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/nope/lock", `{"locked":true}`)

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Zero(t, reader.lockCalls)
}

func TestLock_SurfacesARefusalSynchronously(t *testing.T) {
	// The home workspace and a placeholder with no worktree are both refused by
	// the aggregate. The user pressed a menu item; the answer belongs in the
	// response, not on a stream.
	reader := &fakeReader{get: domain.Workspace{ID: "w1"}, lockErr: apperr.ErrInvalidArgument}

	rec := do(lockRouter(reader), http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/w1/lock", `{"locked":false}`)

	assert.GreaterOrEqual(t, rec.Code, 400)
	assert.Less(t, rec.Code, 500)
}
