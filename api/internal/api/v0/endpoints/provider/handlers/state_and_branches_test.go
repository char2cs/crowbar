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

// configurableReader lets each test dial in the workspace List behavior
// needed to exercise worktreeForRepo's success and not-found branches.
type configurableReader struct {
	all     []domain.Workspace
	listErr error
}

func (c configurableReader) List(
	_ context.Context,
) ([]domain.Workspace, error) {
	if c.listErr != nil {
		return nil, c.listErr
	}
	return c.all, nil
}

// TestState_NoResolvedWorkspace_500 is the wiring-bug guard: a route mounted
// outside chatScoped's resolveChatWorktree middleware finds no workspace on
// reqscope and reports a 500 rather than 404ing on a URL param that no
// chat-scoped route carries any more (Handlers.workspace).
func TestState_NoResolvedWorkspace_500(
	t *testing.T,
) {
	r := newRouterNoWorkspace(configurableEngine{}, configurableReader{})

	rec := do(r, "/v0/chats/chat1/provider")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestState_PollError pins that a poll failure surfaces as a 500.
func TestState_PollError(
	t *testing.T,
) {
	eng := configurableEngine{pollErr: errors.New("auth failed")}
	r := newRouter(eng, configurableReader{})

	rec := do(r, "/v0/chats/chat1/provider")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestState_Success pins the happy poll path returns 200 with the resolved
// provider state.
func TestState_Success(
	t *testing.T,
) {
	eng := configurableEngine{pollState: engineprovider.ProviderState{Protected: true}}
	r := newRouter(eng, configurableReader{})

	rec := do(r, "/v0/chats/chat1/provider")
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
