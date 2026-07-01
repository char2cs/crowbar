package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

// configurableEngine lets each test dial in the PollOnView/ProtectedBranches
// behavior needed to exercise the State/ProtectedBranches error branches.
type configurableEngine struct {
	pollState engineprovider.ProviderState
	pollErr   error
	branches  []string
	branchErr error
}

func (c configurableEngine) PollOnView(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) (engineprovider.ProviderState, error) {
	return c.pollState, c.pollErr
}

func (c configurableEngine) ProtectedBranches(
	_ context.Context,
	_ string,
) ([]string, error) {
	return c.branches, c.branchErr
}

// configurableReader lets each test dial in the workspace Get/List behavior
// needed to exercise worktreeForRepo's success and not-found branches.
type configurableReader struct {
	ws      domain.Workspace
	getErr  error
	all     []domain.Workspace
	listErr error
}

func (c configurableReader) Get(
	_ context.Context,
	id string,
) (domain.Workspace, error) {
	if c.getErr != nil {
		return domain.Workspace{}, c.getErr
	}
	return c.ws, nil
}

func (c configurableReader) List(
	_ context.Context,
) ([]domain.Workspace, error) {
	if c.listErr != nil {
		return nil, c.listErr
	}
	return c.all, nil
}

// TestState_WorkspaceNotFound pins that an unknown workspace id 404s before
// any poll is attempted.
func TestState_WorkspaceNotFound(
	t *testing.T,
) {
	r := newRouter(configurableEngine{}, configurableReader{getErr: errors.New("no such workspace")})

	rec := do(r, "/v0/workspaces/missing/provider")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestState_PollError pins that a poll failure surfaces as a 500.
func TestState_PollError(
	t *testing.T,
) {
	eng := configurableEngine{pollErr: errors.New("auth failed")}
	reader := configurableReader{ws: domain.Workspace{ID: "ws1", WorktreePath: "/repo", Branch: "main"}}
	r := newRouter(eng, reader)

	rec := do(r, "/v0/workspaces/ws1/provider")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestState_Success pins the happy poll path returns 200 with the resolved
// provider state.
func TestState_Success(
	t *testing.T,
) {
	eng := configurableEngine{pollState: engineprovider.ProviderState{Protected: true}}
	reader := configurableReader{ws: domain.Workspace{ID: "ws1", WorktreePath: "/repo", Branch: "main"}}
	r := newRouter(eng, reader)

	rec := do(r, "/v0/workspaces/ws1/provider")
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestProtectedBranches_RepoNotFound pins that a repo with no matching
// workspace 404s.
func TestProtectedBranches_RepoNotFound(
	t *testing.T,
) {
	reader := configurableReader{all: []domain.Workspace{
		{ID: "ws1", RepoID: "other-repo", WorktreePath: "/repo"},
	}}
	r := newRouter(configurableEngine{}, reader)

	rec := do(r, "/v0/repos/missing-repo/protected-branches")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestProtectedBranches_ListError pins that a wsReader.List failure also 404s
// (worktreeForRepo folds a List error into the same not-found response).
func TestProtectedBranches_ListError(
	t *testing.T,
) {
	reader := configurableReader{listErr: errors.New("db down")}
	r := newRouter(configurableEngine{}, reader)

	rec := do(r, "/v0/repos/r1/protected-branches")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestProtectedBranches_EngineError pins that a provider engine failure
// surfaces as a 500 once the worktree path resolves.
func TestProtectedBranches_EngineError(
	t *testing.T,
) {
	eng := configurableEngine{branchErr: errors.New("provider unreachable")}
	reader := configurableReader{all: []domain.Workspace{
		{ID: "ws1", RepoID: "r1", WorktreePath: "/repo"},
	}}
	r := newRouter(eng, reader)

	rec := do(r, "/v0/repos/r1/protected-branches")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestProtectedBranches_NoBranches pins the success path when the repo has no
// protected branches configured.
func TestProtectedBranches_NoBranches(
	t *testing.T,
) {
	eng := configurableEngine{branches: []string{}}
	reader := configurableReader{all: []domain.Workspace{
		{ID: "ws1", RepoID: "r1", WorktreePath: "/repo"},
	}}
	r := newRouter(eng, reader)

	rec := do(r, "/v0/repos/r1/protected-branches")
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestProtectedBranches_SomeBranches pins the success path when the repo has
// protected branches configured, resolved via a repoID matching a
// non-first entry in the workspace list.
func TestProtectedBranches_SomeBranches(
	t *testing.T,
) {
	eng := configurableEngine{branches: []string{"main", "release"}}
	reader := configurableReader{all: []domain.Workspace{
		{ID: "ws0", RepoID: "other", WorktreePath: "/other"},
		{ID: "ws1", RepoID: "r1", WorktreePath: "/repo"},
	}}
	r := newRouter(eng, reader)

	rec := do(r, "/v0/repos/r1/protected-branches")
	assert.Equal(t, http.StatusOK, rec.Code)
}
